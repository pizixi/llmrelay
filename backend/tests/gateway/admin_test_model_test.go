package gateway

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"llmrelay/backend/internal/stats"
)

func installAdminModelTestUpstream(t *testing.T, name string, upstream *UpstreamConfig) {
	t.Helper()
	configMu.Lock()
	oldUpstreams := upstreamCfgs
	oldDefaultName := defaultUpstreamName
	oldDefault := upstreamCfg
	upstreamCfgs = map[string]*UpstreamConfig{name: cloneUpstreamConfig(upstream)}
	defaultUpstreamName = name
	upstreamCfg = cloneUpstreamConfig(upstream)
	configMu.Unlock()
	t.Cleanup(func() {
		configMu.Lock()
		upstreamCfgs = oldUpstreams
		defaultUpstreamName = oldDefaultName
		upstreamCfg = oldDefault
		configMu.Unlock()
	})
}

func TestAdminTestModelStreamsConfiguredProtocol(t *testing.T) {
	previousStatsPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	stats.LoadTokenStats()
	t.Cleanup(func() { stats.SetPath(previousStatsPath) })

	tests := []struct {
		name         string
		apiType      UpstreamType
		baseSuffix   string
		wantPath     string
		wantBodyKey  string
		wantLimitKey string
		streamBody   string
	}{
		{
			name: "OpenAI Chat", apiType: UpstreamOpenAI, baseSuffix: "/v1", wantPath: "/v1/chat/completions", wantBodyKey: "messages", wantLimitKey: "max_tokens",
			streamBody: "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"plan\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello \"},\"finish_reason\":null}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n" +
				"data: [DONE]\n\n",
		},
		{
			name: "Anthropic", apiType: UpstreamAnthropic, wantPath: "/v1/messages", wantBodyKey: "messages", wantLimitKey: "max_tokens",
			streamBody: "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"usage\":{\"input_tokens\":1}}}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\",\"signature\":\"\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"plan\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello world\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		},
		{
			name: "OpenAI Responses", apiType: UpstreamResponses, baseSuffix: "/v1", wantPath: "/v1/responses", wantBodyKey: "input", wantLimitKey: "max_output_tokens",
			streamBody: "event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"plan\"}\n\n" +
				"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello world\"}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n" +
				"data: [DONE]\n\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testCase.wantPath {
					t.Errorf("path=%q, want %q", r.URL.Path, testCase.wantPath)
				}
				if testCase.apiType == UpstreamAnthropic {
					if got := r.Header.Get("x-api-key"); got != "test-key" {
						t.Errorf("x-api-key=%q", got)
					}
				} else if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("Authorization=%q", got)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if body["model"] != "test-model" || body[testCase.wantBodyKey] == nil || body[testCase.wantLimitKey] == nil || body["stream"] != true {
					t.Errorf("unexpected body: %#v", body)
				}
				if testCase.apiType == UpstreamOpenAI {
					streamOptions, _ := body["stream_options"].(map[string]any)
					if streamOptions["include_usage"] != true {
						t.Errorf("stream_options.include_usage=%#v", streamOptions["include_usage"])
					}
				}
				if testCase.apiType == UpstreamResponses {
					if body["input"] != "Tell me something" {
						t.Errorf("input=%#v", body["input"])
					}
				} else {
					messages, _ := body["messages"].([]any)
					message, _ := messages[0].(map[string]any)
					if message["content"] != "Tell me something" {
						t.Errorf("messages=%#v", messages)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, testCase.streamBody)
			}))
			defer server.Close()

			installAdminModelTestUpstream(t, "target", &UpstreamConfig{
				BaseURL: server.URL + testCase.baseSuffix,
				APIKey:  "test-key",
				APIType: testCase.apiType,
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/test-model", strings.NewReader(`{"upstream":"target","model":"test-model","prompt":"Tell me something"}`))
			adminTestModelHandler(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
				t.Fatalf("content-type=%q", contentType)
			}
			responseBody := recorder.Body.String()
			if !strings.Contains(responseBody, "Hello") || !strings.Contains(responseBody, "world") || !strings.Contains(responseBody, "data: [DONE]") {
				t.Fatalf("unexpected stream: %s", responseBody)
			}
			if !strings.Contains(responseBody, `"reasoning_content":"plan"`) {
				t.Fatalf("reasoning stream was not preserved: %s", responseBody)
			}
			if recorder.Header().Get("X-Model-Test-Protocol") != string(testCase.apiType) {
				t.Fatalf("protocol header=%q", recorder.Header().Get("X-Model-Test-Protocol"))
			}
		})
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != int64(len(tests)) || len(page.Items) != len(tests) {
		t.Fatalf("usage records=%#v, want %d records", page, len(tests))
	}
	if page.Summary.RequestCount != int64(len(tests)) || page.Summary.PromptTokens != 3 || page.Summary.CompletionTokens != 6 || page.Summary.TotalTokens != 9 {
		t.Fatalf("usage summary=%#v", page.Summary)
	}
	for _, item := range page.Items {
		if item.RequestModel != "test-model" || item.UpstreamName != "target" || item.UpstreamModel != "test-model" {
			t.Fatalf("usage identity=%#v", item)
		}
	}
}

func TestAdminTestModelReturnsUpstreamFailureDetailsWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad test key"}}`)
	}))
	defer server.Close()
	installAdminModelTestUpstream(t, "target", &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/test-model", strings.NewReader(`{"upstream":"target","model":"test-model","prompt":"hello"}`))
	adminTestModelHandler(recorder, request)

	var result struct {
		UpstreamStatus int    `json:"upstream_status"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusBadGateway || result.UpstreamStatus != http.StatusUnauthorized || result.Error != "bad test key" || requests != 1 {
		t.Fatalf("result=%+v requests=%d", result, requests)
	}
}

func TestAdminTestModelFlushesBeforeUpstreamCompletes(t *testing.T) {
	releaseUpstream := make(chan struct{}, 1)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-releaseUpstream
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstreamServer.Close()
	installAdminModelTestUpstream(t, "target", &UpstreamConfig{BaseURL: upstreamServer.URL, APIType: UpstreamOpenAI})

	adminServer := httptest.NewServer(http.HandlerFunc(adminTestModelHandler))
	defer adminServer.Close()
	defer func() {
		select {
		case releaseUpstream <- struct{}{}:
		default:
		}
	}()

	response, err := http.Post(
		adminServer.URL,
		"application/json",
		strings.NewReader(`{"upstream":"target","model":"test-model","prompt":"hello"}`),
	)
	if err != nil {
		t.Fatalf("post model test: %v", err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	firstLine, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(firstLine, "first") {
		t.Fatalf("first streamed line=%q err=%v", firstLine, err)
	}

	releaseUpstream <- struct{}{}
	rest, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(rest), "second") || !strings.Contains(string(rest), "[DONE]") {
		t.Fatalf("remaining stream=%q err=%v", rest, err)
	}
}

func TestReloadHandlerPreservesTargetModelsWhenCatalogRefreshFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "catalog unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()
	installAdminModelTestUpstream(t, "target", &UpstreamConfig{
		BaseURL:      server.URL + "/v1",
		APIType:      UpstreamOpenAI,
		CustomModels: []string{"configured-model"},
	})

	previousEffective := []ModelInfo{{ID: "effective-before-failure", Object: "model", OwnedBy: "target"}}
	previousCatalog := []ModelInfo{{ID: "catalog-before-failure", Object: "model", OwnedBy: "target"}}
	modelMu.Lock()
	oldEffective := modelsCache
	oldCatalog := upstreamModelCatalogCache
	oldEffectiveLoaded := modelsLoaded
	oldCatalogLoaded := upstreamModelCatalogLoaded
	modelsCache = append([]ModelInfo(nil), previousEffective...)
	upstreamModelCatalogCache = append([]ModelInfo(nil), previousCatalog...)
	modelsLoaded = true
	upstreamModelCatalogLoaded = true
	modelMu.Unlock()
	t.Cleanup(func() {
		modelMu.Lock()
		modelsCache = oldEffective
		upstreamModelCatalogCache = oldCatalog
		modelsLoaded = oldEffectiveLoaded
		upstreamModelCatalogLoaded = oldCatalogLoaded
		modelMu.Unlock()
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/reload?upstream=target", nil)
	reloadHandler(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}

	modelMu.RLock()
	effectiveAfter := append([]ModelInfo(nil), modelsCache...)
	catalogAfter := append([]ModelInfo(nil), upstreamModelCatalogCache...)
	effectiveLoadedAfter := modelsLoaded
	catalogLoadedAfter := upstreamModelCatalogLoaded
	modelMu.RUnlock()
	if !reflect.DeepEqual(effectiveAfter, previousEffective) || !reflect.DeepEqual(catalogAfter, previousCatalog) {
		t.Fatalf("model caches changed after failed refresh: effective=%#v catalog=%#v", effectiveAfter, catalogAfter)
	}
	if !effectiveLoadedAfter || !catalogLoadedAfter {
		t.Fatalf("model cache loaded flags changed: effective=%t catalog=%t", effectiveLoadedAfter, catalogLoadedAfter)
	}
}
