package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	matrixPublicModel   = "matrix-public-model"
	matrixUpstreamModel = "matrix-upstream-model"
	matrixText          = "matrix-ok"
)

type matrixSurface struct {
	name string
	path string
}

var matrixSurfaces = []matrixSurface{
	{name: "chat-completions", path: "/v1/chat/completions"},
	{name: "anthropic-messages", path: "/v1/messages"},
	{name: "openai-responses", path: "/v1/responses"},
}

var matrixUpstreamTypes = []UpstreamType{
	UpstreamOpenAI,
	UpstreamAnthropic,
	UpstreamResponses,
}

type matrixUpstreamCall struct {
	method  string
	path    string
	header  http.Header
	body    map[string]any
	rawBody []byte
}

type matrixUpstreamRecorder struct {
	mu    sync.Mutex
	calls []matrixUpstreamCall
}

func (r *matrixUpstreamRecorder) add(call matrixUpstreamCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *matrixUpstreamRecorder) snapshot() []matrixUpstreamCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]matrixUpstreamCall(nil), r.calls...)
}

// matrixRuntimeSnapshot 隔离生产处理器使用的进程级路由配置。
// 矩阵测试绝不并行运行，即使子测试失败，清理逻辑也会还原此处修改的所有全局变量。
type matrixRuntimeSnapshot struct {
	modelAlias          map[string]ModelAlias
	upstreamCfg         *UpstreamConfig
	upstreamCfgs        map[string]*UpstreamConfig
	defaultUpstreamName string
	webSearchCfg        WebSearchConfig

	socks5Proxies     []Socks5Proxy
	activeSocks5      string
	socks5ClientCache map[socks5ClientCacheKey]*http.Client
	socks5RRIndex     uint32
	socks5RateIndex   uint32
}

func matrixIsolateRuntime(t *testing.T) {
	t.Helper()
	resetHostedWebSearchCapabilityCache()

	configMu.Lock()
	snapshot := matrixRuntimeSnapshot{
		modelAlias:          modelAlias,
		upstreamCfg:         upstreamCfg,
		upstreamCfgs:        upstreamCfgs,
		defaultUpstreamName: defaultUpstreamName,
		webSearchCfg:        webSearchCfg,
	}
	webSearchCfg = WebSearchConfig{}
	configMu.Unlock()

	socks5Mu.Lock()
	snapshot.socks5Proxies = socks5Proxies
	snapshot.activeSocks5 = activeSocks5
	snapshot.socks5ClientCache = socks5ClientCache
	snapshot.socks5RRIndex = atomic.LoadUint32(&socks5RRIndex)
	snapshot.socks5RateIndex = atomic.LoadUint32(&socks5RateLimitIndex)
	socks5Proxies = nil
	activeSocks5 = ""
	socks5ClientCache = map[socks5ClientCacheKey]*http.Client{}
	atomic.StoreUint32(&socks5RRIndex, 0)
	atomic.StoreUint32(&socks5RateLimitIndex, 0)
	socks5Mu.Unlock()

	t.Cleanup(func() {
		resetHostedWebSearchCapabilityCache()
		configMu.Lock()
		modelAlias = snapshot.modelAlias
		upstreamCfg = snapshot.upstreamCfg
		upstreamCfgs = snapshot.upstreamCfgs
		defaultUpstreamName = snapshot.defaultUpstreamName
		webSearchCfg = snapshot.webSearchCfg
		configMu.Unlock()

		socks5Mu.Lock()
		closeSocks5ClientsLocked()
		socks5Proxies = snapshot.socks5Proxies
		activeSocks5 = snapshot.activeSocks5
		socks5ClientCache = snapshot.socks5ClientCache
		atomic.StoreUint32(&socks5RRIndex, snapshot.socks5RRIndex)
		atomic.StoreUint32(&socks5RateLimitIndex, snapshot.socks5RateIndex)
		socks5Mu.Unlock()
	})
}

func matrixSelectUpstream(baseURL string, apiType UpstreamType) {
	zeroRetries := 0
	cfg := &UpstreamConfig{
		BaseURL:    baseURL + "/v1",
		APIKey:     "matrix-api-key",
		APIType:    apiType,
		MaxRetries: &zeroRetries,
	}

	configMu.Lock()
	defer configMu.Unlock()
	modelAlias = map[string]ModelAlias{
		matrixPublicModel: {
			TargetModel: matrixUpstreamModel,
			Upstream:    "matrix",
		},
	}
	upstreamCfgs = map[string]*UpstreamConfig{"matrix": cfg}
	defaultUpstreamName = "matrix"
	upstreamCfg = cloneUpstreamConfig(cfg)
}

func matrixGatewayServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", chatCompletionsHandler)
	mux.HandleFunc("/v1/messages", claudeMessagesHandler)
	mux.HandleFunc("/v1/responses", responsesHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func matrixMockUpstream(t *testing.T, apiType UpstreamType) (*httptest.Server, *matrixUpstreamRecorder) {
	t.Helper()
	recorder := &matrixUpstreamRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		recorder.add(matrixUpstreamCall{
			method:  req.Method,
			path:    req.URL.Path,
			header:  req.Header.Clone(),
			body:    decoded,
			rawBody: append([]byte(nil), body...),
		})

		stream, _ := decoded["stream"].(bool)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, matrixMockStream(apiType))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, matrixMockJSON(apiType))
	}))
	t.Cleanup(server.Close)
	return server, recorder
}

func matrixMockJSON(apiType UpstreamType) string {
	switch apiType {
	case UpstreamAnthropic:
		return `{"id":"msg_matrix","type":"message","role":"assistant","model":"matrix-upstream-model","content":[{"type":"text","text":"matrix-ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`
	case UpstreamResponses:
		return `{"id":"resp_matrix","object":"response","created_at":1710000000,"status":"completed","background":false,"error":null,"incomplete_details":null,"model":"matrix-upstream-model","output":[{"id":"msg_matrix","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"matrix-ok","annotations":[],"logprobs":[]}]}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`
	default:
		return `{"id":"chatcmpl-matrix","object":"chat.completion","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"matrix-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	}
}

func matrixMockStream(apiType UpstreamType) string {
	switch apiType {
	case UpstreamAnthropic:
		return strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_matrix","type":"message","role":"assistant","model":"matrix-upstream-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"matrix-ok"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":0}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
	case UpstreamResponses:
		response := `{"id":"resp_matrix","object":"response","created_at":1710000000,"status":"completed","background":false,"error":null,"incomplete_details":null,"model":"matrix-upstream-model","output":[{"id":"msg_matrix","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"matrix-ok","annotations":[],"logprobs":[]}]}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`
		return strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_matrix","object":"response","created_at":1710000000,"status":"in_progress","output":[]}}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_matrix","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
			``,
			`event: response.content_part.added`,
			`data: {"type":"response.content_part.added","sequence_number":2,"item_id":"msg_matrix","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[],"logprobs":[]}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_matrix","output_index":0,"content_index":0,"delta":"matrix-ok","logprobs":[]}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","sequence_number":4,"response":` + response + `}`,
			``,
		}, "\n")
	default:
		return strings.Join([]string{
			`data: {"id":"chatcmpl-matrix","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-matrix","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"content":"matrix-ok"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-matrix","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
	}
}

func matrixClientRequestBody(surface matrixSurface, stream bool) []byte {
	var request map[string]any
	switch surface.path {
	case "/v1/messages":
		request = map[string]any{
			"model":      matrixPublicModel,
			"max_tokens": 32,
			"messages": []any{
				map[string]any{"role": "user", "content": "hello from matrix"},
			},
		}
	case "/v1/responses":
		request = map[string]any{
			"model": matrixPublicModel,
			"input": "hello from matrix",
		}
	default:
		request = map[string]any{
			"model": matrixPublicModel,
			"messages": []any{
				map[string]any{"role": "user", "content": "hello from matrix"},
			},
		}
	}
	if stream {
		request["stream"] = true
	}
	body, _ := json.Marshal(request)
	return body
}

func matrixDoRequest(t *testing.T, gateway *httptest.Server, surface matrixSurface, stream bool) ([]byte, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, gateway.URL+surface.path, strings.NewReader(string(matrixClientRequestBody(surface, stream))))
	if err != nil {
		t.Fatalf("build gateway request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := gateway.Client().Do(req)
	if err != nil {
		t.Fatalf("call gateway: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read gateway response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gateway status = %d, want 200\nbody: %s", resp.StatusCode, body)
	}
	return body, resp.Header.Clone()
}

func matrixReasoningRequestBody(surface matrixSurface, model string) []byte {
	var request map[string]any
	switch surface.path {
	case "/v1/messages":
		request = map[string]any{
			"model": model, "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": "reason"}},
			"thinking": map[string]any{"type": "enabled", "budget_tokens": 4096},
		}
	case "/v1/responses":
		request = map[string]any{
			"model": model, "input": "reason",
			"reasoning": map[string]any{"effort": "high"},
		}
	default:
		request = map[string]any{
			"model":    model,
			"messages": []any{map[string]any{"role": "user", "content": "reason"}},
			"thinking": map[string]any{"type": "adaptive"}, "reasoning_effort": "high",
		}
	}
	body, _ := json.Marshal(request)
	return body
}

func matrixPostRequest(t *testing.T, gateway *httptest.Server, surface matrixSurface, body []byte) {
	t.Helper()
	response, err := gateway.Client().Post(gateway.URL+surface.path, "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("gateway status=%d body=%s", response.StatusCode, responseBody)
	}
}

func matrixAssertUpstreamCall(t *testing.T, recorder *matrixUpstreamRecorder, apiType UpstreamType, stream bool) {
	t.Helper()
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls = %d, want 1: %#v", len(calls), calls)
	}
	call := calls[0]
	if call.method != http.MethodPost {
		t.Errorf("upstream method = %q, want POST", call.method)
	}

	wantPath := map[UpstreamType]string{
		UpstreamOpenAI:    "/v1/chat/completions",
		UpstreamAnthropic: "/v1/messages",
		UpstreamResponses: "/v1/responses",
	}[apiType]
	if call.path != wantPath {
		t.Errorf("upstream path = %q, want %q", call.path, wantPath)
	}
	if call.body["model"] != matrixUpstreamModel {
		t.Errorf("upstream model = %#v, want %q\nbody: %s", call.body["model"], matrixUpstreamModel, call.rawBody)
	}
	gotStream, _ := call.body["stream"].(bool)
	if gotStream != stream {
		t.Errorf("upstream stream = %t, want %t\nbody: %s", gotStream, stream, call.rawBody)
	}

	switch apiType {
	case UpstreamResponses:
		if _, ok := call.body["input"]; !ok {
			t.Errorf("Responses upstream request has no input: %s", call.rawBody)
		}
		if _, leakedChatShape := call.body["messages"]; leakedChatShape {
			t.Errorf("Responses upstream request leaked Chat messages pivot: %s", call.rawBody)
		}
		if got := call.header.Get("Authorization"); got != "Bearer matrix-api-key" {
			t.Errorf("Responses Authorization = %q", got)
		}
	case UpstreamAnthropic:
		if _, ok := call.body["messages"].([]any); !ok {
			t.Errorf("Anthropic upstream messages has type %T: %s", call.body["messages"], call.rawBody)
		}
		if _, ok := call.body["max_tokens"]; !ok {
			t.Errorf("Anthropic upstream request has no max_tokens: %s", call.rawBody)
		}
		if _, leakedResponsesShape := call.body["input"]; leakedResponsesShape {
			t.Errorf("Anthropic upstream request leaked Responses input shape: %s", call.rawBody)
		}
		if got := call.header.Get("x-api-key"); got != "matrix-api-key" {
			t.Errorf("Anthropic x-api-key = %q", got)
		}
		if got := call.header.Get("Authorization"); got != "Bearer matrix-api-key" {
			t.Errorf("Anthropic compatibility Authorization = %q", got)
		}
		if got := call.header.Get("anthropic-version"); got == "" {
			t.Error("Anthropic request has no anthropic-version header")
		}
	default:
		if _, ok := call.body["messages"].([]any); !ok {
			t.Errorf("Chat upstream messages has type %T: %s", call.body["messages"], call.rawBody)
		}
		if got := call.header.Get("Authorization"); got != "Bearer matrix-api-key" {
			t.Errorf("Chat Authorization = %q", got)
		}
	}
}

func matrixDecodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatalf("decode response JSON: %v\nbody: %s", err, body)
	}
	return object
}

func matrixArray(t *testing.T, value any, path string) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok || len(array) == 0 {
		t.Fatalf("%s = %#v, want non-empty array", path, value)
	}
	return array
}

func matrixObject(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", path, value)
	}
	return object
}

func matrixAssertNonStreamShape(t *testing.T, surface matrixSurface, body []byte, header http.Header) {
	t.Helper()
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	object := matrixDecodeObject(t, body)

	switch surface.path {
	case "/v1/messages":
		if object["type"] != "message" || object["role"] != "assistant" {
			t.Fatalf("Anthropic response envelope is invalid: %s", body)
		}
		content := matrixArray(t, object["content"], "content")
		text := matrixObject(t, content[0], "content[0]")
		if text["type"] != "text" || text["text"] != matrixText {
			t.Fatalf("Anthropic text block is invalid: %#v", text)
		}
	case "/v1/responses":
		if object["object"] != "response" || object["status"] != "completed" {
			t.Fatalf("Responses envelope is invalid: %s", body)
		}
		// 非流式 Responses 调用必须返回 Response 对象本身，
		// 绝不能返回 response.completed SSE 信封。
		if _, wrapped := object["response"]; wrapped || object["type"] == "response.completed" {
			t.Fatalf("non-streaming Responses result is event-wrapped: %s", body)
		}
		output := matrixArray(t, object["output"], "output")
		message := matrixObject(t, output[0], "output[0]")
		content := matrixArray(t, message["content"], "output[0].content")
		text := matrixObject(t, content[0], "output[0].content[0]")
		if text["type"] != "output_text" || text["text"] != matrixText {
			t.Fatalf("Responses output text is invalid: %#v", text)
		}
	default:
		if object["object"] != "chat.completion" {
			t.Fatalf("Chat response object = %#v, want chat.completion\nbody: %s", object["object"], body)
		}
		choices := matrixArray(t, object["choices"], "choices")
		choice := matrixObject(t, choices[0], "choices[0]")
		message := matrixObject(t, choice["message"], "choices[0].message")
		if message["role"] != "assistant" || message["content"] != matrixText {
			t.Fatalf("Chat assistant message is invalid: %#v", message)
		}
	}
}

func matrixAssertBridgePath(t *testing.T, surface matrixSurface, apiType UpstreamType, header http.Header) {
	t.Helper()
	client := clientProtocolFromPath(surface.path)
	upstream := &UpstreamConfig{APIType: apiType}
	want := decideProtocolBridge(client, upstream, BridgeModeCompatible).Path
	if got := BridgePath(header.Get("X-Llm2api-Bridge-Path")); got != want {
		t.Errorf("bridge path header=%q want=%q for %s -> %s", got, want, client, wireProtocolFromUpstream(apiType))
	}
}

func matrixAssertStreamShape(t *testing.T, surface matrixSurface, body []byte, header http.Header) {
	t.Helper()
	if contentType := header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", contentType)
	}
	text := string(body)
	if !strings.Contains(text, matrixText) {
		t.Fatalf("stream does not contain %q:\n%s", matrixText, text)
	}

	var required []string
	switch surface.path {
	case "/v1/messages":
		required = []string{
			"event: message_start",
			`"type":"content_block_delta"`,
			"event: message_stop",
		}
	case "/v1/responses":
		required = []string{
			"event: response.created",
			`"type":"response.output_text.delta"`,
			"event: response.completed",
		}
	default:
		required = []string{
			`"object":"chat.completion.chunk"`,
			"data: [DONE]",
		}
	}
	for _, marker := range required {
		if !strings.Contains(text, marker) {
			t.Errorf("stream is missing %q:\n%s", marker, text)
		}
	}
}

func TestProtocolMatrixNonStreaming(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)

	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			name := fmt.Sprintf("%s_to_%s", surface.name, apiType)
			t.Run(name, func(t *testing.T) {
				upstream, recorder := matrixMockUpstream(t, apiType)
				matrixSelectUpstream(upstream.URL, apiType)

				body, header := matrixDoRequest(t, gateway, surface, false)
				matrixAssertUpstreamCall(t, recorder, apiType, false)
				matrixAssertBridgePath(t, surface, apiType, header)
				matrixAssertNonStreamShape(t, surface, body, header)
			})
		}
	}
}

func TestProtocolMatrixStreaming(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)

	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			name := fmt.Sprintf("%s_to_%s", surface.name, apiType)
			t.Run(name, func(t *testing.T) {
				upstream, recorder := matrixMockUpstream(t, apiType)
				matrixSelectUpstream(upstream.URL, apiType)

				body, header := matrixDoRequest(t, gateway, surface, true)
				matrixAssertUpstreamCall(t, recorder, apiType, true)
				matrixAssertBridgePath(t, surface, apiType, header)
				matrixAssertStreamShape(t, surface, body, header)
			})
		}
	}
}

func TestReasoningDisabledAliasIsConsistentAcrossProtocolMatrix(t *testing.T) {
	for _, surface := range matrixSurfaces {
		for _, upstreamType := range matrixUpstreamTypes {
			t.Run(surface.name+"_to_"+string(upstreamType), func(t *testing.T) {
				matrixIsolateRuntime(t)
				upstream, recorder := matrixMockUpstream(t, upstreamType)
				// matrixSelectUpstream 配置的别名默认 WithReasoning=false。
				matrixSelectUpstream(upstream.URL, upstreamType)
				gateway := matrixGatewayServer(t)

				matrixPostRequest(t, gateway, surface, matrixReasoningRequestBody(surface, matrixPublicModel))
				calls := recorder.snapshot()
				if len(calls) != 1 {
					t.Fatalf("upstream calls=%d, want 1", len(calls))
				}
				for _, key := range []string{"thinking", "reasoning", "reasoning_effort"} {
					if value, exists := calls[0].body[key]; exists {
						t.Errorf("disabled reasoning leaked %s=%#v to %s upstream", key, value, upstreamType)
					}
				}
			})
		}
	}
}

func TestDirectModelsPreserveExplicitReasoningOnNativeProtocols(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		surface      matrixSurface
		upstreamType UpstreamType
		wantKey      string
	}{
		{name: "chat", surface: matrixSurfaces[0], upstreamType: UpstreamOpenAI, wantKey: "reasoning_effort"},
		{name: "anthropic", surface: matrixSurfaces[1], upstreamType: UpstreamAnthropic, wantKey: "thinking"},
		{name: "responses", surface: matrixSurfaces[2], upstreamType: UpstreamResponses, wantKey: "reasoning"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			matrixIsolateRuntime(t)
			upstream, recorder := matrixMockUpstream(t, testCase.upstreamType)
			matrixSelectUpstream(upstream.URL, testCase.upstreamType)
			configMu.Lock()
			modelAlias = map[string]ModelAlias{}
			upstreamCfgs["matrix"].CustomModels = nil
			upstreamCfg = cloneUpstreamConfig(upstreamCfgs["matrix"])
			configMu.Unlock()
			gateway := matrixGatewayServer(t)

			matrixPostRequest(t, gateway, testCase.surface, matrixReasoningRequestBody(testCase.surface, matrixPublicModel))
			calls := recorder.snapshot()
			if len(calls) != 1 {
				t.Fatalf("upstream calls=%d, want 1", len(calls))
			}
			if _, exists := calls[0].body[testCase.wantKey]; !exists {
				t.Fatalf("direct model lost explicit %s: %s", testCase.wantKey, calls[0].rawBody)
			}
		})
	}
}
