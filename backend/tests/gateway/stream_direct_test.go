package gateway

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func testNamedSSE(name, data string) string {
	return "event: " + name + "\n" + "data: " + data + "\n\n"
}

func eventNames(events []parsedTestSSEEvent) map[string]bool {
	names := make(map[string]bool, len(events))
	for _, event := range events {
		names[event.name] = true
	}
	return names
}

func findTestSSEEvent(t *testing.T, events []parsedTestSSEEvent, name string) parsedTestSSEEvent {
	t.Helper()
	for _, event := range events {
		if event.name == name {
			return event
		}
	}
	t.Fatalf("event %q not found in %#v", name, eventNames(events))
	return parsedTestSSEEvent{}
}

func TestAnthropicStreamToResponsesUsesDirectSemanticEvents(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("message_start", `{"type":"message_start","message":{"id":"msg_native","usage":{"input_tokens":3,"output_tokens":0}}}`),
		testNamedSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`),
		testNamedSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan"}}`),
		testNamedSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_native"}}`),
		testNamedSSE("content_block_stop", `{"type":"content_block_stop","index":0}`),
		testNamedSSE("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`),
		testNamedSSE("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hello"}}`),
		testNamedSSE("content_block_stop", `{"type":"content_block_stop","index":1}`),
		testNamedSSE("content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}}`),
		testNamedSSE("content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"x\":1}"}}`),
		testNamedSSE("content_block_stop", `{"type":"content_block_stop","index":2}`),
		testNamedSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`),
		testNamedSSE("message_stop", `{"type":"message_stop"}`),
	}, "")
	recorder := httptest.NewRecorder()
	anthropicStreamToResponsesDirectHandler(
		recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model", BridgeModeCompatible,
		nil, nil, nil, nil, nil,
	)
	events := parseTestSSE(t, recorder.Body.String())
	names := eventNames(events)
	for _, required := range []string{
		"response.created", "response.reasoning_summary_text.delta",
		"response.output_text.delta", "response.function_call_arguments.delta",
		"response.completed",
	} {
		if !names[required] {
			t.Fatalf("missing %s; got %#v", required, names)
		}
	}
	for _, event := range events {
		if event.data["object"] == "chat.completion.chunk" {
			t.Fatalf("direct bridge emitted Chat intermediary: %#v", event.data)
		}
	}
	completed := testObject(t, findTestSSEEvent(t, events, "response.completed").data["response"], "completed.response")
	requireTestEqual(t, "completed.status", completed["status"], "completed")
	output := testArray(t, completed["output"], "completed.output")
	if len(output) != 3 {
		t.Fatalf("completed output=%#v, want reasoning, message and function_call", output)
	}
	requireTestEqual(t, "output[0].type", testObject(t, output[0], "output[0]")["type"], "reasoning")
	requireTestEqual(t, "output[0].anthropic_signature", testObject(t, output[0], "output[0]")["anthropic_signature"], "sig_native")
	if _, exists := testObject(t, output[0], "output[0]")["encrypted_content"]; exists {
		t.Fatalf("Anthropic thinking signature must not be exposed as OpenAI encrypted_content")
	}
	requireTestEqual(t, "output[1].type", testObject(t, output[1], "output[1]")["type"], "message")
	requireTestEqual(t, "output[2].type", testObject(t, output[2], "output[2]")["type"], "function_call")
}

func TestResponsesStreamToAnthropicUsesDirectSemanticEvents(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_native","usage":{"input_tokens":4,"output_tokens":0}}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_1","type":"reasoning","anthropic_signature":"sig_response"}}`),
		testNamedSSE("response.reasoning_summary_text.delta", `{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"delta":"plan"}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","anthropic_signature":"sig_response"}}`),
		testNamedSSE("response.content_part.added", `{"type":"response.content_part.added","item_id":"msg_1","output_index":1,"content_index":0,"part":{"type":"output_text","text":""}}`),
		testNamedSSE("response.output_text.delta", `{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"hello"}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message"}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`),
		testNamedSSE("response.function_call_arguments.delta", `{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"{\"x\":1}"}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"}}`),
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"id":"resp_native","usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}}`),
	}, "")
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(
		recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model",
	)
	events := parseTestSSE(t, recorder.Body.String())
	names := eventNames(events)
	for _, required := range []string{
		"message_start", "content_block_start", "content_block_delta",
		"content_block_stop", "message_delta", "message_stop",
	} {
		if !names[required] {
			t.Fatalf("missing %s; got %#v", required, names)
		}
	}
	var thinking, signature, text, arguments string
	for _, event := range events {
		if event.name != "content_block_delta" {
			continue
		}
		delta := testObject(t, event.data["delta"], "content_block_delta.delta")
		switch delta["type"] {
		case "thinking_delta":
			thinking += delta["thinking"].(string)
		case "signature_delta":
			signature += delta["signature"].(string)
		case "text_delta":
			text += delta["text"].(string)
		case "input_json_delta":
			arguments += delta["partial_json"].(string)
		}
	}
	requireTestEqual(t, "thinking", thinking, "plan")
	requireTestEqual(t, "signature", signature, "sig_response")
	requireTestEqual(t, "text", text, "hello")
	requireTestEqual(t, "arguments", arguments, `{"x":1}`)
	delta := testObject(t, findTestSSEEvent(t, events, "message_delta").data["delta"], "message_delta.delta")
	requireTestEqual(t, "stop_reason", delta["stop_reason"], "tool_use")
}

func TestResponsesWebSearchStreamMapsToAnthropicServerBlocks(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_web","usage":{"input_tokens":2,"output_tokens":0}}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"ws_1","type":"web_search_call","status":"in_progress"}}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"ws_1","type":"web_search_call","status":"completed","action":{"type":"search","query":"news","sources":[{"type":"url","url":"https://example.com/a","title":"A"}]}}}`),
		testNamedSSE("response.content_part.added", `{"type":"response.content_part.added","item_id":"msg_1","output_index":1,"content_index":0,"part":{"type":"output_text","text":""}}`),
		testNamedSSE("response.output_text.delta", `{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"answer"}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message"}}`),
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"id":"resp_web","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`),
	}, "")
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model")
	events := parseTestSSE(t, recorder.Body.String())
	if eventNames(events)["error"] {
		t.Fatalf("web search stream unexpectedly failed: %#v", events)
	}
	seen := map[string]bool{}
	for _, event := range events {
		if event.name != "content_block_start" {
			continue
		}
		block := testObject(t, event.data["content_block"], "content_block_start.content_block")
		seen[bridgeString(block["type"])] = true
	}
	for _, blockType := range []string{"server_tool_use", "web_search_tool_result", "text"} {
		if !seen[blockType] {
			t.Fatalf("missing Anthropic %s block; seen=%#v", blockType, seen)
		}
	}
	delta := testObject(t, findTestSSEEvent(t, events, "message_delta").data["delta"], "message_delta.delta")
	requireTestEqual(t, "stop_reason", delta["stop_reason"], "end_turn")
	usage := testObject(t, findTestSSEEvent(t, events, "message_delta").data["usage"], "message_delta.usage")
	serverToolUse := testObject(t, usage["server_tool_use"], "message_delta.usage.server_tool_use")
	requireTestEqual(t, "web search request count", serverToolUse["web_search_requests"], float64(1))
}

func TestChatWebSearchAnnotationsMapToAnthropicStreamUsage(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"id":"chat_web","choices":[{"index":0,"delta":{"role":"assistant","content":"Current weather.","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/weather"}}]},"finish_reason":null}]}`,
		`data: {"id":"chat_web","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	recorder := httptest.NewRecorder()
	claudeStreamHandler(recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model")
	events := parseTestSSE(t, recorder.Body.String())
	usage := testObject(t, findTestSSEEvent(t, events, "message_delta").data["usage"], "message_delta.usage")
	serverToolUse := testObject(t, usage["server_tool_use"], "message_delta.usage.server_tool_use")
	requireTestEqual(t, "web search request count", serverToolUse["web_search_requests"], float64(1))
}

func TestResponsesWebSearchStreamMissingDoneFailsWithoutInvalidStop(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_web"}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"ws_pending","type":"web_search_call","status":"in_progress"}}`),
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"id":"resp_web","status":"completed"}}`),
	}, "")
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model")
	events := parseTestSSE(t, recorder.Body.String())
	names := eventNames(events)
	if !names["error"] {
		t.Fatalf("missing output_item.done must fail explicitly: %#v", names)
	}
	if names["message_stop"] || names["content_block_stop"] {
		t.Fatalf("pending web search must not emit unmatched stop events: %#v", names)
	}
	errorObject := testObject(t, findTestSSEEvent(t, events, "error").data["error"], "error.error")
	if !strings.Contains(bridgeString(errorObject["message"]), "output item was finalized") {
		t.Fatalf("unexpected error message: %#v", errorObject)
	}
}

func TestDirectStreamKeepsRedactedThinkingSeparateFromSignature(t *testing.T) {
	anthropicRaw := strings.Join([]string{
		testNamedSSE("message_start", `{"type":"message_start","message":{"id":"msg_redacted","usage":{"input_tokens":2,"output_tokens":0}}}`),
		testNamedSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"opaque-anthropic-state"}}`),
		testNamedSSE("content_block_stop", `{"type":"content_block_stop","index":0}`),
		testNamedSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`),
		testNamedSSE("message_stop", `{"type":"message_stop"}`),
	}, "")
	responsesRecorder := httptest.NewRecorder()
	anthropicStreamToResponsesDirectHandler(
		responsesRecorder, io.NopCloser(strings.NewReader(anthropicRaw)), "client-model", "usage-model", BridgeModeCompatible,
		nil, nil, nil, nil, nil,
	)
	responsesEvents := parseTestSSE(t, responsesRecorder.Body.String())
	completed := testObject(t, findTestSSEEvent(t, responsesEvents, "response.completed").data["response"], "completed.response")
	item := testObject(t, testArray(t, completed["output"], "completed.output")[0], "completed.output[0]")
	requireTestEqual(t, "encrypted_content", item["encrypted_content"], "opaque-anthropic-state")
	if _, exists := item["anthropic_signature"]; exists {
		t.Fatalf("redacted thinking must not be exposed as an Anthropic thinking signature")
	}

	responsesRaw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_redacted","usage":{"input_tokens":2,"output_tokens":0}}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_redacted","type":"reasoning"}}`),
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_redacted","type":"reasoning","encrypted_content":"opaque-openai-state"}}`),
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"id":"resp_redacted","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`),
	}, "")
	anthropicRecorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(
		anthropicRecorder, io.NopCloser(strings.NewReader(responsesRaw)), "client-model", "usage-model",
	)
	anthropicEvents := parseTestSSE(t, anthropicRecorder.Body.String())
	start := findTestSSEEvent(t, anthropicEvents, "content_block_start")
	block := testObject(t, start.data["content_block"], "content_block_start.content_block")
	requireTestEqual(t, "redacted type", block["type"], "redacted_thinking")
	requireTestEqual(t, "redacted data", block["data"], "opaque-openai-state")
	for _, event := range anthropicEvents {
		if event.name != "content_block_delta" {
			continue
		}
		delta := testObject(t, event.data["delta"], "content_block_delta.delta")
		if delta["type"] == "signature_delta" {
			t.Fatalf("OpenAI encrypted_content must not be emitted as Anthropic signature_delta")
		}
	}
}

func TestResponsesIncompleteReasonMapsToAnthropicStopReason(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_incomplete","usage":{"input_tokens":1,"output_tokens":0}}}`),
		testNamedSSE("response.incomplete", `{"type":"response.incomplete","response":{"id":"resp_incomplete","incomplete_details":{"reason":"content_filter"},"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`),
	}, "")
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model")
	events := parseTestSSE(t, recorder.Body.String())
	delta := testObject(t, findTestSSEEvent(t, events, "message_delta").data["delta"], "message_delta.delta")
	requireTestEqual(t, "stop_reason", delta["stop_reason"], "refusal")
}

func TestResponsesUnknownOutputItemFailsExplicitly(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp_unknown","usage":{"input_tokens":1,"output_tokens":0}}}`),
		testNamedSSE("response.output_item.added", `{"type":"response.output_item.added","output_index":0,"item":{"id":"img_1","type":"computer_screenshot"}}`),
	}, "")
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model")
	events := parseTestSSE(t, recorder.Body.String())
	if !eventNames(events)["error"] {
		t.Fatalf("unsupported output item must emit an explicit Anthropic error: %#v", eventNames(events))
	}
	if eventNames(events)["message_stop"] {
		t.Fatalf("unsupported output item must not be reported as a successful completed message")
	}
}

func TestAnthropicUnknownBlockFailsInStrictMode(t *testing.T) {
	raw := strings.Join([]string{
		testNamedSSE("message_start", `{"type":"message_start","message":{"id":"msg_unknown","usage":{"input_tokens":1,"output_tokens":0}}}`),
		testNamedSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"future_server_result","payload":{}}}`),
	}, "")
	recorder := httptest.NewRecorder()
	anthropicStreamToResponsesDirectHandler(
		recorder, io.NopCloser(strings.NewReader(raw)), "client-model", "usage-model", BridgeModeStrict,
		nil, nil, nil, nil, nil,
	)
	events := parseTestSSE(t, recorder.Body.String())
	if !eventNames(events)["response.failed"] || !eventNames(events)["error"] {
		t.Fatalf("strict unsupported block must fail explicitly: %#v", eventNames(events))
	}
	if eventNames(events)["response.incomplete"] || eventNames(events)["response.completed"] {
		t.Fatalf("strict unsupported block must not be reported as a partial success")
	}
}
