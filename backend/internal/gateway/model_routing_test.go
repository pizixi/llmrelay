package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/stats"
)

const routingChatResponse = `{"id":"chatcmpl_backup","object":"chat.completion","model":"backup-model","choices":[{"index":0,"message":{"role":"assistant","content":"backup-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`
const routingAnthropicResponse = `{"id":"msg_backup","type":"message","role":"assistant","model":"backup-model","content":[{"type":"text","text":"backup-ok"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`
const routingResponsesResponse = `{"id":"resp_backup","object":"response","model":"backup-model","status":"completed","output":[{"id":"msg_backup","type":"message","role":"assistant","content":[{"type":"output_text","text":"backup-ok","annotations":[]}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}`

func writeRoutingSuccess(w http.ResponseWriter, apiType UpstreamType, stream bool) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		body := routingChatResponse
		if apiType == UpstreamAnthropic {
			body = routingAnthropicResponse
		} else if apiType == UpstreamResponses {
			body = routingResponsesResponse
		}
		_, _ = io.WriteString(w, body)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	switch apiType {
	case UpstreamAnthropic:
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_backup\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"backup-model\",\"content\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\n"+
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"+
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"backup-ok\"}}\n\n"+
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"+
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"+
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	case UpstreamResponses:
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_backup\",\"model\":\"backup-model\",\"status\":\"in_progress\",\"output\":[]}}\n\n"+
			"event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"msg_backup\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n"+
			"event: response.content_part.added\ndata: {\"type\":\"response.content_part.added\",\"item_id\":\"msg_backup\",\"output_index\":0,\"content_index\":0,\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n"+
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_backup\",\"output_index\":0,\"content_index\":0,\"delta\":\"backup-ok\"}\n\n"+
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":"+routingResponsesResponse+"}\n\n")
	default:
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_backup\",\"model\":\"backup-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"backup-ok\"},\"finish_reason\":null}]}\n\n"+
			"data: {\"id\":\"chatcmpl_backup\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"+
			"data: [DONE]\n\n")
	}
}

func isolateModelRoutingTest(t *testing.T) {
	t.Helper()
	previous, previousPath := config.Snapshot(), stats.Path()
	usagePath := filepath.Join(t.TempDir(), "usage.db")
	t.Cleanup(func() {
		config.ApplyConfig(previous)
		stats.SetPath(previousPath)
		stats.LoadTokenStats()
	})
	stats.SetPath(usagePath)
	stats.LoadTokenStats()
}

func installFailoverConfig(t *testing.T, primary, backup *httptest.Server, primaryType, backupType UpstreamType, policy config.ModelRoutingConfig) config.AppConfig {
	t.Helper()
	isolateModelRoutingTest(t)
	noRetries := 0
	cfg := config.AppConfig{
		ModelRouting: policy,
		ModelAlias: map[string]config.ModelAlias{"alias": {Targets: []config.ModelAliasTarget{
			{Upstream: "backup", TargetModel: "backup-model", Weight: 1, Enabled: true},
			{Upstream: "primary", TargetModel: "primary-model", Weight: 10, Enabled: true},
		}}},
		Upstreams: map[string]*config.UpstreamConfig{
			"primary": {BaseURL: primary.URL, APIType: primaryType, MaxRetries: &noRetries},
			"backup":  {BaseURL: backup.URL, APIType: backupType, MaxRetries: &noRetries},
		},
	}
	config.ApplyConfig(cfg)
	return cfg
}

func TestModelFailoverAcrossEveryProtocolAndStreaming(t *testing.T) {
	for _, client := range []UpstreamType{UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses} {
		for _, backupType := range []UpstreamType{UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", client, backupType, stream), func(t *testing.T) {
					var primaryCalls, backupCalls atomic.Int32
					primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						primaryCalls.Add(1)
						var body NativeRequestEnvelope
						_ = json.NewDecoder(r.Body).Decode(&body)
						if body.Model != "primary-model" {
							t.Errorf("primary model=%q", body.Model)
						}
						w.Header().Set("Retry-After", "7200")
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = io.WriteString(w, `{"error":{"message":"failed primary"}}`)
					}))
					defer primary.Close()
					backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						backupCalls.Add(1)
						var body NativeRequestEnvelope
						_ = json.NewDecoder(r.Body).Decode(&body)
						if body.Model != "backup-model" || body.Stream != stream {
							t.Errorf("backup envelope=%#v", body)
						}
						writeRoutingSuccess(w, backupType, stream)
					}))
					defer backup.Close()
					installFailoverConfig(t, primary, backup, client, backupType, config.ModelRoutingConfig{})
					path, handler := "/v1/chat/completions", ChatCompletionsHandler
					body := fmt.Sprintf(`{"model":"alias","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream)
					if client == UpstreamAnthropic {
						path, handler = "/v1/messages", ClaudeMessagesHandler
						body = fmt.Sprintf(`{"model":"alias","stream":%t,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`, stream)
					} else if client == UpstreamResponses {
						path, handler = "/v1/responses", ResponsesHandler
						body = fmt.Sprintf(`{"model":"alias","stream":%t,"input":"hello"}`, stream)
					}
					response := httptest.NewRecorder()
					stats.TrackRequest(handler)(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
					if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") || strings.Contains(response.Body.String(), "failed primary") || response.Header().Get("Retry-After") != "" {
						t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
					}
					if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
						t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
					}
					usage, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
					if err != nil || usage.Total != 1 || len(usage.Items) != 1 || usage.Items[0].UpstreamName != "backup" || usage.Items[0].UpstreamModel != "backup-model" || usage.Items[0].RequestCount != 1 {
						t.Fatalf("usage=%#v err=%v", usage, err)
					}
				})
			}
		}
	}
}

func TestModelFailoverStatusPolicyAndCooldown(t *testing.T) {
	for _, test := range []struct {
		name                      string
		status                    int
		body                      string
		policy                    config.ModelRoutingConfig
		wantStatus                int
		primaryCalls, backupCalls int32
	}{
		{"server cooldown", 503, "upstream failed", config.ModelRoutingConfig{}, 200, 3, 4},
		{"auth cooldown", 401, "invalid key", config.ModelRoutingConfig{}, 200, 3, 4},
		{"rate limit without cooldown", 429, "rate limited", config.ModelRoutingConfig{}, 200, 4, 4},
		{"missing model", 404, "missing model", config.ModelRoutingConfig{}, 200, 3, 4},
		{"model error code", 400, `{"error":{"code":"model_not_found"}}`, config.ModelRoutingConfig{}, 200, 3, 4},
		{"invalid input", 400, `{"error":{"code":"context_length_exceeded"}}`, config.ModelRoutingConfig{}, 400, 4, 0},
		{"cooldown disabled", 503, "upstream failed", config.ModelRoutingConfig{DisableCooldown: true}, 200, 4, 4},
		{"failover disabled", 429, "rate limited", config.ModelRoutingConfig{DisableFailover: true}, 429, 4, 0},
		{"attempt budget", 429, "rate limited", config.ModelRoutingConfig{MaxAttempts: 1}, 503, 4, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var primaryCalls, backupCalls atomic.Int32
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, false)
			}))
			defer backup.Close()
			installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, test.policy)
			for range 4 {
				response := httptest.NewRecorder()
				ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias","messages":[]}`)))
				if response.Code != test.wantStatus {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			}
			if primaryCalls.Load() != test.primaryCalls || backupCalls.Load() != test.backupCalls {
				t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
			}
		})
	}
}

func TestSingleModelTargetBypassesCooldown(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			const failure = `{"error":{"message":"primary key rejected"}}`
			var primaryCalls, backupCalls atomic.Int32
			var recovered atomic.Bool
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				if recovered.Load() {
					writeRoutingSuccess(w, UpstreamOpenAI, stream)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, failure)
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, stream)
			}))
			defer backup.Close()
			cfg := installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
			setBackupEnabled := func(enabled bool) {
				cfg.ModelAlias["alias"].Targets[0].Enabled = enabled
				config.ApplyConfig(cfg)
			}
			body := fmt.Sprintf(`{"model":"alias","stream":%t,"messages":[]}`, stream)
			request := func() *httptest.ResponseRecorder {
				response := httptest.NewRecorder()
				ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
				return response
			}

			setBackupEnabled(false)
			for range 2 {
				response := request()
				if response.Code != http.StatusUnauthorized || response.Body.String() != failure || response.Header().Get("Retry-After") != "30" {
					t.Fatalf("single target returned status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
				}
			}
			setBackupEnabled(true)
			if response := request(); response.Code != http.StatusOK || primaryCalls.Load() != 3 || backupCalls.Load() != 1 {
				t.Fatalf("failover returned status=%d primary calls=%d backup calls=%d body=%s", response.Code, primaryCalls.Load(), backupCalls.Load(), response.Body)
			}
			if response := request(); response.Code != http.StatusOK || primaryCalls.Load() != 3 || backupCalls.Load() != 2 {
				t.Fatalf("cooled primary was not skipped: status=%d primary calls=%d backup calls=%d", response.Code, primaryCalls.Load(), backupCalls.Load())
			}
			setBackupEnabled(false)
			recovered.Store(true)
			if response := request(); response.Code != http.StatusOK || primaryCalls.Load() != 4 || backupCalls.Load() != 2 {
				t.Fatalf("recovered single target returned status=%d primary calls=%d backup calls=%d body=%s", response.Code, primaryCalls.Load(), backupCalls.Load(), response.Body)
			}
		})
	}
}

func TestSingleDirectModelStillRetriesOtherKeys(t *testing.T) {
	var rejectedCalls, successfulCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer rejected-key" {
			rejectedCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		successfulCalls.Add(1)
		writeRoutingSuccess(w, UpstreamOpenAI, false)
	}))
	defer server.Close()
	isolateModelRoutingTest(t)
	oneRetry := 1
	config.ApplyConfig(config.AppConfig{
		AllowDirectModels: true,
		ModelAlias:        map[string]config.ModelAlias{},
		ModelRouting:      config.ModelRoutingConfig{FailureThreshold: 1},
		Upstreams: map[string]*config.UpstreamConfig{
			"single-direct": {BaseURL: server.URL, APIType: UpstreamOpenAI, APIKey: "rejected-key\nworking-key", CustomModels: []string{"direct-model"}, MaxRetries: &oneRetry},
		},
	})
	for range 4 {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"direct-model","messages":[]}`))
		ChatCompletionsHandler(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	}
	if rejectedCalls.Load() != 2 || successfulCalls.Load() != 4 {
		t.Fatalf("calls rejected key=%d working key=%d", rejectedCalls.Load(), successfulCalls.Load())
	}
}

func TestModelCooldownKeepsSiblingKeyRoutable(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var badCalls, goodCalls, backupCalls atomic.Int32
			var allFailed atomic.Bool
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "Bearer bad-key" {
					badCalls.Add(1)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				goodCalls.Add(1)
				if allFailed.Load() {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeRoutingSuccess(w, UpstreamOpenAI, stream)
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, stream)
			}))
			defer backup.Close()
			cfg := installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
			one := 1
			cfg.Upstreams["primary"].APIKey = "bad-key\ngood-key"
			cfg.Upstreams["primary"].MaxRetries = &one
			config.ApplyConfig(cfg)
			body := fmt.Sprintf(`{"model":"alias","stream":%t,"messages":[]}`, stream)
			for range 5 {
				response := httptest.NewRecorder()
				ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
				if response.Code != 200 || !strings.Contains(response.Body.String(), "backup-ok") {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			}
			if badCalls.Load() != 1 || goodCalls.Load() != 5 || backupCalls.Load() != 0 {
				t.Fatalf("one cooled key: bad=%d good=%d backup=%d", badCalls.Load(), goodCalls.Load(), backupCalls.Load())
			}
			allFailed.Store(true)
			for range 2 {
				response := httptest.NewRecorder()
				ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
				if response.Code != 200 || !strings.Contains(response.Body.String(), "backup-ok") {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			}
			if badCalls.Load() != 1 || goodCalls.Load() != 6 || backupCalls.Load() != 2 {
				t.Fatalf("all keys cooled: bad=%d good=%d backup=%d", badCalls.Load(), goodCalls.Load(), backupCalls.Load())
			}
		})
	}
}

func TestModelFailoverUsesOriginalUpstreamFailure(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"model_not_found","message":"The request was rejected"}`)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRoutingSuccess(w, UpstreamAnthropic, false)
	}))
	defer backup.Close()
	installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamAnthropic, config.ModelRoutingConfig{})
	response := httptest.NewRecorder()
	request := `{"model":"alias","max_tokens":64,"messages":[]}`
	ClaudeMessagesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestModelFailoverOnEmptyNonStreamBody(t *testing.T) {
	for _, client := range []UpstreamType{UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses} {
		t.Run(string(client), func(t *testing.T) {
			var primaryCalls, backupCalls atomic.Int32
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, false)
			}))
			defer backup.Close()
			installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{})
			path, handler := "/v1/chat/completions", ChatCompletionsHandler
			body := `{"model":"alias","messages":[]}`
			if client == UpstreamAnthropic {
				path, handler = "/v1/messages", ClaudeMessagesHandler
				body = `{"model":"alias","max_tokens":64,"messages":[]}`
			} else if client == UpstreamResponses {
				path, handler = "/v1/responses", ResponsesHandler
				body = `{"model":"alias","input":"hello"}`
			}
			response := httptest.NewRecorder()
			stats.TrackRequest(handler)(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
				t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
			}
		})
	}
}

func TestModelFailoverOnStreamWithoutEvents(t *testing.T) {
	for _, primaryType := range []UpstreamType{UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses} {
		t.Run(string(primaryType), func(t *testing.T) {
			var primaryCalls, backupCalls atomic.Int32
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				// A 200 stream that contains no SSE events must not be committed
				// to the client; routing should move to the backup target.
				_, _ = io.WriteString(w, "\n\n")
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamAnthropic, true)
			}))
			defer backup.Close()
			installFailoverConfig(t, primary, backup, primaryType, UpstreamAnthropic, config.ModelRoutingConfig{})
			response := httptest.NewRecorder()
			ClaudeMessagesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"alias","stream":true,"max_tokens":64,"messages":[]}`)))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
				t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
			}
		})
	}
}

func TestModelFailoverExhaustionAndCancellation(t *testing.T) {
	var calls atomic.Int32
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "private upstream details")
	}))
	defer failed.Close()
	installFailoverConfig(t, failed, failed, UpstreamOpenAI, UpstreamAnthropic, config.ModelRoutingConfig{})
	for range 4 {
		response := httptest.NewRecorder()
		ClaudeMessagesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"alias","messages":[],"max_tokens":1}`)))
		if response.Code != 503 || response.Header().Get("Retry-After") == "" || strings.Contains(response.Body.String(), "private") || !strings.Contains(response.Body.String(), `"type":"error"`) {
			t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
	}
	if calls.Load() != 6 {
		t.Fatalf("cooled targets retried, calls=%d", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ChatCompletionsHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias","messages":[]}`)).WithContext(ctx))
	if calls.Load() != 6 {
		t.Fatal("canceled request contacted upstream")
	}
}

func TestModelFailoverBeforeStreamFirstByte(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeRoutingSuccess(w, UpstreamOpenAI, true) }))
	defer backup.Close()
	installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{})
	response := httptest.NewRecorder()
	ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias","stream":true,"messages":[]}`)))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "backup-ok") || strings.Contains(response.Body.String(), "upstream_error") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestModelFailoverAfterCommittedStreamUsesCooldownForNextRequest(t *testing.T) {
	var primaryCalls, backupCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "10000")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"primary-text\"}}]}\n\n")
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backupCalls.Add(1)
		writeRoutingSuccess(w, UpstreamOpenAI, true)
	}))
	defer backup.Close()
	installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
	for _, expected := range []string{"primary-text", "backup-ok"} {
		response := httptest.NewRecorder()
		ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias","stream":true,"messages":[]}`)))
		if response.Code != 200 || !strings.Contains(response.Body.String(), expected) || (expected == "primary-text" && strings.Contains(response.Body.String(), "backup-ok")) {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if primaryCalls.Load() != 1 || backupCalls.Load() != 1 {
		t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
	}
}

func TestModelCooldownAfterMalformedStream(t *testing.T) {
	var primaryCalls, backupCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "malformed stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backupCalls.Add(1)
		writeRoutingSuccess(w, UpstreamOpenAI, true)
	}))
	defer backup.Close()
	cfg := installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
	cfg.Upstreams["primary"].APIKey = "primary-key"
	config.ApplyConfig(cfg)
	handler := stats.TrackRequest(ChatCompletionsHandler)
	for range 2 {
		response := httptest.NewRecorder()
		request := `{"model":"alias","stream":true,"messages":[]}`
		handler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(request)))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "backup-ok") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	}
	if primaryCalls.Load() != 1 || backupCalls.Load() != 2 {
		t.Fatalf("calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
	}
	usage, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil || len(usage.Items) != 2 {
		t.Fatalf("usage=%#v err=%v", usage, err)
	}
	for _, item := range usage.Items {
		if item.UpstreamName != "backup" || item.UpstreamKeyIndex != 0 || item.UpstreamKeyTotal != 0 {
			t.Errorf("usage inherited a key slot from the failed target: %#v", item)
		}
	}
}

func TestEqualWeightTargetsBalanceHealthyAndFailoverTraffic(t *testing.T) {
	for _, test := range []struct {
		name                 string
		status, backupWeight int
		primaryCalls         int32
	}{
		{"healthy equal weights", 200, 10, 2},
		{"equal weights with one cooling target", 401, 10, 1},
		{"equal backup tier after rate limit", 429, 5, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			var primaryCalls, backupCalls, thirdCalls atomic.Int32
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				primaryCalls.Add(1)
				if test.status == 200 {
					writeRoutingSuccess(w, UpstreamOpenAI, false)
					return
				}
				w.WriteHeader(test.status)
			}))
			defer primary.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				backupCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, false)
			}))
			defer backup.Close()
			third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				thirdCalls.Add(1)
				writeRoutingSuccess(w, UpstreamOpenAI, false)
			}))
			defer third.Close()
			cfg := installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
			cfg.Upstreams["third"] = &config.UpstreamConfig{BaseURL: third.URL, APIType: UpstreamOpenAI, MaxRetries: cfg.Upstreams["backup"].MaxRetries}
			cfg.ModelAlias["alias"] = config.ModelAlias{Targets: []config.ModelAliasTarget{
				{Upstream: "primary", TargetModel: "primary-model", Weight: 10, Enabled: true},
				{Upstream: "backup", TargetModel: "backup-model", Weight: test.backupWeight, Enabled: true},
				{Upstream: "third", TargetModel: "third-model", Weight: test.backupWeight, Enabled: true},
			}}
			config.ApplyConfig(cfg)
			for range 6 {
				response := httptest.NewRecorder()
				ChatCompletionsHandler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias","messages":[]}`)))
				if response.Code != 200 {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			}
			wantBackup := int32(3)
			if test.status == 200 {
				wantBackup = 2
			}
			if primaryCalls.Load() != test.primaryCalls || backupCalls.Load() != wantBackup || thirdCalls.Load() != wantBackup {
				t.Fatalf("calls primary=%d backup=%d third=%d", primaryCalls.Load(), backupCalls.Load(), thirdCalls.Load())
			}
		})
	}
}

func TestCompletedStreamAtUnexpectedEOFDoesNotCoolTarget(t *testing.T) {
	var primaryCalls, backupCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "10000")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"complete\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		backupCalls.Add(1)
		writeRoutingSuccess(w, UpstreamOpenAI, true)
	}))
	defer backup.Close()
	installFailoverConfig(t, primary, backup, UpstreamOpenAI, UpstreamOpenAI, config.ModelRoutingConfig{FailureThreshold: 1})
	for range 2 {
		response := httptest.NewRecorder()
		ResponsesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"alias","stream":true,"input":"hello"}`)))
		if response.Code != 200 || !strings.Contains(response.Body.String(), "response.completed") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if primaryCalls.Load() != 2 || backupCalls.Load() != 0 {
		t.Fatalf("completed stream cooled, calls primary=%d backup=%d", primaryCalls.Load(), backupCalls.Load())
	}
}
