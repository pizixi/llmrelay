package gateway

import (
	"encoding/json"
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
				config.ApplyConfig(config.AppConfig{Upstreams: map[string]*config.UpstreamConfig{
					"retry-test": {BaseURL: upstream.URL, APIType: UpstreamOpenAI, CustomModels: []string{"retry-model"}, MaxRetries: &retries},
				}})
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
