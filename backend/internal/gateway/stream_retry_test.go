package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/stats"
)

func TestConvertedChatStreamRecoversAfterDeliveredOutput(t *testing.T) {
	for _, client := range []WireProtocol{WireAnthropic, WireResponses} {
		for _, scenario := range []string{"text", "tool arguments", "divergent replay", "disabled retries", "finished before disconnect"} {
			t.Run(string(client)+"/"+scenario, func(t *testing.T) {
				previousConfig, previousPath := config.Snapshot(), stats.Path()
				stats.SetPath(filepath.Join(t.TempDir(), "usage.db"))
				t.Cleanup(func() { config.ApplyConfig(previousConfig); stats.SetPath(previousPath) })
				var attempts atomic.Int32
				observed := make(chan struct{})
				var observedOnce sync.Once
				requests := make(chan string, 3)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					request, _ := io.ReadAll(r.Body)
					requests <- string(request)
					n := attempts.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					if n == 1 {
						// Deliberately truncate a real HTTP response after the client
						// has observed a delta, reproducing the reported read failure.
						w.Header().Set("Content-Length", "100000")
						if scenario == "tool arguments" {
							_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_original\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"a\"}}]},\"finish_reason\":null}]}\n\n")
						} else {
							_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"already-\"},\"finish_reason\":null}]}\n\n")
						}
						if scenario == "finished before disconnect" {
							_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7,\"total_tokens\":10}}\n\n")
						}
						w.(http.Flusher).Flush()
						select {
						case <-observed:
						case <-r.Context().Done():
						case <-time.After(3 * time.Second):
							t.Error("client did not see live output before the stream ended")
						}
						return
					}
					finishReason := "stop"
					if scenario == "tool arguments" {
						finishReason = "tool_calls"
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_new_attempt\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"a.go\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
					} else if scenario == "divergent replay" {
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"different-answer\"},\"finish_reason\":null}]}\n\n")
					} else {
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"al\"},\"finish_reason\":null}]}\n\n")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ready-suffix\"},\"finish_reason\":null}]}\n\n")
					}
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\""+finishReason+"\"}]}\n\n")
					_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7,\"total_tokens\":10}}\n\ndata: [DONE]\n\n")
				}))
				defer upstream.Close()
				retries := 1
				if scenario == "disabled retries" {
					retries = 0
				}
				config.ApplyConfig(config.AppConfig{
					Upstreams: map[string]*config.UpstreamConfig{
						"retry-test": {BaseURL: upstream.URL, APIType: UpstreamOpenAI, CustomModels: []string{"retry-model"}, MaxRetries: &retries},
					},
					AllowDirectModels: true,
				})
				path := "/v1/messages"
				handler := ClaudeMessagesHandler
				request := `{"model":"retry-model","stream":true,"max_tokens":1024,"messages":[{"role":"user","content":"test"}]}`
				if client == WireResponses {
					path, handler = "/v1/responses", ResponsesHandler
					request = `{"model":"retry-model","stream":true,"input":"test"}`
				}
				server := httptest.NewServer(stats.TrackRequest(handler))
				defer server.Close()
				response, err := http.Post(server.URL+path, "application/json", strings.NewReader(request))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				reader := sse.NewReader(response.Body)
				var text, arguments strings.Builder
				terminals, failures, toolStarts := 0, 0, 0
				pings := 0
				for {
					event, err := reader.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					var data map[string]any
					_ = json.Unmarshal([]byte(event.Data), &data)
					switch event.Name {
					case "ping":
						pings++
					case "message_stop", "response.completed":
						terminals++
					case "error", "response.failed":
						failures++
					case "content_block_start":
						block, _ := data["content_block"].(map[string]any)
						if block["type"] == "tool_use" {
							toolStarts++
							if block["id"] != "call_original" {
								t.Errorf("tool identity changed: %v", block)
							}
						}
					case "content_block_delta":
						delta, _ := data["delta"].(map[string]any)
						if value, ok := delta["text"].(string); ok {
							text.WriteString(value)
						}
						if value, ok := delta["partial_json"].(string); ok {
							arguments.WriteString(value)
						}
						observedOnce.Do(func() { close(observed) })
					case "response.output_item.added":
						item, _ := data["item"].(map[string]any)
						if item["type"] == "function_call" {
							toolStarts++
							if item["call_id"] != "call_original" {
								t.Errorf("tool identity changed: %v", item)
							}
						}
					case "response.output_text.delta":
						value, _ := data["delta"].(string)
						text.WriteString(value)
						observedOnce.Do(func() { close(observed) })
					case "response.function_call_arguments.delta":
						value, _ := data["delta"].(string)
						arguments.WriteString(value)
						observedOnce.Do(func() { close(observed) })
					}
				}
				wantAttempts := int32(2)
				if scenario == "disabled retries" || scenario == "finished before disconnect" {
					wantAttempts = 1
				}
				if attempts.Load() != wantAttempts {
					t.Fatalf("attempts=%d, want %d", attempts.Load(), wantAttempts)
				}
				if wantAttempts == 2 && <-requests != <-requests {
					t.Fatal("retry changed the original request")
				}
				if scenario == "divergent replay" || scenario == "disabled retries" {
					if failures != 1 || terminals != 0 || text.String() != "already-" {
						t.Fatalf("failed replay changed output: failures=%d terminals=%d text=%q", failures, terminals, text.String())
					}
					return
				}
				if failures != 0 || terminals != 1 {
					t.Fatalf("failures=%d terminals=%d, want one successful terminal", failures, terminals)
				}
				if client == WireAnthropic && scenario == "text" && pings == 0 {
					t.Fatal("replay keepalive was not forwarded to the client")
				}
				if scenario == "tool arguments" {
					if toolStarts != 1 || arguments.String() != `{"path":"a.go"}` {
						t.Fatalf("tool output duplicated: starts=%d arguments=%q", toolStarts, arguments.String())
					}
				} else {
					wantText := "already-suffix"
					if scenario == "finished before disconnect" {
						wantText = "already-"
					}
					if text.String() != wantText {
						t.Fatalf("text=%q, want %q", text.String(), wantText)
					}
				}
				records, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
				if err != nil || len(records.Items) != 1 || records.Items[0].PromptTokens != 3 || records.Items[0].CompletionTokens != 7 {
					t.Fatalf("usage should be committed once: records=%+v error=%v", records, err)
				}
			})
		}
	}
}

func TestConvertedChatStreamFallsBackBeforeAnyEvent(t *testing.T) {
	for _, client := range []WireProtocol{WireAnthropic, WireResponses} {
		t.Run(string(client), func(t *testing.T) {
			var primaryCalls, backupCalls atomic.Int32
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				// Valid-looking SSE framing with no semantic events: the converter
				// must switch to a backup target instead of emitting an error.
				_, _ = io.WriteString(w, "data: \n\n")
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, true)
			}))
			defer backup.Close()
			installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{})
			path, handler := "/v1/messages", ClaudeMessagesHandler
			request := `{"model":"alias","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"test"}]}`
			if client == WireResponses {
				path, handler = "/v1/responses", ResponsesHandler
				request = `{"model":"alias","stream":true,"input":"test"}`
			}
			response := httptest.NewRecorder()
			stats.TrackRequest(handler)(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(request)))
			if response.Code != 200 || !strings.Contains(response.Body.String(), "backup-ok") || strings.Contains(response.Body.String(), "upstream_error") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
				t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
			}
		})
	}
}

func TestConvertedChatStreamKeepsRouteOrderBeforeOutput(t *testing.T) {
	for _, withLaterChat := range []bool{false, true} {
		t.Run(fmt.Sprintf("later_chat=%t", withLaterChat), func(t *testing.T) {
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: \n\n")
			}))
			defer primary.Close()
			var preferredCalls, laterCalls atomic.Int32
			preferred := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				preferredCalls.Add(1)
				writeRoutingSuccess(w, UpstreamAnthropic, true)
			}))
			defer preferred.Close()
			cfg := installFailoverConfig(t, primary, preferred, UpstreamOpenAI, UpstreamAnthropic, config.ModelRoutingConfig{MaxAttempts: 2})
			if withLaterChat {
				later := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					laterCalls.Add(1)
					writeRoutingSuccess(w, UpstreamOpenAI, true)
				}))
				defer later.Close()
				noRetries := 0
				cfg.Upstreams["later"] = &UpstreamConfig{BaseURL: later.URL, APIType: UpstreamOpenAI, MaxRetries: &noRetries}
				alias := cfg.ModelAlias["alias"]
				alias.Targets[0].Weight = 2
				alias.Targets = append(alias.Targets, config.ModelAliasTarget{Upstream: "later", TargetModel: "later-model", Weight: 1, Enabled: true})
				cfg.ModelAlias["alias"] = alias
				config.ApplyConfig(cfg)
			}
			response := httptest.NewRecorder()
			request := `{"model":"alias","stream":true,"max_tokens":64,"messages":[]}`
			ClaudeMessagesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request)))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") || preferredCalls.Load() != 1 || laterCalls.Load() != 0 {
				t.Fatalf("status=%d preferred=%d later=%d body=%s", response.Code, preferredCalls.Load(), laterCalls.Load(), response.Body)
			}
		})
	}
}

func TestConvertedChatStreamTriesBackupsAfterDivergentReasoning(t *testing.T) {
	for _, client := range []WireProtocol{WireAnthropic, WireResponses} {
		t.Run(string(client), func(t *testing.T) {
			var primaryCalls, divergingCalls, matchingCalls atomic.Int32
			thinkingSeen := make(chan struct{})
			thinking := func(value string) string {
				return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"reasoning_content\":%q}}]}\n\n", value)
			}
			const finish = "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n"
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if primaryCalls.Add(1) == 1 {
					w.Header().Set("Content-Length", "10000")
					_, _ = io.WriteString(w, thinking("shared-reasoning"))
					w.(http.Flusher).Flush()
					select {
					case <-thinkingSeen:
					case <-r.Context().Done():
					case <-time.After(4 * time.Second):
						t.Error("first reasoning delta was withheld while upstream was still generating")
					}
					return
				}
				_, _ = io.WriteString(w, thinking("divergent-primary")+finish)
			}))
			defer primary.Close()
			diverging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				divergingCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, thinking("divergent-backup")+finish)
			}))
			defer diverging.Close()
			matching := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				matchingCalls.Add(1)
				var envelope NativeRequestEnvelope
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil || envelope.Model != "matching-model" || !envelope.Stream {
					t.Errorf("backup request=%#v, err=%v", envelope, err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, thinking("shared-")+thinking("reasoning")+
					"data: {\"choices\":[{\"delta\":{\"content\":\"backup-answer\"}}]}\n\n"+finish)
			}))
			defer matching.Close()
			cfg := installFailoverConfig(t, primary, diverging, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{MaxAttempts: 3})
			one, zero := 1, 0
			cfg.Upstreams["primary"].MaxRetries = &one
			cfg.Upstreams["matching"] = &config.UpstreamConfig{BaseURL: matching.URL, APIType: UpstreamOpenAI, APIKey: "matching-key", MaxRetries: &zero}
			alias := cfg.ModelAlias["alias"]
			alias.Targets[0].Weight = 2
			alias.Targets = append(alias.Targets, config.ModelAliasTarget{Upstream: "matching", TargetModel: "matching-model", Weight: 1, Enabled: true})
			cfg.ModelAlias["alias"] = alias
			config.ApplyConfig(cfg)
			path, handler := "/v1/messages", ClaudeMessagesHandler
			request := `{"model":"alias","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"test"}]}`
			if client == WireResponses {
				path, handler = "/v1/responses", ResponsesHandler
				request = `{"model":"alias","stream":true,"input":"test"}`
			}
			server := httptest.NewServer(stats.TrackRequest(handler))
			defer server.Close()
			httpClient := &http.Client{Timeout: 8 * time.Second}
			response, err := httpClient.Post(server.URL+path, "application/json", strings.NewReader(request))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var reasoning, answer strings.Builder
			var seen sync.Once
			terminals, failures := 0, 0
			reader := sse.NewReader(response.Body)
			for {
				event, err := reader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				var data map[string]any
				_ = json.Unmarshal([]byte(event.Data), &data)
				switch event.Name {
				case "content_block_delta":
					delta, _ := data["delta"].(map[string]any)
					if value, ok := delta["thinking"].(string); ok {
						reasoning.WriteString(value)
						seen.Do(func() { close(thinkingSeen) })
					}
					if value, ok := delta["text"].(string); ok {
						answer.WriteString(value)
					}
				case "response.reasoning_summary_text.delta":
					reasoning.WriteString(data["delta"].(string))
					seen.Do(func() { close(thinkingSeen) })
				case "response.output_text.delta":
					answer.WriteString(data["delta"].(string))
				case "message_stop", "response.completed":
					terminals++
				case "error", "response.failed":
					failures++
				}
			}
			if response.StatusCode != 200 || terminals != 1 || failures != 0 || reasoning.String() != "shared-reasoning" || answer.String() != "backup-answer" {
				t.Fatalf("status=%d terminals=%d failures=%d reasoning=%q answer=%q", response.StatusCode, terminals, failures, reasoning.String(), answer.String())
			}
			if primaryCalls.Load() != 2 || divergingCalls.Load() != 1 || matchingCalls.Load() != 1 {
				t.Fatalf("calls primary=%d diverging=%d matching=%d", primaryCalls.Load(), divergingCalls.Load(), matchingCalls.Load())
			}
			usage, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
			if err != nil || len(usage.Items) != 1 {
				t.Fatalf("usage=%#v err=%v", usage, err)
			}
			if item := usage.Items[0]; item.UpstreamName != "matching" || item.UpstreamModel != "matching-model" || item.UpstreamKeyIndex != 1 || item.UpstreamKeyTotal != 1 {
				t.Fatalf("usage was attributed to the wrong attempt: %#v", item)
			}
		})
	}
}
