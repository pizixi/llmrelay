package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func decodePairwiseTestObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", body, err)
	}
	return value
}

func findPairwiseItem(t *testing.T, items []any, itemType string) map[string]any {
	t.Helper()
	for _, raw := range items {
		item := testObject(t, raw, "item")
		if item["type"] == itemType {
			return item
		}
	}
	t.Fatalf("item type %q not found in %#v", itemType, items)
	return nil
}

func TestAnthropicRequestToResponsesDirectPreservesSemanticBlocks(t *testing.T) {
	body := []byte(`{
		"model":"claude-alias","system":[{"type":"text","text":"be exact"}],
		"max_tokens":9007199254740993,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"plan","signature":"sig-a"},
				{"type":"redacted_thinking","data":"opaque-a"},
				{"type":"text","text":"hello"},
				{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"x":1}}
			]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}
		],
		"tools":[{"name":"lookup","description":"lookup","input_schema":{"type":"object","properties":{"x":{"type":"integer"}}}}],
		"tool_choice":{"type":"tool","name":"lookup","disable_parallel_tool_use":true}
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings=%#v", warnings)
	}
	if !strings.Contains(string(converted), `"max_output_tokens":9007199254740993`) {
		t.Fatalf("large integer lost precision: %s", converted)
	}
	request := decodePairwiseTestObject(t, converted)
	requireTestEqual(t, "model", request["model"], "resolved-model")
	requireTestEqual(t, "instructions", request["instructions"], "be exact")
	items := testArray(t, request["input"], "input")
	reasoningItems := []map[string]any{}
	for _, raw := range items {
		item := testObject(t, raw, "input item")
		if item["type"] == "reasoning" {
			reasoningItems = append(reasoningItems, item)
		}
	}
	if len(reasoningItems) != 2 {
		t.Fatalf("reasoning items=%#v", reasoningItems)
	}
	requireTestEqual(t, "signature", reasoningItems[0]["anthropic_signature"], "sig-a")
	if _, exists := reasoningItems[0]["encrypted_content"]; exists {
		t.Fatalf("signature was confused with encrypted content")
	}
	requireTestEqual(t, "encrypted", reasoningItems[1]["encrypted_content"], "opaque-a")
	call := findPairwiseItem(t, items, "function_call")
	requireTestEqual(t, "call_id", call["call_id"], "toolu_1")
	result := findPairwiseItem(t, items, "function_call_output")
	requireTestEqual(t, "result call_id", result["call_id"], "toolu_1")
	choice := testObject(t, request["tool_choice"], "tool_choice")
	requireTestEqual(t, "tool choice name", choice["name"], "lookup")
	requireTestEqual(t, "parallel", request["parallel_tool_calls"], false)
}

func TestAnthropicSystemCacheControlWarnsAsInfo(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"hello"}],
		"system":[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}}]
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "system_cache_control_ignored" || warnings[0].Severity != "info" {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	requireTestEqual(t, "instructions", request["instructions"], "cached")
}

func TestInvalidAnthropicSystemShapeWarnsWhenSerialized(t *testing.T) {
	body := []byte(`{"system":{"unexpected":true},"messages":[{"role":"user","content":"hello"}]}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "invalid_system_shape" || warnings[0].Severity != "degraded" {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	requireTestEqual(t, "serialized instructions", request["instructions"], `{"unexpected":true}`)
}

func TestAnthropicToolReferenceIsPreservedAsToolOutputText(t *testing.T) {
	body := []byte(`{
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"search_tools","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"tool_reference","tool_name":"lookup_order"}]}]}
		]
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "tool_reference_downgraded" {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	result := findPairwiseItem(t, testArray(t, request["input"], "input"), "function_call_output")
	parts := testArray(t, result["output"], "function_call_output.output")
	part := testObject(t, parts[0], "function_call_output.output[0]")
	requireTestEqual(t, "tool reference text", part["text"], "[tool_reference] lookup_order")
}

func TestAnthropicWebSearchMapsToResponsesHostedTool(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"latest news"}],
		"tools":[{"type":"web_search_20250305","name":"web_search","allowed_domains":["example.com"],"user_location":{"type":"approximate","country":"CN"}}],
		"tool_choice":{"type":"tool","name":"web_search"}
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	tool := testObject(t, testArray(t, request["tools"], "tools")[0], "tools[0]")
	requireTestEqual(t, "tool type", tool["type"], "web_search")
	filters := testObject(t, tool["filters"], "tools[0].filters")
	requireTestEqual(t, "allowed domain", testArray(t, filters["allowed_domains"], "allowed_domains")[0], "example.com")
	choice := testObject(t, request["tool_choice"], "tool_choice")
	requireTestEqual(t, "tool choice type", choice["type"], "web_search")
	requireTestEqual(t, "include sources", testArray(t, request["include"], "include")[0], "web_search_call.action.sources")
}

func TestFunctionNamedWebSearchKeepsFunctionToolChoice(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"search"}],
		"tools":[{"name":"web_search","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"tool","name":"web_search"}
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	choice := testObject(t, request["tool_choice"], "tool_choice")
	requireTestEqual(t, "tool choice type", choice["type"], "function")
	requireTestEqual(t, "tool choice name", choice["name"], "web_search")
}

func TestAnthropicWebSearchReplayOmitsOpaqueEncryptedContent(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"assistant","content":[
			{"type":"web_search_tool_result","tool_use_id":"ws_1","content":[
				{"type":"web_search_result","url":"https://example.com/a","title":"A","encrypted_content":"opaque-secret"}
			]}
		]}]
	}`)
	converted, warnings, err := convertAnthropicRequestToResponsesDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "web_search_results_downgraded" {
		t.Fatalf("warnings=%#v", warnings)
	}
	if strings.Contains(string(converted), "opaque-secret") {
		t.Fatalf("opaque Anthropic carrier leaked into Responses text: %s", converted)
	}
	request := decodePairwiseTestObject(t, converted)
	message := findPairwiseItem(t, testArray(t, request["input"], "input"), "message")
	part := testObject(t, testArray(t, message["content"], "content")[0], "content[0]")
	requireTestEqual(t, "assistant replay part type", part["type"], "output_text")
	if !strings.Contains(bridgeString(part["text"]), "https://example.com/a") {
		t.Fatalf("search metadata was not preserved: %#v", part)
	}
}

func TestWebSearchMappingsAcceptMissingInputAndAction(t *testing.T) {
	items, warnings := anthropicContentToResponsesItems("assistant", []any{
		map[string]any{"type": "server_tool_use", "id": "ws_1", "name": "web_search"},
	}, "messages[0].content")
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	call := findPairwiseItem(t, items, "web_search_call")
	action := testObject(t, call["action"], "web_search_call.action")
	requireTestEqual(t, "default action type", action["type"], "search")

	blocks := responsesWebSearchToAnthropicBlocks(map[string]any{"type": "web_search_call", "id": "ws_2"})
	serverUse := findPairwiseItem(t, blocks, "server_tool_use")
	if len(testObject(t, serverUse["input"], "server_tool_use.input")) != 0 {
		t.Fatalf("missing action should become empty input: %#v", serverUse)
	}
}

func TestResponsesWebSearchCallMapsToAnthropicServerBlocks(t *testing.T) {
	body := []byte(`{
		"id":"resp_web","status":"completed","usage":{"input_tokens":1,"output_tokens":2},
		"output":[
			{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"news","sources":[{"type":"url","url":"https://example.com/a","title":"A"}]}},
			{"type":"message","content":[{"type":"output_text","text":"answer"}]}
		]
	}`)
	converted, warnings, err := convertResponsesResponseToAnthropicDirect(body, "client-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	message := decodePairwiseTestObject(t, converted)
	blocks := testArray(t, message["content"], "content")
	serverUse := findPairwiseItem(t, blocks, "server_tool_use")
	requireTestEqual(t, "server tool id", serverUse["id"], "ws_1")
	result := findPairwiseItem(t, blocks, "web_search_tool_result")
	sources := testArray(t, result["content"], "web_search result content")
	requireTestEqual(t, "source url", testObject(t, sources[0], "source")["url"], "https://example.com/a")
	usage := testObject(t, message["usage"], "usage")
	serverToolUsage := testObject(t, usage["server_tool_use"], "usage.server_tool_use")
	requireTestEqual(t, "web search request count", serverToolUsage["web_search_requests"], float64(1))
}

func TestResponsesRequestToAnthropicDirectPreservesItemsAndCarriers(t *testing.T) {
	body := []byte(`{
		"model":"gpt-alias","instructions":"be concise","max_output_tokens":2048,
		"input":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}],"anthropic_signature":"sig-r"},
			{"type":"reasoning","summary":[],"encrypted_content":"opaque-r"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]},
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"}
		],
		"tools":[{"type":"function","name":"lookup","description":"lookup","parameters":{"type":"object"}}],
		"tool_choice":{"type":"function","name":"lookup"},"parallel_tool_calls":false,
		"reasoning":{"effort":"low"}
	}`)
	converted, mappings, warnings, err := convertResponsesRequestToAnthropicDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "reasoning_effort_approximated" {
		t.Fatalf("reasoning bridge warnings: %#v", warnings)
	}
	if len(mappings) != 1 {
		t.Fatalf("mappings=%#v", mappings)
	}
	request := decodePairwiseTestObject(t, converted)
	requireTestEqual(t, "system", request["system"], "be concise")
	requireTestEqual(t, "max_tokens", request["max_tokens"], float64(2048))
	messages := testArray(t, request["messages"], "messages")
	var allBlocks []any
	for _, raw := range messages {
		message := testObject(t, raw, "message")
		allBlocks = append(allBlocks, testArray(t, message["content"], "message.content")...)
	}
	thinking := findPairwiseItem(t, allBlocks, "thinking")
	requireTestEqual(t, "thinking signature", thinking["signature"], "sig-r")
	redacted := findPairwiseItem(t, allBlocks, "redacted_thinking")
	requireTestEqual(t, "redacted data", redacted["data"], "opaque-r")
	toolUse := findPairwiseItem(t, allBlocks, "tool_use")
	requireTestEqual(t, "tool use id", toolUse["id"], "call_1")
	findPairwiseItem(t, allBlocks, "tool_result")
	choice := testObject(t, request["tool_choice"], "tool_choice")
	requireTestEqual(t, "choice type", choice["type"], "tool")
	requireTestEqual(t, "disable parallel", choice["disable_parallel_tool_use"], true)
	thinkingConfig := testObject(t, request["thinking"], "thinking")
	requireTestEqual(t, "thinking type", thinkingConfig["type"], "enabled")
}

func TestPairwiseNonStreamResponsesPreserveReasoningCarriers(t *testing.T) {
	anthropicBody := []byte(`{
		"id":"msg_a","type":"message","role":"assistant","model":"claude",
		"content":[
			{"type":"thinking","thinking":"plan","signature":"sig-a"},
			{"type":"redacted_thinking","data":"opaque-a"},
			{"type":"text","text":"answer"},
			{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"x":1}}
		],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":4}
	}`)
	request := map[string]any{"tools": []any{map[string]any{"type": "function", "name": "lookup"}}}
	responsesBody, _, err := convertAnthropicResponseToResponsesDirect(anthropicBody, "client-model", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := decodePairwiseTestObject(t, responsesBody)
	items := testArray(t, response["output"], "output")
	var reasoning []map[string]any
	for _, raw := range items {
		item := testObject(t, raw, "output item")
		if item["type"] == "reasoning" {
			reasoning = append(reasoning, item)
		}
	}
	if len(reasoning) != 2 {
		t.Fatalf("reasoning=%#v", reasoning)
	}
	requireTestEqual(t, "signature", reasoning[0]["anthropic_signature"], "sig-a")
	requireTestEqual(t, "encrypted", reasoning[1]["encrypted_content"], "opaque-a")

	responsesRaw := []byte(`{
		"id":"resp_r","object":"response","status":"incomplete","model":"gpt",
		"incomplete_details":{"reason":"model_context_window_exceeded"},
		"output":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}],"anthropic_signature":"sig-r"},
			{"type":"reasoning","summary":[],"encrypted_content":"opaque-r"},
			{"type":"message","content":[{"type":"output_text","text":"partial"}]},
			{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"}
		],"usage":{"input_tokens":5,"output_tokens":6,"total_tokens":11}
	}`)
	anthropicConverted, warnings, err := convertResponsesResponseToAnthropicDirect(responsesRaw, "client-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings=%#v", warnings)
	}
	message := decodePairwiseTestObject(t, anthropicConverted)
	requireTestEqual(t, "stop reason", message["stop_reason"], "model_context_window_exceeded")
	blocks := testArray(t, message["content"], "content")
	requireTestEqual(t, "signature", findPairwiseItem(t, blocks, "thinking")["signature"], "sig-r")
	requireTestEqual(t, "encrypted", findPairwiseItem(t, blocks, "redacted_thinking")["data"], "opaque-r")
}

func TestResponsesReasoningWithoutAnthropicSignatureDowngradesExplicitly(t *testing.T) {
	body := []byte(`{
		"model":"gpt-alias","input":[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"portable summary"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)
	converted, _, warnings, err := convertResponsesRequestToAnthropicDirect(body, "resolved-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "reasoning_summary_downgraded" {
		t.Fatalf("warnings=%#v", warnings)
	}
	request := decodePairwiseTestObject(t, converted)
	messages := testArray(t, request["messages"], "messages")
	first := testObject(t, messages[0], "messages[0]")
	block := testObject(t, testArray(t, first["content"], "messages[0].content")[0], "messages[0].content[0]")
	requireTestEqual(t, "downgraded type", block["type"], "text")
	requireTestEqual(t, "downgraded text", block["text"], "portable summary")
}

func TestAnthropicResponseDirectRestoresResponsesCustomToolKind(t *testing.T) {
	body := []byte(`{
		"id":"msg_custom","type":"message","role":"assistant","model":"claude",
		"content":[{"type":"tool_use","id":"call_custom","name":"custom__shell","input":{"input":"ls"}}],
		"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}
	}`)
	mappings := map[string]ResponseToolNameMapping{
		"custom__shell": {Kind: "custom", Name: "shell"},
	}
	converted, _, err := convertAnthropicResponseToResponsesDirect(body, "client-model", map[string]any{}, mappings)
	if err != nil {
		t.Fatal(err)
	}
	response := decodePairwiseTestObject(t, converted)
	item := testObject(t, testArray(t, response["output"], "output")[0], "output[0]")
	requireTestEqual(t, "custom item type", item["type"], "custom_tool_call")
	requireTestEqual(t, "custom name", item["name"], "shell")
	requireTestEqual(t, "custom input", item["input"], "ls")
}
