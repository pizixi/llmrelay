package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bridgestate "llmrelay/backend/internal/bridge/state"
	"llmrelay/backend/internal/netproxy"
)

func decodeTestObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode JSON: %v\nbody: %s", err, body)
	}
	return got
}

func testObject(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	got, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s: want object, got %T (%v)", path, value, value)
	}
	return got
}

func testArray(t *testing.T, value any, path string) []any {
	t.Helper()
	got, ok := value.([]any)
	if !ok {
		t.Fatalf("%s: want array, got %T (%v)", path, value, value)
	}
	return got
}

func testString(t *testing.T, value any, path string) string {
	t.Helper()
	got, ok := value.(string)
	if !ok {
		t.Fatalf("%s: want string, got %T (%v)", path, value, value)
	}
	return got
}

func requireTestEqual(t *testing.T, path string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %#v, want %#v", path, got, want)
	}
}

func findTestObject(t *testing.T, values []any, path, key string, want any) map[string]any {
	t.Helper()
	for i, value := range values {
		obj, ok := value.(map[string]any)
		if ok && reflect.DeepEqual(obj[key], want) {
			return obj
		}
		_ = i
	}
	t.Fatalf("%s: no object with %s=%#v in %#v", path, key, want, values)
	return nil
}

func TestGetUpstreamEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		upstream *UpstreamConfig
		want     string
	}{
		{
			name:     "OpenAI Chat Completions",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/v1/", APIType: UpstreamOpenAI},
			want:     "https://example.test/v1/chat/completions",
		},
		{
			name:     "Anthropic Messages",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/v1/", APIType: UpstreamAnthropic},
			want:     "https://example.test/v1/messages",
		},
		{
			name:     "Anthropic root adds API version",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/", APIType: UpstreamAnthropic},
			want:     "https://example.test/v1/messages",
		},
		{
			name:     "OpenAI Responses",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/v1/", APIType: UpstreamResponses},
			want:     "https://example.test/v1/responses",
		},
		{
			name:     "complete endpoint is not duplicated",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/v1/messages", APIType: UpstreamAnthropic},
			want:     "https://example.test/v1/messages",
		},
		{
			name:     "unknown type safely defaults to Chat Completions",
			upstream: &UpstreamConfig{BaseURL: "https://example.test/v1", APIType: UpstreamType("future")},
			want:     "https://example.test/v1/chat/completions",
		},
		{name: "nil upstream", upstream: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getUpstreamEndpoint(tt.upstream); got != tt.want {
				t.Fatalf("getUpstreamEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAnthropicModelsEndpointAddsAPIVersion(t *testing.T) {
	tests := map[string]string{
		"https://example.test":             "https://example.test/v1/models",
		"https://example.test/v1":          "https://example.test/v1/models",
		"https://example.test/v1/messages": "https://example.test/v1/models",
	}
	for baseURL, want := range tests {
		upstream := &UpstreamConfig{BaseURL: baseURL, APIType: UpstreamAnthropic}
		if got := getUpstreamModelsEndpoint(upstream); got != want {
			t.Fatalf("getUpstreamModelsEndpoint(%q) = %q, want %q", baseURL, got, want)
		}
	}
}

func TestOpenAIToAnthropicRequestPreservesTextToolsAndImages(t *testing.T) {
	body := []byte(`{
		"model":"model-a",
		"messages":[
			{"role":"system","content":"Follow policy"},
			{"role":"user","content":[
				{"type":"text","text":"What is here?"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,AQID","detail":"high"}}
			]},
			{"role":"assistant","content":"I will check.","tool_calls":[{
				"id":"call_weather","type":"function",
				"function":{"name":"weather","arguments":"{\"city\":\"Shanghai\"}"}
			}]},
			{"role":"tool","tool_call_id":"call_weather","content":"sunny"}
		],
		"tools":[{"type":"function","function":{
			"name":"weather","description":"Get weather",
			"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
		}}],
		"tool_choice":{"type":"function","function":{"name":"weather"}}
	}`)

	got := decodeTestObject(t, openAIToAnthropicRequest(body))
	requireTestEqual(t, "system", got["system"], "Follow policy")

	messages := testArray(t, got["messages"], "messages")
	if len(messages) != 3 {
		t.Fatalf("messages: got %d entries, want 3: %#v", len(messages), messages)
	}
	user := testObject(t, messages[0], "messages[0]")
	requireTestEqual(t, "messages[0].role", user["role"], "user")
	userContent := testArray(t, user["content"], "messages[0].content")
	textBlock := findTestObject(t, userContent, "messages[0].content", "type", "text")
	requireTestEqual(t, "user text", textBlock["text"], "What is here?")
	imageBlock := findTestObject(t, userContent, "messages[0].content", "type", "image")
	source := testObject(t, imageBlock["source"], "image.source")
	requireTestEqual(t, "image.source.type", source["type"], "base64")
	requireTestEqual(t, "image.source.media_type", source["media_type"], "image/png")
	requireTestEqual(t, "image.source.data", source["data"], "AQID")

	assistant := testObject(t, messages[1], "messages[1]")
	assistantContent := testArray(t, assistant["content"], "messages[1].content")
	toolUse := findTestObject(t, assistantContent, "messages[1].content", "type", "tool_use")
	requireTestEqual(t, "tool_use.id", toolUse["id"], "call_weather")
	requireTestEqual(t, "tool_use.name", toolUse["name"], "weather")
	requireTestEqual(t, "tool_use.input.city", testObject(t, toolUse["input"], "tool_use.input")["city"], "Shanghai")

	toolResultMessage := testObject(t, messages[2], "messages[2]")
	toolResult := findTestObject(t, testArray(t, toolResultMessage["content"], "messages[2].content"), "messages[2].content", "type", "tool_result")
	requireTestEqual(t, "tool_result.tool_use_id", toolResult["tool_use_id"], "call_weather")
	requireTestEqual(t, "tool_result.content", toolResult["content"], "sunny")

	tools := testArray(t, got["tools"], "tools")
	tool := testObject(t, tools[0], "tools[0]")
	requireTestEqual(t, "tools[0].name", tool["name"], "weather")
	choice := testObject(t, got["tool_choice"], "tool_choice")
	requireTestEqual(t, "tool_choice.type", choice["type"], "tool")
	requireTestEqual(t, "tool_choice.name", choice["name"], "weather")
}

func TestOpenAIToAnthropicThinkingBudgetFitsMaxTokens(t *testing.T) {
	body := []byte(`{"model":"claude","messages":[{"role":"user","content":"hi"}],"max_tokens":4096,"temperature":0,"reasoning_effort":"high"}`)
	got := decodeTestObject(t, openAIToAnthropicRequest(body))
	thinking := testObject(t, got["thinking"], "thinking")
	budget, _ := getFloat(thinking, "budget_tokens")
	maxTokens, _ := getFloat(got, "max_tokens")
	if budget < 1024 || budget >= maxTokens {
		t.Fatalf("budget_tokens=%v max_tokens=%v, want 1024 <= budget < max", budget, maxTokens)
	}
	if _, exists := got["temperature"]; exists {
		t.Fatal("temperature must be omitted while Anthropic extended thinking is enabled")
	}
}

func TestOpenAIToResponsesRequestUsesNativeItemsAndPreservesImages(t *testing.T) {
	body := []byte(`{
		"model":"model-r",
		"messages":[
			{"role":"system","content":"Be concise"},
			{"role":"user","content":[
				{"type":"text","text":"Inspect"},
				{"type":"image_url","image_url":{"url":"https://example.test/image.png","detail":"high"}}
			]},
			{"role":"assistant","content":"Checking","tool_calls":[{
				"id":"call_1","type":"function","function":{"name":"inspect","arguments":"{\"mode\":\"fast\"}"}
			}]},
			{"role":"tool","tool_call_id":"call_1","content":"done"}
		],
		"tools":[{"type":"function","function":{
			"name":"inspect","description":"Inspect an image",
			"parameters":{"type":"object","properties":{"mode":{"type":"string"}}},"strict":true
		}}],
		"tool_choice":{"type":"function","function":{"name":"inspect"}}
	}`)

	got := decodeTestObject(t, openAIToResponsesRequest(body, &UpstreamConfig{APIType: UpstreamResponses}))
	requireTestEqual(t, "instructions", got["instructions"], "Be concise")
	input := testArray(t, got["input"], "input")

	var userMessage map[string]any
	for _, value := range input {
		obj, ok := value.(map[string]any)
		if ok && obj["role"] == "user" {
			userMessage = obj
			break
		}
	}
	if userMessage == nil {
		t.Fatalf("input: user message not found: %#v", input)
	}
	content := testArray(t, userMessage["content"], "user.content")
	inputText := findTestObject(t, content, "user.content", "type", "input_text")
	requireTestEqual(t, "input_text.text", inputText["text"], "Inspect")
	inputImage := findTestObject(t, content, "user.content", "type", "input_image")
	requireTestEqual(t, "input_image.image_url", inputImage["image_url"], "https://example.test/image.png")
	requireTestEqual(t, "input_image.detail", inputImage["detail"], "high")

	functionCall := findTestObject(t, input, "input", "type", "function_call")
	requireTestEqual(t, "function_call.call_id", functionCall["call_id"], "call_1")
	requireTestEqual(t, "function_call.name", functionCall["name"], "inspect")
	requireTestEqual(t, "function_call.arguments", functionCall["arguments"], `{"mode":"fast"}`)
	functionOutput := findTestObject(t, input, "input", "type", "function_call_output")
	requireTestEqual(t, "function_call_output.call_id", functionOutput["call_id"], "call_1")
	requireTestEqual(t, "function_call_output.output", functionOutput["output"], "done")
	for i, value := range input {
		obj := testObject(t, value, "input item")
		if _, exists := obj["tool_calls"]; exists {
			t.Fatalf("input[%d]: Responses function calls must be independent items, got %#v", i, obj)
		}
		if obj["role"] == "tool" {
			t.Fatalf("input[%d]: Chat role=tool is invalid in Responses input: %#v", i, obj)
		}
	}

	tools := testArray(t, got["tools"], "tools")
	tool := testObject(t, tools[0], "tools[0]")
	requireTestEqual(t, "tools[0].type", tool["type"], "function")
	requireTestEqual(t, "tools[0].name", tool["name"], "inspect")
	if _, nested := tool["function"]; nested {
		t.Fatalf("tools[0]: Responses function tool must be flat, got %#v", tool)
	}
	requireTestEqual(t, "tools[0].strict", tool["strict"], true)
	choice := testObject(t, got["tool_choice"], "tool_choice")
	requireTestEqual(t, "tool_choice.type", choice["type"], "function")
	requireTestEqual(t, "tool_choice.name", choice["name"], "inspect")
	if _, nested := choice["function"]; nested {
		t.Fatalf("tool_choice: Responses function choice must be flat, got %#v", choice)
	}
}

func TestConvertResponsesToChatMapsUsage(t *testing.T) {
	body := []byte(`{
		"id":"resp_123","object":"response","status":"completed","model":"upstream-model",
		"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],
		"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18,
			"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}
	}`)

	got := decodeTestObject(t, convertResponsesToChat(body, "client-model"))
	usage := testObject(t, got["usage"], "usage")
	requireTestEqual(t, "usage.prompt_tokens", usage["prompt_tokens"], float64(11))
	requireTestEqual(t, "usage.completion_tokens", usage["completion_tokens"], float64(7))
	requireTestEqual(t, "usage.total_tokens", usage["total_tokens"], float64(18))
	requireTestEqual(t, "usage.prompt_tokens_details.cached_tokens", testObject(t, usage["prompt_tokens_details"], "usage.prompt_tokens_details")["cached_tokens"], float64(3))
	requireTestEqual(t, "usage.completion_tokens_details.reasoning_tokens", testObject(t, usage["completion_tokens_details"], "usage.completion_tokens_details")["reasoning_tokens"], float64(2))
}

func TestConvertChatToResponsesReturnsTopLevelResponse(t *testing.T) {
	t.Run("text response", func(t *testing.T) {
		body := []byte(`{
			"id":"chatcmpl_123","created":1234,"model":"upstream-model",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}],
			"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}
		}`)

		got := decodeTestObject(t, convertChatToResponsesObject(body, "client-model", nil, nil, nil, nil))
		requireTestEqual(t, "object", got["object"], "response")
		requireTestEqual(t, "status", got["status"], "completed")
		if _, wrapped := got["response"]; wrapped {
			t.Fatalf("non-streaming Responses result must not use an SSE event wrapper: %#v", got)
		}
		output := testArray(t, got["output"], "output")
		message := findTestObject(t, output, "output", "type", "message")
		outputText := findTestObject(t, testArray(t, message["content"], "message.content"), "message.content", "type", "output_text")
		requireTestEqual(t, "output_text.text", outputText["text"], "hello")
	})

	t.Run("tool-only response has no fabricated empty message", func(t *testing.T) {
		body := []byte(`{
			"id":"chatcmpl_tool","created":1234,
			"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{
				"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}
			}]}}]
		}`)

		got := decodeTestObject(t, convertChatToResponsesObject(body, "client-model", nil, nil, nil, nil))
		output := testArray(t, got["output"], "output")
		if len(output) != 1 {
			t.Fatalf("output: got %d items, want only the function_call: %#v", len(output), output)
		}
		requireTestEqual(t, "output[0].type", testObject(t, output[0], "output[0]")["type"], "function_call")
	})
}

func TestConvertChatToResponsesForRequestPreservesNativeTools(t *testing.T) {
	chatBody := []byte(`{
		"id":"chatcmpl_native_tools","created":1234,
		"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"done"}}]
	}`)
	requestBody := []byte(`{
		"model":"model-r","input":"hello",
		"tools":[
			{"type":"function","name":"lookup","description":"Look up data","parameters":{"type":"object"},"strict":false},
			{"type":"web_search","search_context_size":"low"}
		],
		"tool_choice":{"type":"function","name":"lookup"}
	}`)

	got := decodeTestObject(t, convertChatToResponsesForRequest(chatBody, "model-r", requestBody, nil))
	request := decodeTestObject(t, requestBody)
	requireTestEqual(t, "tools", got["tools"], request["tools"])
	requireTestEqual(t, "tool_choice", got["tool_choice"], request["tool_choice"])

	tools := testArray(t, got["tools"], "tools")
	functionTool := testObject(t, tools[0], "tools[0]")
	if strict, exists := functionTool["strict"]; !exists || strict != false {
		t.Fatalf("tools[0].strict: got %#v (exists=%v), want explicit false", strict, exists)
	}
	if _, nested := functionTool["function"]; nested {
		t.Fatalf("tools[0]: native Responses function tool became nested: %#v", functionTool)
	}
	requireTestEqual(t, "tools[1].type", testObject(t, tools[1], "tools[1]")["type"], "web_search")
}

type parsedTestSSEEvent struct {
	name string
	data map[string]any
}

func parseTestSSE(t *testing.T, raw string) []parsedTestSSEEvent {
	t.Helper()
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	blocks := strings.Split(raw, "\n\n")
	events := make([]parsedTestSSEEvent, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var name string
		var dataLines []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(dataLines) == 0 || strings.Join(dataLines, "\n") == "[DONE]" {
			continue
		}
		data := decodeTestObject(t, []byte(strings.Join(dataLines, "\n")))
		events = append(events, parsedTestSSEEvent{name: name, data: data})
	}
	return events
}

func collectChatToolArgumentDeltas(t *testing.T, events []parsedTestSSEEvent) map[int]string {
	t.Helper()
	arguments := map[int]string{}
	for eventIndex, event := range events {
		choices, ok := event.data["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		choice := testObject(t, choices[0], "SSE choices[0]")
		delta, _ := choice["delta"].(map[string]any)
		toolCalls, _ := delta["tool_calls"].([]any)
		for _, value := range toolCalls {
			toolCall := testObject(t, value, "SSE tool_call")
			indexNumber, ok := toolCall["index"].(float64)
			if !ok {
				t.Fatalf("SSE event %d tool index: got %T (%v)", eventIndex, toolCall["index"], toolCall["index"])
			}
			function, _ := toolCall["function"].(map[string]any)
			argumentDelta, _ := function["arguments"].(string)
			arguments[int(indexNumber)] += argumentDelta
		}
	}
	return arguments
}

func TestAnthropicStreamToolBlockIndexMapsToChatToolIndex(t *testing.T) {
	// Anthropic 内容块索引包含 text/thinking 块，而 Chat 工具调用索引不包含，
	// 因此下面的内容块索引 1 必须转换为工具索引 0。
	stream := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"checking"}}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"weather","input":{}}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"Shanghai\"}"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":0}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"

	recorder := httptest.NewRecorder()
	anthropicStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-a", "model-a", true)
	events := parseTestSSE(t, recorder.Body.String())
	arguments := collectChatToolArgumentDeltas(t, events)
	if len(arguments) != 1 {
		t.Fatalf("tool argument streams: got %#v, want one Chat tool index", arguments)
	}
	requireTestEqual(t, "tool[0] arguments", arguments[0], `{"city":"Shanghai"}`)
}

func TestClaudeStreamKeepsParallelToolArgumentIndexes(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[` +
			`{"index":0,"id":"call_a","type":"function","function":{"name":"alpha","arguments":"{\"a\":"}},` +
			`{"index":1,"id":"call_b","type":"function","function":{"name":"beta","arguments":"{\"b\":"}}` +
			`]},"finish_reason":null}]}`,
		// 反转增量顺序，以发现所有工具都错误使用最近打开内容块索引的实现。
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[` +
			`{"index":1,"function":{"arguments":"2}"}},` +
			`{"index":0,"function":{"arguments":"1}"}}` +
			`]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	recorder := httptest.NewRecorder()
	claudeStreamHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-c", "model-c")
	events := parseTestSSE(t, recorder.Body.String())
	arguments := map[int]string{}
	toolNames := map[int]string{}
	for _, event := range events {
		indexNumber, hasIndex := event.data["index"].(float64)
		if !hasIndex {
			continue
		}
		index := int(indexNumber)
		switch event.name {
		case "content_block_start":
			block := testObject(t, event.data["content_block"], "content_block_start.content_block")
			if block["type"] == "tool_use" {
				toolNames[index], _ = block["name"].(string)
			}
		case "content_block_delta":
			delta := testObject(t, event.data["delta"], "content_block_delta.delta")
			if delta["type"] == "input_json_delta" {
				arguments[index] += testString(t, delta["partial_json"], "input_json_delta.partial_json")
			}
		}
	}
	requireTestEqual(t, "tool block names", toolNames, map[int]string{0: "alpha", 1: "beta"})
	requireTestEqual(t, "tool block arguments", arguments, map[int]string{0: `{"a":1}`, 1: `{"b":2}`})
}

func TestConvertStreamChunkWithUsagePreservesUsageOnlyChunk(t *testing.T) {
	line := `data: {"id":"chatcmpl_usage","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":5,"total_tokens":13}}`
	gotLine, usage := convertStreamChunkWithUsage(line)
	if strings.TrimSpace(gotLine) == "" {
		t.Fatal("usage-only Chat chunk was swallowed")
	}
	requireTestEqual(t, "extracted usage.total_tokens", usage["total_tokens"], float64(13))
	if !strings.HasPrefix(gotLine, "data: ") {
		t.Fatalf("converted line = %q, want data: prefix", gotLine)
	}
	got := decodeTestObject(t, []byte(strings.TrimPrefix(gotLine, "data: ")))
	if choices := testArray(t, got["choices"], "choices"); len(choices) != 0 {
		t.Fatalf("choices: got %#v, want empty usage-only chunk", choices)
	}
	requireTestEqual(t, "forwarded usage.total_tokens", testObject(t, got["usage"], "usage")["total_tokens"], float64(13))
}

func TestAnthropicThinkingSignatureSurvivesChatBridge(t *testing.T) {
	upstream := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_sig","usage":{"input_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reason"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signed-value"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"

	chatRecorder := httptest.NewRecorder()
	anthropicStreamToChatHandler(chatRecorder, io.NopCloser(strings.NewReader(upstream)), "model-a", "model-a", true)
	chatEvents := parseTestSSE(t, chatRecorder.Body.String())
	chatSignature := ""
	for _, event := range chatEvents {
		choices, _ := event.data["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice := testObject(t, choices[0], "choices[0]")
		delta, _ := choice["delta"].(map[string]any)
		if signature, _ := delta["reasoning_signature"].(string); signature != "" {
			chatSignature += signature
		}
	}
	requireTestEqual(t, "Chat reasoning signature", chatSignature, "signed-value")

	claudeRecorder := httptest.NewRecorder()
	claudeStreamHandler(claudeRecorder, io.NopCloser(strings.NewReader(chatRecorder.Body.String())), "model-a", "model-a")
	claudeEvents := parseTestSSE(t, claudeRecorder.Body.String())
	claudeSignature := ""
	for _, event := range claudeEvents {
		if event.name != "content_block_delta" {
			continue
		}
		delta := testObject(t, event.data["delta"], "content_block_delta.delta")
		if delta["type"] == "signature_delta" {
			claudeSignature += testString(t, delta["signature"], "signature_delta.signature")
		}
	}
	requireTestEqual(t, "Claude signature", claudeSignature, "signed-value")
}

func TestResponsesStreamToChatForwardsUpstreamDoneOnce(t *testing.T) {
	stream := strings.Join([]string{
		"event: response.output_text.delta\n" + `data: {"type":"response.output_text.delta","delta":"hello"}`,
		"event: response.completed\n" + `data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"

	recorder := httptest.NewRecorder()
	responsesStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-r", "model-r", false)
	got := recorder.Body.String()
	if count := strings.Count(got, "data: [DONE]"); count != 1 {
		t.Fatalf("[DONE] count = %d, want exactly one\nstream:\n%s", count, got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "data: [DONE]") {
		t.Fatalf("stream does not end with forwarded [DONE]:\n%s", got)
	}
}

func TestStreamFailuresDoNotEmitNormalCompletion(t *testing.T) {
	t.Run("Responses upstream to Chat", func(t *testing.T) {
		stream := "event: response.failed\n" +
			`data: {"type":"response.failed","response":{"status":"failed","error":{"type":"server_error","message":"boom"}}}` + "\n\n"
		recorder := httptest.NewRecorder()
		responsesStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-r", "model-r", false)
		got := recorder.Body.String()
		if !strings.Contains(got, `"error"`) || !strings.Contains(got, "boom") {
			t.Fatalf("Chat error event missing upstream failure:\n%s", got)
		}
		if strings.Contains(got, "data: [DONE]") || strings.Contains(got, `"finish_reason":"stop"`) {
			t.Fatalf("failed Chat stream emitted normal termination:\n%s", got)
		}
	})

	t.Run("Chat bridge to Claude", func(t *testing.T) {
		stream := `data: {"error":{"type":"upstream_error","message":"boom"}}` + "\n\n"
		recorder := httptest.NewRecorder()
		claudeStreamHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-c", "model-c")
		got := recorder.Body.String()
		if !strings.Contains(got, "event: error") || !strings.Contains(got, "boom") {
			t.Fatalf("Claude error event missing upstream failure:\n%s", got)
		}
		if strings.Contains(got, "event: message_stop") {
			t.Fatalf("failed Claude stream emitted message_stop:\n%s", got)
		}
	})

	t.Run("Chat bridge to Responses", func(t *testing.T) {
		stream := `data: {"type":"error","error":{"type":"upstream_error","message":"boom"}}` + "\n\n"
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/v1/responses", nil)
		response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(stream)), Header: make(http.Header)}
		responsesStreamHandler(recorder, request, response, "model-r", "model-r", nil, nil, nil, nil, nil)
		got := recorder.Body.String()
		if !strings.Contains(got, "event: response.failed") || !strings.Contains(got, "boom") {
			t.Fatalf("Responses failed event missing upstream failure:\n%s", got)
		}
		if strings.Contains(got, "event: response.completed") || strings.Contains(got, "event: response.incomplete") {
			t.Fatalf("failed Responses stream emitted normal terminal event:\n%s", got)
		}
	})
}

func TestClaudeToolChoiceToOpenAI(t *testing.T) {
	tests := []struct {
		name         string
		choice       any
		wantChoice   any
		wantParallel *bool
	}{
		{name: "auto", choice: map[string]any{"type": "auto"}, wantChoice: "auto"},
		{name: "any", choice: map[string]any{"type": "any"}, wantChoice: "required"},
		{
			name:       "specific tool",
			choice:     map[string]any{"type": "tool", "name": "weather"},
			wantChoice: map[string]any{"type": "function", "function": map[string]any{"name": "weather"}},
		},
		{name: "none", choice: map[string]any{"type": "none"}, wantChoice: "none"},
		{
			name:         "disable parallel tools",
			choice:       map[string]any{"type": "auto", "disable_parallel_tool_use": true},
			wantChoice:   "auto",
			wantParallel: boolTestPointer(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChoice, gotParallel := claudeToolChoiceToOpenAI(tt.choice)
			requireTestEqual(t, "tool choice", gotChoice, tt.wantChoice)
			switch {
			case tt.wantParallel == nil && gotParallel != nil:
				t.Fatalf("parallel_tool_calls = %v, want nil", *gotParallel)
			case tt.wantParallel != nil && gotParallel == nil:
				t.Fatalf("parallel_tool_calls = nil, want %v", *tt.wantParallel)
			case tt.wantParallel != nil && *gotParallel != *tt.wantParallel:
				t.Fatalf("parallel_tool_calls = %v, want %v", *gotParallel, *tt.wantParallel)
			}
		})
	}
}

func TestClaudeToolResultImmediatelyFollowsAssistantToolCall(t *testing.T) {
	messages := claudeToOpenAIMessages([]ClaudeMessage{
		{Role: "assistant", Content: []any{map[string]any{
			"type": "tool_use", "id": "toolu_1", "name": "weather", "input": map[string]any{"city": "Shanghai"},
		}}},
		{Role: "user", Content: []any{map[string]any{
			"type": "tool_result", "tool_use_id": "toolu_1", "content": "sunny",
		}}},
	}, nil)
	if len(messages) != 2 {
		t.Fatalf("messages=%#v, want assistant tool call followed by one tool result", messages)
	}
	if messages[0].Role != "assistant" || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("messages[0]=%#v, want assistant tool call", messages[0])
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != "toolu_1" {
		t.Fatalf("messages[1]=%#v, want matching tool result", messages[1])
	}
}

func boolTestPointer(value bool) *bool { return &value }

func TestOpenAIRequestPreservesUnknownStandardFields(t *testing.T) {
	raw := []byte(`{
		"model":"model-a","messages":[{"role":"user","content":"hello"}],
		"stop":["END","STOP"],"n":2,"seed":42,"logprobs":true,"top_logprobs":3,
		"frequency_penalty":0.25,"presence_penalty":-0.5,
		"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"}}},
		"modalities":["text"],"metadata":{"trace_id":"abc"},"user":"user-1"
	}`)
	var request OpenAIRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("unmarshal OpenAIRequest: %v", err)
	}
	got := decodeTestObject(t, buildUpstreamBody(&request))
	want := decodeTestObject(t, raw)
	for _, field := range []string{
		"stop", "n", "seed", "logprobs", "top_logprobs", "frequency_penalty",
		"presence_penalty", "response_format", "modalities", "metadata", "user",
	} {
		requireTestEqual(t, field, got[field], want[field])
	}
}

func TestResponsesRequestPreservesExplicitZeroSampling(t *testing.T) {
	var request ResponsesAPIRequest
	if err := json.Unmarshal([]byte(`{"model":"m","input":"hi","temperature":0,"top_p":0,"frequency_penalty":0,"presence_penalty":0}`), &request); err != nil {
		t.Fatalf("unmarshal Responses request: %v", err)
	}
	for name, value := range map[string]*float64{
		"temperature":       request.Temperature,
		"top_p":             request.TopP,
		"frequency_penalty": request.FrequencyPenalty,
		"presence_penalty":  request.PresencePenalty,
	} {
		if value == nil || *value != 0 {
			t.Fatalf("%s = %v, want explicit zero", name, value)
		}
	}

	chatBody := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"temperature":0,"top_p":0,"frequency_penalty":0,"presence_penalty":0}`)
	converted := decodeTestObject(t, openAIToResponsesRequest(chatBody, &UpstreamConfig{APIType: UpstreamResponses}))
	for _, name := range []string{"temperature", "top_p"} {
		requireTestEqual(t, name, converted[name], float64(0))
	}
	for _, name := range []string{"frequency_penalty", "presence_penalty"} {
		if _, exists := converted[name]; exists {
			t.Fatalf("%s must not be sent to the Responses API", name)
		}
	}
}

func TestResponsesRequestPointerSamplingRoundTrip(t *testing.T) {
	// 防止指针类型的采样字段退化回值类型。值类型配合 omitempty 时无法区分显式 0
	// 与字段缺失，会在线路传输时静默丢弃客户端意图。
	zero := 0.0
	encoded, err := json.Marshal(ResponsesAPIRequest{
		Model:       "m",
		Temperature: &zero,
		TopP:        &zero,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"temperature":0`) || !strings.Contains(string(encoded), `"top_p":0`) {
		t.Fatalf("explicit zero sampling fields must survive marshal: %s", string(encoded))
	}

	var decoded ResponsesAPIRequest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for name, value := range map[string]*float64{
		"temperature": decoded.Temperature,
		"top_p":       decoded.TopP,
	} {
		if value == nil || *value != 0 {
			t.Fatalf("%s = %v after round-trip, want explicit zero", name, value)
		}
	}
}
func TestNativeAnthropicRequestPassthroughPreservesOpaqueFields(t *testing.T) {
	original := map[string]any{
		"model":      "client-model",
		"max_tokens": float64(512),
		"system": []any{map[string]any{
			"type": "text", "text": "policy", "cache_control": map[string]any{"type": "ephemeral"},
		}},
		"messages": []any{map[string]any{
			"role": "assistant",
			"content": []any{map[string]any{
				"type": "thinking", "thinking": "reason", "signature": "opaque-signature",
			}},
		}},
		"tool_choice": map[string]any{"type": "any", "disable_parallel_tool_use": true},
	}
	wrapper, err := json.Marshal(map[string]any{
		"model":                     "resolved-model",
		"stream":                    true,
		internalAnthropicRequestKey: original,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeTestObject(t, openAIToAnthropicRequest(wrapper))
	requireTestEqual(t, "model", got["model"], "resolved-model")
	requireTestEqual(t, "stream", got["stream"], true)
	thinking := testObject(t, testArray(t, testObject(t, testArray(t, got["messages"], "messages")[0], "messages[0]")["content"], "content")[0], "thinking")
	requireTestEqual(t, "thinking.signature", thinking["signature"], "opaque-signature")
	systemBlock := testObject(t, testArray(t, got["system"], "system")[0], "system[0]")
	if systemBlock["cache_control"] == nil {
		t.Fatal("system cache_control was lost")
	}
}

func TestCleanJSONSchemaPreservesConstraintsAndDoesNotMutate(t *testing.T) {
	original := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"email": map[string]any{"type": "string", "format": "email", "examples": []any{"a@example.test"}},
		},
	}
	cleaned := testObject(t, cleanJsonSchema(original), "schema")
	requireTestEqual(t, "additionalProperties", cleaned["additionalProperties"], false)
	email := testObject(t, testObject(t, cleaned["properties"], "properties")["email"], "email")
	requireTestEqual(t, "email.format", email["format"], "email")
	delete(email, "format")
	originalEmail := testObject(t, testObject(t, original["properties"], "original.properties")["email"], "original.email")
	requireTestEqual(t, "original remains unchanged", originalEmail["format"], "email")
}

func TestValidateConfigRejectsUnknownRoutes(t *testing.T) {
	badType := &AppConfig{Upstreams: map[string]*UpstreamConfig{
		"bad": {BaseURL: "https://example.test/v1", APIType: UpstreamType("future")},
	}}
	if err := validateConfig(badType); err == nil {
		t.Fatal("validateConfig accepted an unknown api_type")
	}
	badBridgeMode := &AppConfig{Upstreams: map[string]*UpstreamConfig{
		"bad": {BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI, BridgeMode: BridgeMode("silent")},
	}}
	if err := validateConfig(badBridgeMode); err == nil {
		t.Fatal("validateConfig accepted an unknown bridge_mode")
	}
	badAlias := &AppConfig{
		Upstreams:  map[string]*UpstreamConfig{"good": {BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI}},
		ModelAlias: map[string]ModelAlias{"model": {TargetModel: "target", Upstream: "missing"}},
	}
	if err := validateConfig(badAlias); err == nil {
		t.Fatal("validateConfig accepted an alias pointing to an unknown upstream")
	}
	badWeightedAlias := &AppConfig{
		Upstreams: map[string]*UpstreamConfig{"good": {BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI}},
		ModelAlias: map[string]ModelAlias{"model": {Targets: []ModelAliasTarget{
			{TargetModel: "target", Upstream: "missing", Weight: 1},
		}}},
	}
	if err := validateConfig(badWeightedAlias); err == nil {
		t.Fatal("validateConfig accepted a weighted target pointing to an unknown upstream")
	}
	badWeight := &AppConfig{
		Upstreams: map[string]*UpstreamConfig{"good": {BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI}},
		ModelAlias: map[string]ModelAlias{"model": {Targets: []ModelAliasTarget{
			{TargetModel: "target", Upstream: "good", Weight: -1},
		}}},
	}
	if err := validateConfig(badWeight); err == nil {
		t.Fatal("validateConfig accepted a negative model target weight")
	}
	allDisabled := &AppConfig{
		Upstreams: map[string]*UpstreamConfig{"good": {BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI}},
		ModelAlias: map[string]ModelAlias{"model": {Targets: []ModelAliasTarget{
			{TargetModel: "target", Upstream: "good", Weight: 0},
		}}},
	}
	if err := validateConfig(allDisabled); err == nil {
		t.Fatal("validateConfig accepted an alias with no positive target weight")
	}
}

func TestNonOpenAIModelSyncFallsBackToOpenAIV1(t *testing.T) {
	for _, apiType := range []UpstreamType{UpstreamAnthropic, UpstreamResponses} {
		t.Run(string(apiType), func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/models":
					http.Error(w, "native endpoint unavailable", http.StatusNotFound)
				case "/v1/models":
					if apiType == UpstreamAnthropic {
						if got := r.Header.Get("x-api-key"); got != "secret" {
							t.Errorf("native Anthropic x-api-key=%q, want secret", got)
						}
						if got := r.Header.Get("Authorization"); got != "Bearer secret" {
							t.Errorf("Anthropic compatibility Authorization=%q, want Bearer secret", got)
						}
					} else if got := r.Header.Get("Authorization"); got != "Bearer secret" {
						t.Errorf("fallback Authorization=%q, want Bearer secret", got)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":[{"id":"fallback-model"}]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			models, err := fetchModelsFromUpstream("secondary", &UpstreamConfig{
				BaseURL: server.URL,
				APIKey:  "secret",
				APIType: apiType,
			}, false)
			if err != nil {
				t.Fatalf("fetch models: %v", err)
			}
			if len(models) != 1 || models[0].ID != "fallback-model" || models[0].OwnedBy != "secondary" {
				t.Fatalf("models=%#v", models)
			}
			wantPaths := []string{"/models", "/v1/models"}
			if apiType == UpstreamAnthropic {
				wantPaths = []string{"/v1/models"}
			}
			requireTestEqual(t, "request paths", paths, wantPaths)
		})
	}
}

func TestModelSyncFallbackOnlyAppliesToNonOpenAIBaseWithoutV1(t *testing.T) {
	for _, tc := range []struct {
		name    string
		apiType UpstreamType
		baseV1  bool
	}{
		{name: "openai root", apiType: UpstreamOpenAI},
		{name: "default openai root"},
		{name: "anthropic v1", apiType: UpstreamAnthropic, baseV1: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				http.Error(w, "unavailable", http.StatusNotFound)
			}))
			defer server.Close()
			baseURL := server.URL
			if tc.baseV1 {
				baseURL += "/v1"
			}
			_, err := fetchModelsFromUpstream("guarded", &UpstreamConfig{BaseURL: baseURL, APIType: tc.apiType}, false)
			if err == nil {
				t.Fatal("fetch models unexpectedly succeeded")
			}
			if requests != 1 {
				t.Fatalf("requests=%d, want 1", requests)
			}
		})
	}
}

func TestNormalizeConfigPreservesUpstreamOrderAndAppendsMissingNames(t *testing.T) {
	cfg := AppConfig{
		ModelAlias: map[string]ModelAlias{
			"model": {
				TargetModel: "legacy-target",
				Upstream:    "alpha",
				Targets: []ModelAliasTarget{
					{TargetModel: " target ", Upstream: " bravo ", Weight: 0},
				},
				ReasoningEffortMap: map[string]string{" low ": " high "},
			},
		},
		Upstreams: map[string]*UpstreamConfig{
			"charlie": {BaseURL: "https://charlie.example/v1", APIType: UpstreamOpenAI},
			"alpha":   {BaseURL: "https://alpha.example/v1", APIType: UpstreamOpenAI},
			"bravo":   {BaseURL: "https://bravo.example/v1", APIType: UpstreamOpenAI},
		},
		UpstreamOrder: []string{"bravo", "missing", "bravo"},
	}
	normalizeConfig(&cfg)
	requireTestEqual(t, "upstream order", cfg.UpstreamOrder, []string{"bravo", "alpha", "charlie"})
	requireTestEqual(t, "default follows order", cfg.DefaultUpstream, "bravo")
	requireTestEqual(t, "normalized model effort map", cfg.ModelAlias["model"].ReasoningEffortMap, map[string]string{"low": "high"})
	requireTestEqual(t, "normalized weighted targets", cfg.ModelAlias["model"].Targets, []ModelAliasTarget{{TargetModel: "target", Upstream: "bravo", Weight: 0}})
	if cfg.ModelAlias["model"].TargetModel != "" || cfg.ModelAlias["model"].Upstream != "" {
		t.Fatal("normalized weighted alias retained legacy target fields")
	}
}

func TestModelAliasTargetWeightJSONCompatibility(t *testing.T) {
	var legacyTarget ModelAliasTarget
	if err := json.Unmarshal([]byte(`{"target_model":"legacy","upstream":"primary"}`), &legacyTarget); err != nil {
		t.Fatalf("unmarshal legacy target: %v", err)
	}
	if legacyTarget.Weight != 1 {
		t.Fatalf("legacy target weight=%d, want 1", legacyTarget.Weight)
	}

	var disabledTarget ModelAliasTarget
	if err := json.Unmarshal([]byte(`{"target_model":"disabled","upstream":"backup","weight":0}`), &disabledTarget); err != nil {
		t.Fatalf("unmarshal disabled target: %v", err)
	}
	if disabledTarget.Weight != 0 {
		t.Fatalf("disabled target weight=%d, want 0", disabledTarget.Weight)
	}
	encoded, err := json.Marshal(disabledTarget)
	if err != nil {
		t.Fatalf("marshal disabled target: %v", err)
	}
	if !strings.Contains(string(encoded), `"weight":0`) {
		t.Fatalf("disabled target JSON %s does not preserve zero weight", encoded)
	}
}

func TestModelReasoningEffortMapOverridesGlobalMap(t *testing.T) {
	configMu.Lock()
	oldGlobal := reasoningEffortMap
	reasoningEffortMap = map[string]string{"low": "global-low", "medium": "global-medium"}
	configMu.Unlock()
	syncLegacyConfig()
	t.Cleanup(func() {
		configMu.Lock()
		reasoningEffortMap = oldGlobal
		configMu.Unlock()
		syncLegacyConfig()
	})

	modelMap := getReasoningEffortMapForAlias(ModelAlias{ReasoningEffortMap: map[string]string{"low": "high"}})
	requireTestEqual(t, "model override", mapConfiguredReasoningEffort("low", modelMap), "high")
	requireTestEqual(t, "model map does not fall back globally", mapConfiguredReasoningEffort("medium", modelMap), "medium")

	globalMap := getReasoningEffortMapForAlias(ModelAlias{})
	requireTestEqual(t, "global fallback", mapConfiguredReasoningEffort("medium", globalMap), "global-medium")

	requestBody := decodeTestObject(t, buildUpstreamBody(&OpenAIRequest{
		Model: "target", ReasoningEffort: "low", ConfiguredReasoningEffortMap: modelMap,
	}, true))
	requireTestEqual(t, "request model override", requestBody["reasoning_effort"], "high")
}

func TestModelReasoningSwitchControlsForwardedFields(t *testing.T) {
	var request OpenAIRequest
	if err := json.Unmarshal([]byte(`{
		"model":"target","messages":[{"role":"user","content":"hello"}],
		"thinking":{"type":"adaptive"},"reasoning_effort":"high",
		"extra_body":{"thinking":{"type":"invalid"},"reasoning_effort":"low","future_option":true}
	}`), &request); err != nil {
		t.Fatal(err)
	}

	disabled := decodeTestObject(t, buildUpstreamBody(&request, false))
	if _, exists := disabled["thinking"]; exists {
		t.Fatalf("thinking must be omitted when model reasoning is disabled: %#v", disabled["thinking"])
	}
	if _, exists := disabled["reasoning_effort"]; exists {
		t.Fatalf("reasoning_effort must be omitted when model reasoning is disabled: %#v", disabled["reasoning_effort"])
	}
	requireTestEqual(t, "unrelated extra_body field", disabled["future_option"], true)

	enabled := decodeTestObject(t, buildUpstreamBody(&request, true))
	requireTestEqual(t, "enabled thinking type", testObject(t, enabled["thinking"], "thinking")["type"], "adaptive")
	requireTestEqual(t, "enabled reasoning effort", enabled["reasoning_effort"], "high")
}

func TestChatPassthroughDropsReasoningWhenModelReasoningIsDisabled(t *testing.T) {
	raw := []byte(`{
		"model":"alias","messages":[{"role":"user","content":"hello"}],
		"thinking":{"type":"invalid"},"reasoning_effort":"high",
		"extra_body":{"thinking":{"type":"invalid-extra"},"reasoning_effort":"low","future_option":true}
	}`)

	converted, err := prepareChatPassthroughBody(raw, "target", "high", false)
	if err != nil {
		t.Fatal(err)
	}
	body := decodeTestObject(t, converted)
	if _, exists := body["thinking"]; exists {
		t.Fatalf("thinking must be omitted from passthrough request: %#v", body["thinking"])
	}
	if _, exists := body["reasoning_effort"]; exists {
		t.Fatalf("reasoning_effort must be omitted from passthrough request: %#v", body["reasoning_effort"])
	}
	if _, exists := body["extra_body"]; exists {
		t.Fatal("extra_body must be flattened for passthrough requests")
	}
	requireTestEqual(t, "passthrough unrelated extra_body field", body["future_option"], true)
}

func TestModelReasoningEffortMapAppliesToResponsesPaths(t *testing.T) {
	effortMap := map[string]string{"low": "high"}
	raw := []byte(`{"model":"alias","reasoning":{"effort":"low"},"input":"hello"}`)

	passthrough, err := prepareResponsesPassthroughBodyWithEffort(raw, "target", effortMap, true)
	if err != nil {
		t.Fatal(err)
	}
	passthroughBody := decodeTestObject(t, passthrough)
	passthroughReasoning := testObject(t, passthroughBody["reasoning"], "reasoning")
	requireTestEqual(t, "passthrough effort", passthroughReasoning["effort"], "high")

	toAnthropic, _, _, err := convertResponsesRequestToAnthropicDirect(raw, "target", true, effortMap)
	if err != nil {
		t.Fatal(err)
	}
	anthropicBody := decodeTestObject(t, toAnthropic)
	thinking := testObject(t, anthropicBody["thinking"], "thinking")
	requireTestEqual(t, "direct Anthropic budget", thinking["budget_tokens"], float64(32000))

	toResponses, _, err := convertAnthropicRequestToResponsesDirect([]byte(`{
		"model":"alias","max_tokens":100,"messages":[{"role":"user","content":"hello"}],
		"thinking":{"type":"enabled","budget_tokens":3000}
	}`), "target", true, effortMap)
	if err != nil {
		t.Fatal(err)
	}
	responsesBody := decodeTestObject(t, toResponses)
	responsesReasoning := testObject(t, responsesBody["reasoning"], "reasoning")
	requireTestEqual(t, "direct Responses effort", responsesReasoning["effort"], "high")
}

func TestUpstreamMaxRetriesCanBeDisabled(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"try later","type":"unavailable"}}`)
	}))
	defer server.Close()

	zero := 0
	upstream := &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI, MaxRetries: &zero}
	body, status, _, err := callPreparedUpstream(t.Context(), []byte(`{"model":"m","messages":[]}`), "test", "m", upstream)
	if err == nil {
		t.Fatal("callPreparedUpstream returned nil error for 503")
	}
	if status != http.StatusServiceUnavailable || attempts != 1 {
		t.Fatalf("status=%d attempts=%d, want 503 and one attempt; body=%s", status, attempts, body)
	}
}

func TestRequireAPIAuth(t *testing.T) {
	oldKey := apiAccessKey
	apiAccessKey = "secret"
	t.Cleanup(func() { apiAccessKey = oldKey })
	handler := requireAPIAuth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	unauthorized := httptest.NewRecorder()
	handler(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("without key status=%d, want 401", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer secret")
	authorized := httptest.NewRecorder()
	handler(authorized, authorizedRequest)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("with key status=%d, want 204", authorized.Code)
	}
}

func TestResponsesBridgeRejectsStatefulOrHostedRequestFields(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{name: "previous response", fields: map[string]any{"previous_response_id": "resp_1"}, want: "previous_response_id"},
		{name: "conversation", fields: map[string]any{"conversation": map[string]any{"id": "conv_1"}}, want: "conversation"},
		{name: "prompt template", fields: map[string]any{"prompt": map[string]any{"id": "pmpt_1"}}, want: "prompt"},
		{name: "background", fields: map[string]any{"background": true}, want: "background"},
		{name: "stored response", fields: map[string]any{"store": true}, want: "store"},
		{name: "obfuscation", fields: map[string]any{"stream_options": map[string]any{"include_obfuscation": true}}, want: "stream_options.include_obfuscation"},
		{name: "unknown stream option", fields: map[string]any{"stream_options": map[string]any{"future": true}}, want: "stream_options.future"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResponsesBridgeRequest(tt.fields)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want unsupported field %q", err, tt.want)
			}
		})
	}

	t.Run("include output hint is observable in strict mode", func(t *testing.T) {
		fields := map[string]any{"include": []any{"reasoning.encrypted_content"}}
		if err := validateResponsesBridgeRequest(fields); err == nil || !strings.Contains(err.Error(), "include") {
			t.Fatalf("strict validation error=%v, want include", err)
		}
		warnings := responsesBridgeRequestWarnings(fields)
		if len(warnings) != 1 || warnings[0].Code != "encrypted_reasoning_unavailable" || warnings[0].Path != "include[0]" {
			t.Fatalf("warnings=%#v", warnings)
		}
		if err := validateResponsesBridgeRequest(map[string]any{"include": []any{}}); err != nil {
			t.Fatalf("empty include should remain harmless: %v", err)
		}
	})

	allowed := map[string]any{
		"background": false,
		"store":      false,
		"stream_options": map[string]any{
			"include_obfuscation": false,
		},
	}
	if err := validateResponsesBridgeRequest(allowed); err != nil {
		t.Fatalf("harmless defaults rejected: %v", err)
	}
}

func TestResponsesHandlerRejectsUnsupportedBridgeFieldBeforeUpstreamCall(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"hello","previous_response_id":"resp_1"
	}`))
	request.Header.Set("X-Llm2api-Bridge-Mode", "strict")
	recorderHTTP := httptest.NewRecorder()
	responsesHandler(recorderHTTP, request)
	if recorderHTTP.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", recorderHTTP.Code, recorderHTTP.Body.String())
	}
	if !strings.Contains(recorderHTTP.Body.String(), "previous_response_id") {
		t.Fatalf("response does not name unsupported field: %s", recorderHTTP.Body.String())
	}
	if calls := recorder.snapshot(); len(calls) != 0 {
		t.Fatalf("upstream was called for an invalid bridge request: %#v", calls)
	}
}

func TestConvertChatToResponsesMarksInvalidToolArgumentsIncomplete(t *testing.T) {
	body := []byte(`{
		"id":"chatcmpl_truncated","created":1234,
		"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{
			"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":"}
		}]}}]
	}`)
	got := decodeTestObject(t, convertChatToResponsesObject(body, "model-r", nil, nil, nil, nil))
	requireTestEqual(t, "status", got["status"], "incomplete")
	details := testObject(t, got["incomplete_details"], "incomplete_details")
	requireTestEqual(t, "incomplete reason", details["reason"], "tool_call_arguments_incomplete")
	call := findTestObject(t, testArray(t, got["output"], "output"), "output", "type", "function_call")
	requireTestEqual(t, "function call status", call["status"], "incomplete")
}

func TestStreamBridgesRejectPrematureEOF(t *testing.T) {
	t.Run("Responses to Chat", func(t *testing.T) {
		stream := "event: response.output_text.delta\n" +
			`data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"
		recorder := httptest.NewRecorder()
		responsesStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-r", "model-r", false)
		got := recorder.Body.String()
		if !strings.Contains(got, "ended before a terminal response event") {
			t.Fatalf("missing premature EOF error:\n%s", got)
		}
		if strings.Contains(got, "data: [DONE]") || strings.Contains(got, `"finish_reason":"stop"`) {
			t.Fatalf("premature stream emitted normal completion:\n%s", got)
		}
	})

	t.Run("Anthropic to Chat", func(t *testing.T) {
		stream := strings.Join([]string{
			`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":1}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
		}, "\n\n") + "\n\n"
		recorder := httptest.NewRecorder()
		anthropicStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-a", "model-a", false)
		got := recorder.Body.String()
		if !strings.Contains(got, "ended before a terminal event") {
			t.Fatalf("missing premature EOF error:\n%s", got)
		}
		if strings.Contains(got, "data: [DONE]") || strings.Contains(got, `"finish_reason":"stop"`) {
			t.Fatalf("premature stream emitted normal completion:\n%s", got)
		}
	})

	t.Run("Chat to Anthropic", func(t *testing.T) {
		stream := `data: {"id":"chatcmpl_1","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"},"finish_reason":null}]}` + "\n\n"
		recorder := httptest.NewRecorder()
		claudeStreamHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model-c", "model-c")
		got := recorder.Body.String()
		if !strings.Contains(got, "event: error") || !strings.Contains(got, "before finish_reason") {
			t.Fatalf("missing premature EOF error:\n%s", got)
		}
		if strings.Contains(got, "event: message_stop") {
			t.Fatalf("premature stream emitted message_stop:\n%s", got)
		}
	})
}

func TestResponsesStreamContentFilterIsIncomplete(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl_filter","created":1234,"choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_filter","created":1234,"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	recorder := httptest.NewRecorder()
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(stream)), Header: make(http.Header)}
	responsesStreamHandler(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil), response, "model-r", "model-r", nil, nil, nil, nil, nil)

	var terminal map[string]any
	for _, event := range parseTestSSE(t, recorder.Body.String()) {
		if event.name == "response.incomplete" {
			terminal = event.data
		}
	}
	if terminal == nil {
		t.Fatalf("response.incomplete event missing:\n%s", recorder.Body.String())
	}
	responseObject := testObject(t, terminal["response"], "response.incomplete.response")
	details := testObject(t, responseObject["incomplete_details"], "incomplete_details")
	requireTestEqual(t, "incomplete reason", details["reason"], "content_filter")
	requireTestEqual(t, "tools default", responseObject["tools"], []any{})
	requireTestEqual(t, "tool choice default", responseObject["tool_choice"], "auto")
	requireTestEqual(t, "parallel tools default", responseObject["parallel_tool_calls"], true)
}

func TestResponsesGeneratedObjectEchoesAppliedRequestFields(t *testing.T) {
	chatBody := []byte(`{
		"id":"chatcmpl_echo","created":1234,
		"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]
	}`)
	requestBody := []byte(`{
		"model":"model-r","input":"hello","instructions":"be concise",
		"metadata":{"trace":"abc"},"temperature":0,"top_p":0,"max_output_tokens":32,"store":false,
		"reasoning":{"effort":"low"}
	}`)
	got := decodeTestObject(t, convertChatToResponsesForRequest(chatBody, "model-r", requestBody, nil))
	request := decodeTestObject(t, requestBody)
	for _, field := range []string{"instructions", "metadata", "temperature", "top_p", "max_output_tokens", "store", "reasoning"} {
		requireTestEqual(t, field, got[field], request[field])
	}
	requireTestEqual(t, "tools default", got["tools"], []any{})
	requireTestEqual(t, "tool choice default", got["tool_choice"], "auto")
	requireTestEqual(t, "parallel tools default", got["parallel_tool_calls"], true)
}

func TestOpenAIToResponsesDoesNotInjectChatIncludeUsage(t *testing.T) {
	body := []byte(`{
		"model":"model-r","messages":[{"role":"user","content":"hello"}],"stream":true,
		"stream_options":{"include_usage":true,"include_obfuscation":false}
	}`)
	got := decodeTestObject(t, openAIToResponsesRequest(body, &UpstreamConfig{APIType: UpstreamResponses}))
	options := testObject(t, got["stream_options"], "stream_options")
	requireTestEqual(t, "include_obfuscation", options["include_obfuscation"], false)
	if _, exists := options["include_usage"]; exists {
		t.Fatal("Chat stream_options.include_usage leaked into Responses request")
	}
}

func TestClaudeToolResultErrorFlagDoesNotLeakIntoChatMessage(t *testing.T) {
	messages := claudeToOpenAIMessages([]ClaudeMessage{{
		Role: "user",
		Content: []any{map[string]any{
			"type": "tool_result", "tool_use_id": "toolu_1", "content": "failed", "is_error": true,
		}},
	}}, nil)
	if len(messages) != 1 || messages[0].Role != "tool" {
		t.Fatalf("messages=%#v, want one tool message", messages)
	}
	encoded, err := json.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "is_error") {
		t.Fatalf("Anthropic is_error leaked into Chat message: %s", encoded)
	}
	if content, _ := messages[0].Content.(string); !strings.Contains(content, "[tool_error]") {
		t.Fatalf("Anthropic tool error semantics were lost: %#v", messages[0].Content)
	}
}

func TestAnthropicProtocolHeadersAreForwardedForNativeMessages(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamAnthropic)
	matrixSelectUpstream(upstreamServer.URL, UpstreamAnthropic)
	gateway := matrixGatewayServer(t)

	request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(`{
		"model":"matrix-public-model","max_tokens":32,"messages":[{"role":"user","content":"hello"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer client-secret")
	request.Header.Set("anthropic-version", "2024-01-01")
	request.Header.Set("anthropic-beta", "custom-beta-2026-01-01")
	response, err := gateway.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", response.StatusCode)
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	requireTestEqual(t, "anthropic-version", calls[0].header.Get("anthropic-version"), "2024-01-01")
	requireTestEqual(t, "anthropic-beta", calls[0].header.Get("anthropic-beta"), "custom-beta-2026-01-01")
	if got := calls[0].header.Get("Authorization"); got != "Bearer matrix-api-key" {
		t.Fatalf("upstream Authorization=%q, want configured gateway credential", got)
	}
}

func TestUpstreamRequestCancellationStopsInFlightCall(t *testing.T) {
	started := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-releaseHandler:
		}
	}))
	defer server.Close()
	defer close(releaseHandler)

	zero := 0
	upstream := &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI, MaxRetries: &zero}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, _, err := callPreparedUpstream(ctx, []byte(`{"model":"m","messages":[]}`), "cancel", "m", upstream)
		result <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not start")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled upstream request did not return")
	}
}

func TestConfigDatabaseRejectsJSONAndReplacesNormalizedConfigCleanly(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "llmrelay.db")
	if _, err := loadConfig(filepath.Join(tempDir, "config.json")); err == nil {
		t.Fatal("loadConfig accepted a JSON configuration path")
	}

	first := AppConfig{
		Upstreams:       map[string]*UpstreamConfig{"one": {BaseURL: "https://one.example/v1", APIType: UpstreamOpenAI}},
		UpstreamOrder:   []string{"one"},
		DefaultUpstream: "one",
	}
	second := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"three": {BaseURL: "https://three.example/v1", APIType: UpstreamAnthropic},
			"two":   {BaseURL: "https://two.example/v1", APIType: UpstreamResponses},
		},
		UpstreamOrder:   []string{"two", "three"},
		DefaultUpstream: "two",
	}
	if err := saveConfig(path, first); err != nil {
		t.Fatalf("save first config: %v", err)
	}
	if err := saveConfig(path, second); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load replaced config: %v", err)
	}
	if loaded.DefaultUpstream != "two" || loaded.Upstreams["two"] == nil {
		t.Fatalf("loaded stale config: %#v", loaded)
	}
	requireTestEqual(t, "persisted upstream order", loaded.UpstreamOrder, []string{"two", "three"})
	if _, err := os.Stat(filepath.Join(tempDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("JSON configuration file was created: %v", err)
	}
}

func TestTokenStatsConsecutiveAndQueuedSavesUseLatestSnapshot(t *testing.T) {
	oldPath := tokenStatsPath
	tokenStatsMu.Lock()
	oldStats := tokenStats
	oldDate := statsDate
	tokenStatsMu.Unlock()
	t.Cleanup(func() {
		tokenStatsMu.Lock()
		tokenStats = oldStats
		statsDate = oldDate
		tokenStatsMu.Unlock()
		tokenStatsPath = oldPath
	})

	tokenStatsPath = filepath.Join(t.TempDir(), "stats.json")
	tokenStatsMu.Lock()
	tokenStats = &TokenStatsData{TotalRequests: 1, Models: map[string]*ModelStats{"m": {RequestCount: 1}}}
	tokenStatsMu.Unlock()
	saveTokenStats()
	tokenStatsMu.Lock()
	tokenStats.TotalRequests = 2
	tokenStats.Models["m"].RequestCount = 2
	tokenStatsMu.Unlock()
	saveTokenStats()

	readStats := func() TokenStatsData {
		data, err := os.ReadFile(tokenStatsPath)
		if err != nil {
			t.Fatal(err)
		}
		var stats TokenStatsData
		if err := json.Unmarshal(data, &stats); err != nil {
			t.Fatalf("decode stats: %v; body=%s", err, data)
		}
		return stats
	}
	if got := readStats(); got.TotalRequests != 2 || got.Models["m"].RequestCount != 2 {
		t.Fatalf("consecutive save wrote stale data: %#v", got)
	}

	scheduleTokenStatsSave()
	tokenStatsMu.Lock()
	tokenStats = &TokenStatsData{Models: map[string]*ModelStats{}, Daily: &DailyStats{Date: getToday(), Models: map[string]*ModelStats{}}}
	statsDate = getToday()
	tokenStatsMu.Unlock()
	saveTokenStats()
	time.Sleep(350 * time.Millisecond)
	if got := readStats(); got.TotalRequests != 0 || len(got.Models) != 0 {
		t.Fatalf("queued save restored cleared stats: %#v", got)
	}
}

func TestRoundRobinSocksClientsAreReusedPerProxyAndMode(t *testing.T) {
	matrixIsolateRuntime(t)
	socks5Mu.Lock()
	socks5Proxies = []Socks5Proxy{{Addr: "127.0.0.1:10001"}, {Addr: "127.0.0.1:10002"}}
	activeSocks5 = socks5RR
	socks5ClientCache = map[socks5ClientCacheKey]*http.Client{}
	atomic.StoreUint32(&socks5RRIndex, 0)
	socks5Mu.Unlock()
	netproxy.Configure(socks5Proxies, activeSocks5)

	first, firstExit := getHTTPClientWithExit(false)
	second, secondExit := getHTTPClientWithExit(false)
	third, thirdExit := getHTTPClientWithExit(false)
	if firstExit == secondExit || firstExit != thirdExit {
		t.Fatalf("round-robin exits=%q,%q,%q", firstExit, secondExit, thirdExit)
	}
	if first != third || first == second {
		t.Fatal("round-robin clients were not reused per proxy")
	}
	streamClient, _ := getHTTPClientWithExit(true)
	if streamClient == first || streamClient == second {
		t.Fatal("stream and non-stream clients unexpectedly share timeout configuration")
	}
	cacheSize := netproxy.CacheSize()
	if cacheSize != 3 {
		t.Fatalf("client cache size=%d, want 3", cacheSize)
	}
}

func TestResponsesHostedToolWarnsAndContinues(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model",
		"input":"hello",
		"tools":[{"type":"web_search"}],
		"tool_choice":{"type":"web_search"}
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Llm2api-Warning-Count") == "" {
		t.Fatalf("bridge warning header is missing: %#v", response.Header())
	}
	body := decodeTestObject(t, response.Body.Bytes())
	if warnings, ok := body["llm2api_warnings"].([]any); !ok || len(warnings) == 0 {
		t.Fatalf("response warnings are missing: %#v", body["llm2api_warnings"])
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	if tools, exists := calls[0].body["tools"]; exists {
		t.Fatalf("unsupported hosted tool leaked upstream: %#v", tools)
	}
	if calls[0].body["tool_choice"] != "auto" {
		t.Fatalf("tool_choice=%#v, want compatibility downgrade to auto", calls[0].body["tool_choice"])
	}
}

func TestResponsesStrictModeRejectsLossyToolBridge(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"hello","tools":[{"type":"computer_use_preview"}]
	}`))
	request.Header.Set("X-Llm2api-Bridge-Mode", "strict")
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", response.Code, response.Body.String())
	}
	if calls := recorder.snapshot(); len(calls) != 0 {
		t.Fatalf("strict lossy bridge reached upstream: %#v", calls)
	}
}

func TestResponsesStatefulFieldsUseBestEffortByDefault(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model",
		"input":"continue with the available context",
		"previous_response_id":"resp_previous",
		"background":true,
		"store":true
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
	}
	if len(recorder.snapshot()) != 1 {
		t.Fatalf("best-effort request did not reach upstream: %#v", recorder.snapshot())
	}
	body := decodeTestObject(t, response.Body.Bytes())
	warnings, ok := body["llm2api_warnings"].([]any)
	if !ok || len(warnings) < 3 {
		t.Fatalf("warnings=%#v, want state, background, and store warnings", body["llm2api_warnings"])
	}
	requireTestEqual(t, "effective store", body["store"], true)
	storageEmulated := false
	for _, rawWarning := range warnings {
		warning, _ := rawWarning.(map[string]any)
		if warning["code"] == "storage_emulated" && warning["path"] == "store" {
			storageEmulated = true
			break
		}
	}
	if !storageEmulated {
		t.Fatalf("warnings=%#v, want storage_emulated", body["llm2api_warnings"])
	}
}

func TestResponsesLocalStateIsEmulatedOnlyWhenAvailable(t *testing.T) {
	matrixIsolateRuntime(t)
	bridgestate.Default().Reset()
	t.Cleanup(bridgestate.Default().Reset)
	_, stored := bridgestate.Default().PutResponse(map[string]any{
		"id": "resp_saved",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": "remembered context"}},
		}},
	})
	if !stored {
		t.Fatal("failed to seed local Responses state")
	}
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"continue","previous_response_id":"resp_saved","store":true
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	messages, ok := calls[0].body["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("converted messages=%#v, want previous state plus current input", calls[0].body["messages"])
	}
	if !strings.Contains(fmt.Sprint(messages[0]), "remembered context") {
		t.Fatalf("previous state was not forwarded: %#v", messages[0])
	}
	body := decodeTestObject(t, response.Body.Bytes())
	if body["store"] != true {
		t.Fatalf("store=%#v, want true for local emulation", body["store"])
	}
	warnings, _ := body["llm2api_warnings"].([]any)
	foundState, foundStore := false, false
	for _, rawWarning := range warnings {
		warning, _ := rawWarning.(map[string]any)
		switch warning["code"] {
		case "stateful_context_emulated":
			foundState = true
		case "storage_emulated":
			foundStore = true
		}
	}
	if !foundState || !foundStore {
		t.Fatalf("warnings=%#v, want stateful_context_emulated and storage_emulated", warnings)
	}
	responseID, _ := body["id"].(string)
	if responseID == "" {
		t.Fatal("converted response has no id")
	}
	if _, ok := bridgestate.Default().Get(responseID); !ok {
		t.Fatalf("stored converted response %q was not available for the next request", responseID)
	}
}

func TestResponsesCustomToolUsesReversibleFunctionWrapper(t *testing.T) {
	tools, mappings, warnings := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{
		Type: "custom", Name: "code.exec", Description: "Run code",
	}})
	if len(tools) != 1 || len(mappings) != 1 {
		t.Fatalf("tools=%#v mappings=%#v", tools, mappings)
	}
	if len(warnings) == 0 || warnings[0].Code != "tool_name_rewritten" && warnings[0].Code != "custom_tool_emulated" {
		t.Fatalf("custom compatibility warning missing: %#v", warnings)
	}
	upstreamName := tools[0].Function.Name
	mapping, ok := mappings[upstreamName]
	if !ok || mapping.Kind != "custom" || mapping.Name != "code.exec" {
		t.Fatalf("mapping[%q]=%#v, found=%v", upstreamName, mapping, ok)
	}

	chatBody, err := json.Marshal(map[string]any{
		"id": "chatcmpl_custom",
		"choices": []any{map[string]any{
			"finish_reason": "tool_calls",
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []any{map[string]any{
					"id": "call_custom", "type": "function",
					"function": map[string]any{"name": upstreamName, "arguments": `{"input":"print(1)"}`},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	converted := decodeTestObject(t, convertChatToResponsesObject(chatBody, "model-r", nil, nil, nil, mappings))
	output := testArray(t, converted["output"], "output")
	call := findTestObject(t, output, "output", "type", "custom_tool_call")
	requireTestEqual(t, "custom call name", call["name"], "code.exec")
	requireTestEqual(t, "custom call input", call["input"], "print(1)")
}

func TestResponsesCustomHistoryAndUnknownItemsBridgeSafely(t *testing.T) {
	tools, mappings, _ := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{Type: "custom", Name: "code_exec"}})
	if len(tools) != 1 {
		t.Fatalf("tools=%#v", tools)
	}
	input := []any{
		map[string]any{"type": "custom_tool_call", "call_id": "call_1", "name": "code_exec", "input": "print(1)"},
		map[string]any{"type": "custom_tool_call_output", "call_id": "call_1", "output": "1"},
		map[string]any{"type": "computer_call_output", "call_id": "computer_1", "output": "opaque"},
	}
	messages, warnings := responsesInputToMessagesWithWarnings(input, "", mappings)
	if len(messages) != 4 {
		t.Fatalf("messages=%#v, want custom call/result plus emulated computer result", messages)
	}
	if messages[0].Role != "assistant" || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("messages[0]=%#v", messages[0])
	}
	if messages[0].ToolCalls[0].Function.Name != tools[0].Function.Name {
		t.Fatalf("custom history name=%q, want %q", messages[0].ToolCalls[0].Function.Name, tools[0].Function.Name)
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != "call_1" {
		t.Fatalf("messages[1]=%#v", messages[1])
	}
	if messages[2].Role != "assistant" || len(messages[2].ToolCalls) != 1 || messages[3].Role != "tool" {
		t.Fatalf("emulated computer history=%#v", messages[2:])
	}
	if len(warnings) != 1 || warnings[0].Code != "tool_history_emulated" {
		t.Fatalf("warnings=%#v", warnings)
	}
}

func TestResponsesNamespaceToolNamesCannotCollide(t *testing.T) {
	converted, mappings, warnings := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{
		{Type: "function", Name: "crm__lookup", Parameters: map[string]any{"type": "object"}},
		{Type: "namespace", Name: "crm", Tools: []ResponsesTool{{Type: "function", Name: "lookup", Parameters: map[string]any{"type": "object"}}}},
	})
	if len(converted) != 2 {
		t.Fatalf("converted=%#v", converted)
	}
	first := converted[0].Function.Name
	second := converted[1].Function.Name
	if first == second {
		t.Fatalf("namespace collision was not resolved: %q", first)
	}
	if mappings[first].Kind != "function" || mappings[second].Kind != "namespace" {
		t.Fatalf("mappings=%#v", mappings)
	}
	foundRewrite := false
	for _, warning := range warnings {
		if warning.Code == "tool_name_rewritten" {
			foundRewrite = true
		}
	}
	if !foundRewrite {
		t.Fatalf("collision rewrite warning missing: %#v", warnings)
	}
}

func TestClaudeResponseRejectsInvalidToolArguments(t *testing.T) {
	body := []byte(`{
		"id":"chatcmpl_bad",
		"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{
			"id":"call_bad","type":"function","function":{"name":"dangerous","arguments":"{\"path\":"}
		}]}}]
	}`)
	converted, err := openAIToClaudeResponseWithError(body, "model-c")
	if err == nil {
		t.Fatalf("invalid tool arguments produced a Claude response: %s", converted)
	}
}

func TestResponsesStreamAllocatesUniqueIndexesByArrival(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl_order","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_order","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_order","choices":[{"index":0,"delta":{"content":"after tool"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_order","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(stream)), Header: make(http.Header)}
	recorder := httptest.NewRecorder()
	responsesStreamHandler(recorder, nil, response, "model-r", "model-r", nil, nil, nil, nil, nil)

	indexes := map[int]bool{}
	var completed map[string]any
	for _, event := range parseTestSSE(t, recorder.Body.String()) {
		if event.name == "response.output_item.added" {
			index, ok := event.data["output_index"].(float64)
			if !ok {
				t.Fatalf("output_index=%#v", event.data["output_index"])
			}
			idx := int(index)
			if indexes[idx] {
				t.Fatalf("duplicate output_index=%d\n%s", idx, recorder.Body.String())
			}
			indexes[idx] = true
		}
		if event.name == "response.completed" {
			completed = testObject(t, event.data["response"], "response.completed.response")
		}
	}
	if !reflect.DeepEqual(indexes, map[int]bool{0: true, 1: true}) {
		t.Fatalf("indexes=%#v, want 0 and 1", indexes)
	}
	output := testArray(t, completed["output"], "completed.output")
	if len(output) != 2 {
		t.Fatalf("output=%#v", output)
	}
	requireTestEqual(t, "output[0].type", testObject(t, output[0], "output[0]")["type"], "function_call")
	requireTestEqual(t, "output[1].type", testObject(t, output[1], "output[1]")["type"], "message")
}

func TestChatPassthroughStreamPreservesExactSSEBytes(t *testing.T) {
	raw := "event: chunk\n" +
		`data:{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}` + "\n\n" +
		"data:[DONE]"
	recorder := httptest.NewRecorder()
	if err := proxyChatPassthroughStream(recorder, io.NopCloser(strings.NewReader(raw)), "model-a", false); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != raw {
		t.Fatalf("passthrough changed SSE bytes:\n got: %q\nwant: %q", recorder.Body.String(), raw)
	}
}

func TestChatPassthroughRequestPreservesFutureToolShapes(t *testing.T) {
	raw := []byte(`{
		"model":"client-model",
		"stream":true,
		"stream_options":{"future_option":true},
		"messages":[{"role":"assistant","tool_calls":[{
			"id":"call_custom","type":"custom","custom":{"name":"code_exec","input":"print(1)"}
		}]}],
		"tools":[{"type":"custom","custom":{"name":"code_exec","format":{"type":"text"}}}],
		"future_top_level":{"enabled":true}
	}`)
	converted, err := prepareChatPassthroughBody(raw, "resolved-model", "high", true)
	if err != nil {
		t.Fatal(err)
	}
	body := decodeTestObject(t, converted)
	requireTestEqual(t, "model", body["model"], "resolved-model")
	requireTestEqual(t, "reasoning_effort", body["reasoning_effort"], "high")
	if body["future_top_level"] == nil {
		t.Fatal("future top-level field was lost")
	}
	options := testObject(t, body["stream_options"], "stream_options")
	requireTestEqual(t, "future stream option", options["future_option"], true)
	// 流式透传必须补齐 include_usage，否则兼容上游不回传 usage，用量统计恒为 0。
	requireTestEqual(t, "stream usage opt-in", options["include_usage"], true)
	messages := testArray(t, body["messages"], "messages")
	message := testObject(t, messages[0], "messages[0]")
	requireTestEqual(t, "tool-only assistant content", message["content"], "")
	calls := testArray(t, message["tool_calls"], "messages[0].tool_calls")
	call := testObject(t, calls[0], "messages[0].tool_calls[0]")
	requireTestEqual(t, "custom tool call type", call["type"], "custom")
	if call["custom"] == nil {
		t.Fatalf("custom tool call payload was lost: %#v", call)
	}
}

func TestChatBridgeToolMessagesAlwaysIncludeContent(t *testing.T) {
	messages := []Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID: "call_1", Type: "function",
				Function: FunctionCall{Name: "lookup", Arguments: `{}`},
			}},
		},
		{Role: "tool", ToolCallID: "call_1"},
	}
	converted := convertMessagesForUpstream(messages, false)
	if len(converted) != 2 {
		t.Fatalf("converted=%#v", converted)
	}
	for index, message := range converted {
		content, exists := message["content"]
		if !exists {
			t.Fatalf("messages[%d] is missing required content: %#v", index, message)
		}
		if content != "" {
			t.Fatalf("messages[%d].content=%#v, want empty string", index, content)
		}
	}
}

func TestResponsesToolSearchUsesReversibleFunctionWrapper(t *testing.T) {
	tools, mappings, warnings := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{
		Type: "tool_search", Description: "Find an appropriate tool",
	}})
	if len(tools) != 1 || len(mappings) != 1 {
		t.Fatalf("tools=%#v mappings=%#v", tools, mappings)
	}
	upstreamName := tools[0].Function.Name
	mapping, ok := mappings[upstreamName]
	if !ok || mapping.Kind != "tool_search" {
		t.Fatalf("mapping[%q]=%#v, found=%v", upstreamName, mapping, ok)
	}
	if len(warnings) != 1 || warnings[0].Code != "tool_search_emulated" || warnings[0].Severity != "degraded" {
		t.Fatalf("warnings=%#v", warnings)
	}

	item := responseFunctionCallItem("tsc_1", "completed", `{"query":"filesystem"}`, "call_1", upstreamName, mappings)
	requireTestEqual(t, "item type", item["type"], "tool_search_call")
	requireTestEqual(t, "execution", item["execution"], "client")
	requireTestEqual(t, "search query", testObject(t, item["arguments"], "arguments")["query"], "filesystem")
	requireTestEqual(t, "call id", item["call_id"], "call_1")
}

func TestResponsesToolSearchHistoryRoundTrips(t *testing.T) {
	_, mappings, _ := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{Type: "tool_search"}})
	messages, warnings := responsesInputToMessagesWithWarnings([]any{
		map[string]any{"type": "tool_search_call", "execution": "client", "call_id": "call_search", "arguments": map[string]any{"query": "filesystem"}},
		map[string]any{"type": "tool_search_output", "execution": "client", "call_id": "call_search", "status": "completed", "tools": []any{
			map[string]any{"type": "function", "name": "apply_patch", "parameters": map[string]any{"type": "object"}},
		}},
	}, "", mappings)
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v, want none", warnings)
	}
	if len(messages) != 2 || messages[0].Role != "assistant" || messages[1].Role != "tool" {
		t.Fatalf("messages=%#v", messages)
	}
	wrapperName := responseUpstreamToolName("tool_search", "", "tool_search", mappings)
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Function.Name != wrapperName {
		t.Fatalf("tool search call=%#v", messages[0].ToolCalls)
	}
	requireTestEqual(t, "tool result call id", messages[1].ToolCallID, "call_search")
	loaded := responsesLoadedToolDefinitions([]any{map[string]any{
		"type": "tool_search_output", "tools": []any{map[string]any{"type": "function", "name": "apply_patch"}},
	}})
	if len(loaded) != 1 || loaded[0].Name != "apply_patch" {
		t.Fatalf("loaded tools=%#v", loaded)
	}
}

func TestResponsesHostedWebSearchAutomaticallyUsesChatOptions(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"latest news","tools":[{"type":"web_search"}]
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	if _, exists := calls[0].body["tools"]; exists {
		t.Fatalf("hosted web search leaked upstream: %#v", calls[0].body["tools"])
	}
	options := testObject(t, calls[0].body["web_search_options"], "web_search_options")
	if len(options) != 0 {
		t.Fatalf("unexpected default web search options=%#v", options)
	}
	messages := testArray(t, calls[0].body["messages"], "messages")
	first := testObject(t, messages[0], "messages[0]")
	if first["role"] != "user" {
		t.Fatalf("unexpected compatibility guidance=%#v", first)
	}
}

func TestAnthropicServerWebSearchAutomaticallyMapsToChat(t *testing.T) {
	t.Run("compatible", func(t *testing.T) {
		matrixIsolateRuntime(t)
		upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
		matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

		request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
			"model":"matrix-public-model","max_tokens":32,
			"messages":[{"role":"user","content":"latest news"}],
			"tools":[{"type":"web_search_20250305","name":"web_search"}]
		}`))
		response := httptest.NewRecorder()
		claudeMessagesHandler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
		}
		calls := recorder.snapshot()
		if len(calls) != 1 {
			t.Fatalf("upstream calls=%d, want 1", len(calls))
		}
		if _, exists := calls[0].body["tools"]; exists {
			t.Fatalf("Anthropic server tool leaked upstream: %#v", calls[0].body["tools"])
		}
		if _, ok := calls[0].body["web_search_options"].(map[string]any); !ok {
			t.Fatalf("web_search_options missing: %#v", calls[0].body)
		}
		messages := testArray(t, calls[0].body["messages"], "messages")
		first := testObject(t, messages[0], "messages[0]")
		if first["role"] != "user" {
			t.Fatalf("unexpected compatibility guidance=%#v", first)
		}
	})

	t.Run("strict", func(t *testing.T) {
		matrixIsolateRuntime(t)
		upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
		matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
		request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
			"model":"matrix-public-model","max_tokens":32,
			"messages":[{"role":"user","content":"latest news"}],
			"tools":[{"type":"web_search_20250305","name":"web_search"}]
		}`))
		request.Header.Set("X-Llm2api-Bridge-Mode", "strict")
		response := httptest.NewRecorder()
		claudeMessagesHandler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
		}
		calls := recorder.snapshot()
		if len(calls) != 1 {
			t.Fatalf("strict native request calls=%#v", calls)
		}
		if _, ok := calls[0].body["web_search_options"].(map[string]any); !ok {
			t.Fatalf("strict native web_search_options missing: %#v", calls[0].body)
		}
	})
}

func TestAnthropicServerWebSearchUsesNativeResponsesTool(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamResponses)
	matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"matrix-public-model","max_tokens":32,
		"messages":[{"role":"user","content":"latest news"}],
		"tools":[{"type":"web_search_20250305","name":"web_search","blocked_domains":["example.com"]}]
	}`))
	response := httptest.NewRecorder()
	claudeMessagesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Llm2api-Warning-Count") != "" {
		t.Fatalf("native Responses web search unexpectedly warned: %#v", response.Header())
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	tool := testObject(t, testArray(t, calls[0].body["tools"], "tools")[0], "tools[0]")
	requireTestEqual(t, "tool type", tool["type"], "web_search")
	filters := testObject(t, tool["filters"], "tools[0].filters")
	requireTestEqual(t, "blocked domain", testArray(t, filters["blocked_domains"], "blocked_domains")[0], "example.com")
}

func TestResponsesTextJSONSchemaMapsToChatResponseFormat(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"return json",
		"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object"},"strict":true}}
	}`))
	request.Header.Set("X-Llm2api-Bridge-Mode", "strict")
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", response.Code, response.Body.String())
	}
	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("upstream calls=%d, want 1", len(calls))
	}
	responseFormat := testObject(t, calls[0].body["response_format"], "response_format")
	requireTestEqual(t, "response format type", responseFormat["type"], "json_schema")
	jsonSchema := testObject(t, responseFormat["json_schema"], "response_format.json_schema")
	requireTestEqual(t, "schema name", jsonSchema["name"], "answer")
	requireTestEqual(t, "schema strict", jsonSchema["strict"], true)
	if response.Header().Get("X-Llm2api-Warning-Count") != "" {
		t.Fatalf("supported text.format unexpectedly warned: %#v", response.Header())
	}
}

func TestBridgeWarningSeverityClassification(t *testing.T) {
	fields := map[string]any{
		"include":                []any{"reasoning.encrypted_content"},
		"prompt_cache_key":       "cache-key",
		"prompt_cache_retention": "24h",
		"previous_response_id":   "resp_1",
	}
	warnings := responsesBridgeRequestWarningsForUpstream(fields, &UpstreamConfig{APIType: UpstreamAnthropic})
	severities := map[string]string{}
	for _, warning := range warnings {
		severities[warning.Code+":"+warning.Path] = warning.Severity
	}
	for _, path := range []string{"prompt_cache_key", "prompt_cache_retention"} {
		code := "prompt_cache_hint_ignored"
		if got := severities[code+":"+path]; got != "info" {
			t.Fatalf("%s severity=%q, want info; warnings=%#v", path, got, warnings)
		}
	}
	if got := severities["encrypted_reasoning_unavailable:include[0]"]; got != "degraded" {
		t.Fatalf("encrypted reasoning severity=%q, want degraded; warnings=%#v", got, warnings)
	}
	if got := severities["stateful_context_ignored:previous_response_id"]; got != "degraded" {
		t.Fatalf("stateful context severity=%q, want degraded", got)
	}
}

func TestUpstreamBridgeModeNormalizationAndPrecedence(t *testing.T) {
	upstream := &UpstreamConfig{BaseURL: "https://example.test/v1", APIType: UpstreamOpenAI}
	if !normalizeSingleUpstream(upstream) {
		t.Fatal("valid upstream was rejected")
	}
	if upstream.BridgeMode != BridgeModeCompatible {
		t.Fatalf("default bridge mode=%q, want compatible", upstream.BridgeMode)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Header.Set("X-Llm2api-Bridge-Mode", "strict")
	if got := effectiveBridgeMode(request, upstream); got != BridgeModeStrict {
		t.Fatalf("request strict mode=%q, want strict", got)
	}

	upstream.BridgeMode = BridgeModeStrict
	request.Header.Set("X-Llm2api-Bridge-Mode", "compatible")
	if got := effectiveBridgeMode(request, upstream); got != BridgeModeStrict {
		t.Fatalf("configured strict mode was loosened to %q", got)
	}
}

func TestConfiguredStrictBridgeModeAppliesWithoutRequestHeader(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	configMu.Lock()
	upstreamCfgs["matrix"].BridgeMode = BridgeModeStrict
	upstreamCfg = cloneUpstreamConfig(upstreamCfgs["matrix"])
	configMu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"latest news","tools":[{"type":"computer_use_preview"}]
	}`))
	// 客户端不能放宽管理员配置为 strict 的上游策略。
	request.Header.Set("X-Llm2api-Bridge-Mode", "compatible")
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Llm2api-Bridge-Mode") != "strict" {
		t.Fatalf("bridge mode header=%q, want strict", response.Header().Get("X-Llm2api-Bridge-Mode"))
	}
	if calls := recorder.snapshot(); len(calls) != 0 {
		t.Fatalf("configured strict request reached upstream: %#v", calls)
	}
}

func TestConfiguredStrictBridgeModeAlsoCoversChatSurface(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamAnthropic)
	matrixSelectUpstream(upstreamServer.URL, UpstreamAnthropic)
	configMu.Lock()
	upstreamCfgs["matrix"].BridgeMode = BridgeModeStrict
	upstreamCfg = cloneUpstreamConfig(upstreamCfgs["matrix"])
	configMu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model":"matrix-public-model","messages":[{"role":"user","content":"hello"}],
		"tools":[{"type":"custom","name":"code_exec"}]
	}`))
	response := httptest.NewRecorder()
	chatCompletionsHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", response.Code, response.Body.String())
	}
	if calls := recorder.snapshot(); len(calls) != 0 {
		t.Fatalf("configured strict Chat bridge reached upstream: %#v", calls)
	}
}
