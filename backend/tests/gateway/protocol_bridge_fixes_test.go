package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func warningByCode(warnings []BridgeWarning, code string) (BridgeWarning, bool) {
	for _, warning := range warnings {
		if warning.Code == code {
			return warning, true
		}
	}
	return BridgeWarning{}, false
}

func TestSSEHeadersDisableProxyBuffering(t *testing.T) {
	header := http.Header{}
	setSSEHeaders(header)
	if header.Get("Content-Type") != "text/event-stream" || header.Get("Cache-Control") != "no-cache" || header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("SSE headers=%v", header)
	}
}

func TestResponsesChatAutomaticallyForwardsStandardFields(t *testing.T) {
	matrixIsolateRuntime(t)
	var upstreamBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl_caps","created":1,"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	matrixSelectUpstream(server.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"hello",
		"text":{"verbosity":"low","format":{"type":"json_object"}},
		"prompt_cache_key":"cache-1","prompt_cache_options":{"ttl":"30m"},"prompt_cache_retention":"24h",
		"service_tier":"priority","safety_identifier":"user-hash","moderation":{"type":"auto"},"top_logprobs":3,
		"tools":[{"type":"web_search","search_context_size":"high"}]
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if upstreamBody["verbosity"] != "low" || upstreamBody["prompt_cache_key"] != "cache-1" || upstreamBody["prompt_cache_options"] == nil || upstreamBody["prompt_cache_retention"] != "24h" {
		t.Fatalf("optional fields not forwarded: %#v", upstreamBody)
	}
	for _, field := range []string{"service_tier", "safety_identifier", "moderation", "top_logprobs"} {
		if upstreamBody[field] == nil {
			t.Fatalf("standard Chat field %q not forwarded: %#v", field, upstreamBody)
		}
	}
	if upstreamBody["logprobs"] != true {
		t.Fatalf("top_logprobs did not enable Chat logprobs: %#v", upstreamBody)
	}
	format := testObject(t, upstreamBody["response_format"], "response_format")
	requireTestEqual(t, "response format", format["type"], "json_object")
	webSearchOptions := testObject(t, upstreamBody["web_search_options"], "web_search_options")
	requireTestEqual(t, "search context size", webSearchOptions["search_context_size"], "high")
	if response.Header().Get("X-Llm2api-Warning-Count") != "" {
		t.Fatalf("supported fields unexpectedly warned: %#v", response.Header())
	}
}

func TestResponsesChatAutomaticallyRetriesWithoutRejectedOptionalFields(t *testing.T) {
	matrixIsolateRuntime(t)
	calls := 0
	var retryBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		if calls == 1 {
			if request["verbosity"] != "low" || request["prompt_cache_key"] != "cache-1" {
				t.Errorf("first request did not try standard fields: %#v", request)
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"unknown field verbosity"}}`)
			return
		}
		retryBody = request
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl_retry","created":1,"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()
	matrixSelectUpstream(server.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"hello","text":{"verbosity":"low"},
		"prompt_cache_key":"cache-1","prompt_cache_retention":"24h"
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK || calls != 2 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	for _, field := range []string{"verbosity", "prompt_cache_key", "prompt_cache_retention"} {
		if _, exists := retryBody[field]; exists {
			t.Fatalf("retry retained rejected field %q: %#v", field, retryBody)
		}
	}
	if response.Header().Get("X-Llm2api-Warning-Count") != "3" {
		t.Fatalf("automatic downgrade warning headers=%#v", response.Header())
	}
	converted := decodeTestObject(t, response.Body.Bytes())
	warnings := testArray(t, converted["llm2api_warnings"], "llm2api_warnings")
	if len(warnings) != 3 {
		t.Fatalf("automatic downgrade warnings=%#v", warnings)
	}
}

func TestResponsesChatStreamAutomaticallyRetriesWithoutRejectedOptionalFields(t *testing.T) {
	matrixIsolateRuntime(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}
		if calls == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"error":{"message":"unsupported prompt_cache_key"}}`)
			return
		}
		if _, exists := request["prompt_cache_key"]; exists {
			t.Errorf("stream retry retained prompt_cache_key: %#v", request)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_stream_retry\",\"created\":1,\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	matrixSelectUpstream(server.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"hello","stream":true,"prompt_cache_key":"cache-1"
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK || calls != 2 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "event: response.completed") || !strings.Contains(response.Body.String(), "chat_option_auto_downgraded") {
		t.Fatalf("stream retry response is incomplete: %s", response.Body.String())
	}
}

func TestResponsesStandardChatFieldsOnlyWarnForNonChatUpstream(t *testing.T) {
	fields := map[string]any{
		"text":             map[string]any{"verbosity": "high"},
		"prompt_cache_key": "cache-1", "prompt_cache_options": map[string]any{"ttl": "30m"}, "prompt_cache_retention": "24h",
		"service_tier": "priority", "safety_identifier": "user-hash", "moderation": map[string]any{"type": "auto"}, "top_logprobs": float64(3),
	}
	if warnings := responsesBridgeRequestWarningsForUpstream(fields, &UpstreamConfig{APIType: UpstreamOpenAI}); len(warnings) != 0 {
		t.Fatalf("standard Chat fields unexpectedly warned: %#v", warnings)
	}
	if warnings := responsesBridgeRequestWarningsForUpstream(fields, &UpstreamConfig{APIType: UpstreamResponses}); len(warnings) != 0 {
		t.Fatalf("native Responses fields unexpectedly warned: %#v", warnings)
	}
	warnings := responsesBridgeRequestWarningsForUpstream(fields, &UpstreamConfig{APIType: UpstreamAnthropic})
	cacheWarnings := 0
	for _, warning := range warnings {
		if warning.Code == "prompt_cache_hint_ignored" {
			cacheWarnings++
		}
	}
	if cacheWarnings != 3 || len(warnings) != 8 {
		t.Fatalf("cache warnings=%d, all=%#v", cacheWarnings, warnings)
	}
}

func TestResponsesIncludeWarningsArePerItem(t *testing.T) {
	warnings := responsesBridgeRequestWarnings(map[string]any{"include": []any{
		"reasoning.encrypted_content", "web_search_call.action.sources",
	}})
	if len(warnings) != 2 {
		t.Fatalf("warnings=%#v", warnings)
	}
	if warnings[0].Code != "encrypted_reasoning_unavailable" || warnings[0].Path != "include[0]" || warnings[0].Severity != "degraded" {
		t.Fatalf("encrypted warning=%#v", warnings[0])
	}
	if warnings[1].Code != "output_hint_ignored" || warnings[1].Path != "include[1]" || warnings[1].Severity != "info" {
		t.Fatalf("output warning=%#v", warnings[1])
	}
}

func TestAnthropicReasoningEffortMappingAndBudgetApproximation(t *testing.T) {
	tests := []struct {
		thinking any
		output   any
		want     string
	}{
		{map[string]any{"type": "adaptive"}, nil, "high"},
		{map[string]any{"type": "adaptive"}, map[string]any{"effort": "low"}, "low"},
		{map[string]any{"type": "adaptive"}, map[string]any{"effort": "max"}, "max"},
		{nil, map[string]any{"effort": "medium"}, "medium"},
		{map[string]any{"type": "enabled", "budget_tokens": 4000}, nil, "low"},
		{map[string]any{"type": "enabled", "budget_tokens": 16000}, nil, "medium"},
		{map[string]any{"type": "enabled", "budget_tokens": 32000}, nil, "high"},
		{map[string]any{"type": "enabled", "budget_tokens": 32001}, nil, "xhigh"},
		{map[string]any{"type": "enabled"}, nil, "high"},
	}
	for _, testCase := range tests {
		if got := reasoningEffortFromAnthropic(testCase.thinking, testCase.output); got != testCase.want {
			t.Fatalf("thinking=%#v output=%#v effort=%q, want %q", testCase.thinking, testCase.output, got, testCase.want)
		}
	}
	if got := reasoningEffortToAnthropicThinking("adaptive"); got["type"] != "adaptive" {
		t.Fatalf("adaptive reverse mapping=%#v", got)
	}
	if got := reasoningEffortToAnthropicThinking("none"); got != nil {
		t.Fatalf("none reverse mapping=%#v, want nil", got)
	}

	body := []byte(`{"messages":[{"role":"user","content":"hello"}],"thinking":{"type":"enabled","budget_tokens":12345}}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "model", true)
	if err != nil {
		t.Fatal(err)
	}
	if warning, ok := warningByCode(warnings, "thinking_budget_approximated"); !ok || warning.Path != "thinking.budget_tokens" || !strings.Contains(warning.Message, "12345") {
		t.Fatalf("approximation warnings=%#v", warnings)
	}
	if strings.Contains(string(converted), `"max_tokens":12345`) {
		t.Fatalf("non-standard max_tokens forwarded: %s", converted)
	}
}

func TestServiceTierMappingsStayProtocolValid(t *testing.T) {
	converted, warnings, err := convertAnthropicRequestToResponsesDirect([]byte(`{
		"messages":[{"role":"user","content":"hello"}],"service_tier":"standard_only"
	}`), "model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("exact service tier mapping warned: %#v", warnings)
	}
	request := decodeTestObject(t, converted)
	requireTestEqual(t, "Responses service tier", request["service_tier"], "default")

	chat := decodeTestObject(t, openAIToAnthropicRequest([]byte(`{
		"model":"claude-sonnet-5","messages":[{"role":"user","content":"hello"}],"service_tier":"default"
	}`)))
	requireTestEqual(t, "Anthropic service tier", chat["service_tier"], "standard_only")

	warnings = chatBridgeWarnings(&OpenAIRequest{AdditionalFields: map[string]any{"service_tier": "priority"}}, &UpstreamConfig{APIType: UpstreamAnthropic})
	if warning, ok := warningByCode(warnings, "service_tier_approximated"); !ok || warning.Path != "service_tier" {
		t.Fatalf("service tier approximation warnings=%#v", warnings)
	}
}

func TestCurrentAnthropicModelsUseAdaptiveThinking(t *testing.T) {
	converted := decodeTestObject(t, openAIToAnthropicRequest([]byte(`{
		"model":"claude-opus-4-8","messages":[{"role":"user","content":"hello"}],
		"max_tokens":4096,"reasoning_effort":"xhigh"
	}`)))
	requireTestEqual(t, "thinking type", testObject(t, converted["thinking"], "thinking")["type"], "adaptive")
	requireTestEqual(t, "output effort", testObject(t, converted["output_config"], "output_config")["effort"], "xhigh")
	if thinking := testObject(t, converted["thinking"], "thinking"); thinking["budget_tokens"] != nil {
		t.Fatalf("adaptive request retained manual budget: %#v", thinking)
	}

	direct, _, warnings, err := convertResponsesRequestToAnthropicDirect([]byte(`{
		"input":"hello","reasoning":{"effort":"max"}
	}`), "claude-opus-4-8", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("exact adaptive mapping warned: %#v", warnings)
	}
	directRequest := decodeTestObject(t, direct)
	requireTestEqual(t, "direct thinking type", testObject(t, directRequest["thinking"], "thinking")["type"], "adaptive")
	requireTestEqual(t, "direct output effort", testObject(t, directRequest["output_config"], "output_config")["effort"], "max")
}

func TestCustomToolFormatAndToolSearchDegradation(t *testing.T) {
	converted, mappings, warnings := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{
		{Type: "custom", Name: "apply_patch", Format: map[string]any{"type": "grammar", "syntax": "lark"}},
		{Type: "tool_search", Execution: "client"},
	})
	if warning, ok := warningByCode(warnings, "custom_tool_format_not_enforced"); !ok || warning.Severity != "degraded" || warning.Path != "tools[0].format" {
		t.Fatalf("custom format warnings=%#v", warnings)
	}
	if warning, ok := warningByCode(warnings, "tool_search_emulated"); !ok || warning.Severity != "degraded" || !strings.Contains(warning.Message, "does not execute") {
		t.Fatalf("tool search warnings=%#v", warnings)
	}
	customName := responseUpstreamToolName("custom", "", "apply_patch", mappings)
	if mappings[customName].Format == nil {
		t.Fatalf("custom format mapping was lost: %#v", mappings)
	}
	searchName := responseUpstreamToolName("tool_search", "", "tool_search", mappings)
	if mappings[searchName].Execution != "client" {
		t.Fatalf("tool search execution mapping=%#v", mappings[searchName])
	}
	searchParameters := converted[1].Function.Parameters
	properties := testObject(t, searchParameters["properties"], "tool search properties")
	if properties["goal"] == nil || properties["query"] != nil {
		t.Fatalf("default tool search schema=%#v", searchParameters)
	}
}

func TestAdditionalToolsAreHoistedForProtocolBridges(t *testing.T) {
	input := []any{map[string]any{
		"type": "additional_tools",
		"role": "developer",
		"tools": []any{map[string]any{
			"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object"},
		}},
	}}
	loaded := responsesLoadedToolDefinitions(input)
	if len(loaded) != 1 || loaded[0].Name != "lookup" {
		t.Fatalf("additional tools=%#v", loaded)
	}
	_, mappings, _ := convertResponsesToolsWithMappingsDetailed(loaded)
	_, warnings := responsesInputToMessagesWithWarnings(input, "", mappings)
	if _, ok := warningByCode(warnings, "additional_tools_hoisted"); !ok {
		t.Fatalf("Chat bridge warnings=%#v", warnings)
	}

	requestBody, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		t.Fatal(err)
	}
	body, _, warnings, err := convertResponsesRequestToAnthropicDirect(requestBody, "claude-sonnet-5", true)
	if err != nil {
		t.Fatal(err)
	}
	converted := decodeTestObject(t, body)
	if len(testArray(t, converted["tools"], "Anthropic tools")) != 1 {
		t.Fatalf("Anthropic tools=%#v", converted["tools"])
	}
	if _, ok := warningByCode(warnings, "additional_tools_hoisted"); !ok {
		t.Fatalf("Anthropic bridge warnings=%#v", warnings)
	}
}

func TestHostedToolSearchHistoryDoesNotCreateInvalidAnthropicBlocks(t *testing.T) {
	input := []any{
		map[string]any{"type": "tool_search_call", "execution": "server", "call_id": nil, "status": "completed", "arguments": map[string]any{"paths": []any{"crm"}}},
		map[string]any{"type": "tool_search_output", "execution": "server", "call_id": nil, "status": "completed", "tools": []any{
			map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object"}},
		}},
		map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "continue"}}},
	}
	requestBody, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		t.Fatal(err)
	}
	body, _, warnings, err := convertResponsesRequestToAnthropicDirect(requestBody, "claude-sonnet-5", true)
	if err != nil {
		t.Fatal(err)
	}
	converted := decodeTestObject(t, body)
	if len(testArray(t, converted["tools"], "Anthropic tools")) != 1 {
		t.Fatalf("Anthropic tools=%#v", converted["tools"])
	}
	encodedMessages, _ := json.Marshal(converted["messages"])
	if strings.Contains(string(encodedMessages), "tool_use") || strings.Contains(string(encodedMessages), "tool_result") {
		t.Fatalf("hosted search produced invalid Anthropic history: %s", encodedMessages)
	}
	if _, ok := warningByCode(warnings, "hosted_tool_search_history_omitted"); !ok {
		t.Fatalf("warnings=%#v", warnings)
	}
}

func TestCustomToolStreamEmitsInputDone(t *testing.T) {
	_, mappings, _ := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{Type: "custom", Name: "apply_patch"}})
	name := responseUpstreamToolName("custom", "", "apply_patch", mappings)
	stream := "data: {\"id\":\"chatcmpl_1\",\"created\":1,\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"" + name + "\",\"arguments\":\"{\\\"input\\\":\\\"patch\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	recorder := httptest.NewRecorder()
	responsesStreamHandler(recorder, nil, response, "model", "model", nil, nil, nil, mappings, nil)
	body := recorder.Body.String()
	if !strings.Contains(body, "event: response.custom_tool_call_input.done") || !strings.Contains(body, `"input":"patch"`) {
		t.Fatalf("custom input done event missing: %s", body)
	}
	if strings.Contains(body, "event: response.function_call_arguments.done") {
		t.Fatalf("custom call emitted function arguments done event: %s", body)
	}
}

func TestChatHostedWebSearchAutomaticMappingAndAnnotations(t *testing.T) {
	upstream := &UpstreamConfig{APIType: UpstreamOpenAI}
	resetHostedWebSearchCapabilityCache()
	if shouldUseGatewayWebSearchFallback(upstream, WebSearchConfig{Enabled: true}) {
		t.Fatal("unknown Chat upstream should try native search first")
	}
	upstream.BaseURL = "https://chat.example/v1"
	markHostedWebSearchUnsupported(upstream)
	if !shouldUseGatewayWebSearchFallback(upstream, WebSearchConfig{Enabled: true}) {
		t.Fatal("unsupported Chat search capability was not cached")
	}
	options := responsesWebSearchOptionsForChat([]ResponsesTool{{
		Type: "web_search", SearchContextSize: "high", UserLocation: map[string]any{"type": "approximate", "country": "CN"},
	}})
	if options["search_context_size"] != "high" || options["user_location"] == nil {
		t.Fatalf("web search options=%#v", options)
	}
	warnings := responsesWebSearchToChatWarnings([]ResponsesTool{
		{Type: "web_search", Filters: map[string]any{"allowed_domains": []any{"example.com"}}},
		{Type: "web_search_preview"},
	})
	if len(warnings) != 2 || warnings[0].Code != "web_search_option_ignored" || warnings[1].Code != "multiple_hosted_search_tools" {
		t.Fatalf("web search mapping warnings=%#v", warnings)
	}

	chatBody := []byte(`{"id":"chatcmpl_web","created":1,"choices":[{"finish_reason":"stop","message":{"content":"answer","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"Example","start_index":0,"end_index":6}}]}}]}`)
	converted := decodeTestObject(t, convertChatToResponsesObject(chatBody, "model", nil, nil, nil, nil))
	message := testObject(t, testArray(t, converted["output"], "output")[0], "message")
	part := testObject(t, testArray(t, message["content"], "content")[0], "content part")
	annotations := testArray(t, part["annotations"], "annotations")
	if len(annotations) != 1 {
		t.Fatalf("annotations=%#v", annotations)
	}
}

func TestChatAnnotationsSurviveStreamingTerminalObject(t *testing.T) {
	stream := "data: {\"id\":\"chatcmpl_web\",\"created\":1,\"service_tier\":\"priority\",\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl_web\",\"choices\":[{\"delta\":{\"annotations\":[{\"type\":\"url_citation\",\"url_citation\":{\"url\":\"https://example.com\"}}]},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	recorder := httptest.NewRecorder()
	responsesStreamHandler(recorder, nil, response, "model", "model", nil, nil, nil, nil, nil)
	body := recorder.Body.String()
	if !strings.Contains(body, "event: response.output_text.annotation.added") || !strings.Contains(body, `"annotations":[{"type":"url_citation"`) || !strings.Contains(body, "https://example.com") || !strings.Contains(body, `"service_tier":"priority"`) {
		t.Fatalf("stream terminal annotations missing: %s", body)
	}
	terminal := body[strings.LastIndex(body, "event: response.completed"):]
	if !strings.Contains(terminal, `"sequence_number":`) {
		t.Fatalf("terminal Responses event has no sequence number: %s", body)
	}
}

func TestChatRefusalSurvivesResponsesStream(t *testing.T) {
	stream := "data: {\"id\":\"chatcmpl_refusal\",\"created\":1,\"choices\":[{\"delta\":{\"refusal\":\"cannot comply\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	recorder := httptest.NewRecorder()
	responsesStreamHandler(recorder, nil, response, "model", "model", nil, nil, nil, nil, nil)
	body := recorder.Body.String()
	for _, expected := range []string{
		"event: response.refusal.delta", "event: response.refusal.done",
		`"refusal":"cannot comply","type":"refusal"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("refusal stream missing %q: %s", expected, body)
		}
	}
}

func TestChatLogprobsSurviveResponsesConversion(t *testing.T) {
	chatBody := []byte(`{
		"id":"chatcmpl_logprobs","choices":[{
			"finish_reason":"stop","message":{"content":"A"},
			"logprobs":{"content":[{"token":"A","logprob":-0.1,"bytes":[65],"top_logprobs":[]}]}
		}]
	}`)
	converted := decodeTestObject(t, convertChatToResponsesObject(chatBody, "model", nil, nil, nil, nil))
	message := testObject(t, testArray(t, converted["output"], "output")[0], "message")
	part := testObject(t, testArray(t, message["content"], "content")[0], "content part")
	if len(testArray(t, part["logprobs"], "logprobs")) != 1 {
		t.Fatalf("non-stream logprobs=%#v", part["logprobs"])
	}

	stream := "data: {\"id\":\"chatcmpl_logprobs\",\"choices\":[{\"delta\":{\"content\":\"A\"},\"logprobs\":{\"content\":[{\"token\":\"A\",\"logprob\":-0.1}]},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{Body: io.NopCloser(strings.NewReader(stream))}
	recorder := httptest.NewRecorder()
	responsesStreamHandler(recorder, nil, response, "model", "model", nil, nil, nil, nil, nil)
	body := recorder.Body.String()
	if !strings.Contains(body, `"logprobs":[{"logprob":-0.1,"token":"A"}]`) {
		t.Fatalf("stream logprobs missing: %s", body)
	}
}

func TestChatBridgeRejectsMalformedSuccessfulUpstreamResponse(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		apiType UpstreamType
		body    string
	}{
		{"responses", UpstreamResponses, `{"object":"response","status":"completed","output":"invalid"}`},
		{"anthropic", UpstreamAnthropic, `{"type":"message","content":"invalid"}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			matrixIsolateRuntime(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, testCase.body)
			}))
			defer server.Close()
			matrixSelectUpstream(server.URL, testCase.apiType)

			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
				"model":"matrix-public-model","messages":[{"role":"user","content":"hello"}]
			}`))
			response := httptest.NewRecorder()
			chatCompletionsHandler(response, request)
			if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "upstream_protocol_error") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
