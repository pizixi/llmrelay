package websearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func ContainsAnthropicHostedWebSearch(tools []ClaudeTool) bool {
	for _, tool := range tools {
		if strings.Contains(strings.ToLower(strings.TrimSpace(tool.Type)), "web_search") {
			return true
		}
	}
	return false
}

func RequestContainsAnthropicHostedWebSearch(body []byte) bool {
	var request struct {
		Tools []ClaudeTool `json:"tools"`
	}
	return json.Unmarshal(body, &request) == nil && ContainsAnthropicHostedWebSearch(request.Tools)
}

func RequestRequiresAnthropicHostedWebSearch(body []byte) bool {
	var request struct {
		Tools      []ClaudeTool `json:"tools"`
		ToolChoice any          `json:"tool_choice"`
	}
	if json.Unmarshal(body, &request) != nil || !ContainsAnthropicHostedWebSearch(request.Tools) {
		return false
	}
	hostedTools := 0
	for _, tool := range request.Tools {
		if strings.Contains(strings.ToLower(strings.TrimSpace(tool.Type)), "web_search") {
			hostedTools++
		}
	}
	return HostedWebSearchChoiceRequired(request.ToolChoice, len(request.Tools), hostedTools)
}

func AnthropicWebSearchOptionsForChat(tools []ClaudeTool) map[string]any {
	for _, tool := range tools {
		if !strings.Contains(strings.ToLower(strings.TrimSpace(tool.Type)), "web_search") {
			continue
		}
		options := map[string]any{}
		if tool.UserLocation != nil {
			options["user_location"] = tool.UserLocation
		}
		return options
	}
	return nil
}

func FilterAnthropicNativeChatWebSearchWarnings(warnings []BridgeWarning, tools []ClaudeTool) []BridgeWarning {
	paths := map[string]bool{}
	for index, tool := range tools {
		if strings.Contains(strings.ToLower(strings.TrimSpace(tool.Type)), "web_search") {
			paths[fmt.Sprintf("tools[%d]", index)] = true
		}
	}
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning.Code == "unsupported_anthropic_server_tool" && paths[warning.Path] {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func RewriteAnthropicWebSearchToolChoice(choice any) any {
	choiceMap, _ := choice.(map[string]any)
	if choiceMap == nil {
		return choice
	}
	function, _ := choiceMap["function"].(map[string]any)
	name, _ := function["name"].(string)
	if name == "web_search" {
		cloned := CloneAnyMap(choiceMap)
		cloned["function"] = map[string]any{"name": internalWebSearchToolName}
		return cloned
	}
	return choice
}

func AnthropicWebSearchRequestsFromUsage(usage map[string]any) int {
	serverToolUse, _ := usage["server_tool_use"].(map[string]any)
	requests, _ := GetFloat(serverToolUse, "web_search_requests")
	return int(requests)
}

func WithAnthropicWebSearchUsage(usage map[string]any, requests int) map[string]any {
	if usage == nil {
		usage = map[string]any{}
	}
	if requests <= AnthropicWebSearchRequestsFromUsage(usage) {
		return usage
	}
	serverToolUse, _ := usage["server_tool_use"].(map[string]any)
	serverToolUse = CloneAnyMap(serverToolUse)
	serverToolUse["web_search_requests"] = requests
	usage["server_tool_use"] = serverToolUse
	return usage
}

func ChatWebSearchBlocks(providerOutput []any) ([]ClaudeContent, int) {
	var blocks []ClaudeContent
	requests := 0
	for index, raw := range providerOutput {
		item, _ := raw.(map[string]any)
		if BridgeString(item["type"]) != "web_search_call" {
			continue
		}
		requests++
		callID := BridgeString(item["id"])
		if callID == "" {
			callID = fmt.Sprintf("srvtoolu_chat_%d", index)
		}
		action, _ := item["action"].(map[string]any)
		input := CloneAnyMap(action)
		delete(input, "sources")
		delete(input, "type")
		blocks = append(blocks, ClaudeContent{
			Type: "server_tool_use", ID: callID, Name: "web_search", Input: input,
		})
		results := make([]any, 0)
		for _, rawSource := range BridgeArray(action["sources"]) {
			source, _ := rawSource.(map[string]any)
			url := BridgeString(source["url"])
			if url == "" {
				continue
			}
			title := BridgeString(source["title"])
			if title == "" {
				title = url
			}
			results = append(results, map[string]any{
				"type": "web_search_result", "url": url, "title": title,
			})
		}
		blocks = append(blocks, ClaudeContent{
			Type: "web_search_tool_result", ToolUseID: callID, Content: results,
		})
	}
	return blocks, requests
}

func ChatWebSearchEvidenceCount(providerOutput, annotations []any) int {
	_, requests := ChatWebSearchBlocks(providerOutput)
	if requests == 0 && HasWebSearchAnnotations(annotations) {
		// Chat hosted search normally exposes citations only. Citations prove that
		// at least one server-side search ran even though Chat has no request count.
		return 1
	}
	return requests
}

func HasWebSearchAnnotations(annotations []any) bool {
	for _, raw := range annotations {
		annotation, _ := raw.(map[string]any)
		if strings.EqualFold(BridgeString(annotation["type"]), "url_citation") {
			return true
		}
		if citation, ok := annotation["url_citation"].(map[string]any); ok && BridgeString(citation["url"]) != "" {
			return true
		}
	}
	return false
}

func InjectAnthropicWebSearchMetadata(body []byte, traces []Trace) ([]byte, error) {
	if len(traces) == 0 {
		return body, nil
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Anthropic fallback response: %w", err)
	}
	content, _ := response["content"].([]any)
	blocks := make([]any, 0, len(traces)*2+len(content))
	for index, trace := range traces {
		callID := strings.TrimSpace(trace.CallID)
		if callID == "" {
			callID = fmt.Sprintf("srvtoolu_gateway_%d", index)
		}
		blocks = append(blocks, map[string]any{
			"type": "server_tool_use", "id": callID, "name": "web_search",
			"input": map[string]any{"query": trace.Query},
		})
		var resultContent any
		if trace.Error != "" {
			resultContent = map[string]any{
				"type": "web_search_tool_result_error", "error_code": "unavailable",
			}
		} else {
			results := make([]any, 0, len(trace.Results))
			for _, result := range trace.Results {
				results = append(results, map[string]any{
					"type": "web_search_result", "url": result.URL, "title": result.Title,
				})
			}
			resultContent = results
		}
		blocks = append(blocks, map[string]any{
			"type": "web_search_tool_result", "tool_use_id": callID, "content": resultContent,
		})
	}
	response["content"] = append(blocks, content...)
	usage, _ := response["usage"].(map[string]any)
	response["usage"] = WithAnthropicWebSearchUsage(usage, len(traces))
	encoded, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic fallback response: %w", err)
	}
	return encoded, nil
}

// WriteBufferedAnthropicStream 在网关托管的搜索循环完成后输出完整的 Anthropic SSE 序列。
// 原生上游流仍保持透传，不经过此路径缓冲。
func WriteBufferedAnthropicStream(w http.ResponseWriter, body []byte) {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		WriteClientAPIError(w, WireAnthropic, http.StatusBadGateway, "upstream_protocol_error", "invalid buffered Anthropic result")
		return
	}
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	emit := func(event string, payload map[string]any) {
		payload["type"] = event
		encoded, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
		if flusher != nil {
			flusher.Flush()
		}
	}
	started := CloneJSONMap(response)
	started["content"] = []any{}
	started["stop_reason"] = nil
	started["stop_sequence"] = nil
	if usage, ok := response["usage"].(map[string]any); ok {
		started["usage"] = map[string]any{"input_tokens": usage["input_tokens"], "output_tokens": 0}
	}
	emit("message_start", map[string]any{"message": started})
	emit("ping", map[string]any{})
	content, _ := response["content"].([]any)
	for index, raw := range content {
		block, _ := raw.(map[string]any)
		if block == nil {
			continue
		}
		blockType, _ := block["type"].(string)
		startBlock := CloneJSONMap(block)
		switch blockType {
		case "text":
			startBlock["text"] = ""
		case "thinking":
			startBlock["thinking"] = ""
		case "tool_use", "server_tool_use":
			startBlock["input"] = map[string]any{}
		}
		emit("content_block_start", map[string]any{"index": index, "content_block": startBlock})
		switch blockType {
		case "text":
			if text, _ := block["text"].(string); text != "" {
				emit("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": "text_delta", "text": text}})
			}
		case "thinking":
			if thinking, _ := block["thinking"].(string); thinking != "" {
				emit("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": "thinking_delta", "thinking": thinking}})
			}
		case "tool_use", "server_tool_use":
			input, _ := json.Marshal(block["input"])
			emit("content_block_delta", map[string]any{"index": index, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)}})
		}
		emit("content_block_stop", map[string]any{"index": index})
	}
	usage, _ := response["usage"].(map[string]any)
	terminalUsage := map[string]any{"output_tokens": usage["output_tokens"]}
	if serverToolUse, ok := usage["server_tool_use"].(map[string]any); ok {
		terminalUsage["server_tool_use"] = serverToolUse
	}
	emit("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": response["stop_reason"], "stop_sequence": response["stop_sequence"]},
		"usage": terminalUsage,
	})
	emit("message_stop", map[string]any{})
}
