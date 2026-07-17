package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func DecodeBridgeObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func BridgeArray(value any) []any {
	items, _ := value.([]any)
	return items
}

func BridgeString(value any) string {
	text, _ := value.(string)
	return text
}

func AppendPairwiseWarning(warnings []BridgeWarning, code, path, message string) []BridgeWarning {
	return AppendBridgeWarning(warnings, BridgeWarning{Code: code, Path: path, Message: message})
}

func AnthropicSystemToResponsesInstructions(system any) (string, []BridgeWarning) {
	if system == nil {
		return "", nil
	}
	if text, ok := system.(string); ok {
		return text, nil
	}
	blocks, ok := system.([]any)
	if !ok {
		return ExtractClaudeSystemText(system), []BridgeWarning{{
			Code: "invalid_system_shape", Path: "system",
			Message: "Non-string Anthropic system value was serialized into Responses instructions",
		}}
	}
	var parts []string
	var warnings []BridgeWarning
	for index, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok || BridgeString(block["type"]) != "text" {
			warnings = AppendPairwiseWarning(warnings, "unsupported_system_block", fmt.Sprintf("system[%d]", index), "non-text Anthropic system block was skipped")
			continue
		}
		parts = append(parts, BridgeString(block["text"]))
		if _, exists := block["cache_control"]; exists {
			warnings = AppendPairwiseWarning(warnings, "system_cache_control_ignored", fmt.Sprintf("system[%d].cache_control", index), "Anthropic system cache breakpoint is unavailable in Responses instructions")
		}
	}
	return strings.Join(parts, "\n"), warnings
}

func AnthropicMediaToResponsesPart(block map[string]any) (map[string]any, bool) {
	chatPart, ok := ClaudeMediaBlockToChat(block)
	if !ok {
		return nil, false
	}
	switch chatPart["type"] {
	case "image_url":
		imageURL, _ := chatPart["image_url"].(map[string]any)
		if imageURL == nil {
			if typed, ok := chatPart["image_url"].(map[string]string); ok {
				imageURL = map[string]any{"url": typed["url"]}
			}
		}
		url, _ := imageURL["url"].(string)
		if url != "" {
			return map[string]any{"type": "input_image", "image_url": url}, true
		}
	case "file":
		file, _ := chatPart["file"].(map[string]any)
		if file != nil {
			result := map[string]any{"type": "input_file"}
			for _, key := range []string{"filename", "file_data", "file_url"} {
				if value, exists := file[key]; exists {
					result[key] = value
				}
			}
			return result, true
		}
	case "input_file":
		result := CloneAnyMap(chatPart)
		return result, true
	}
	return nil, false
}

func AnthropicContentToResponsesItems(role string, content any, path string) ([]any, []BridgeWarning) {
	if text, ok := content.(string); ok {
		partType := "input_text"
		if role == "assistant" {
			partType = "output_text"
		}
		return []any{map[string]any{"type": "message", "role": role, "content": []any{map[string]any{"type": partType, "text": text}}}}, nil
	}
	blocks := BridgeArray(content)
	var items []any
	var warnings []BridgeWarning
	pendingParts := []any{}
	flushMessage := func() {
		if len(pendingParts) == 0 {
			return
		}
		items = append(items, map[string]any{"type": "message", "role": role, "content": pendingParts})
		pendingParts = nil
	}
	for index, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			warnings = AppendPairwiseWarning(warnings, "unsupported_content_block", fmt.Sprintf("%s[%d]", path, index), "non-object Anthropic content block was skipped")
			continue
		}
		blockType := BridgeString(block["type"])
		switch blockType {
		case "text":
			partType := "input_text"
			if role == "assistant" {
				partType = "output_text"
			}
			pendingParts = append(pendingParts, map[string]any{"type": partType, "text": BridgeString(block["text"])})
		case "image", "document":
			if part, ok := AnthropicMediaToResponsesPart(block); ok {
				pendingParts = append(pendingParts, part)
			} else {
				warnings = AppendPairwiseWarning(warnings, "unsupported_media_source", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Anthropic %s source could not be represented by Responses", blockType))
			}
		case "thinking":
			flushMessage()
			item := map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": BridgeString(block["thinking"])}}}
			if signature := BridgeString(block["signature"]); signature != "" {
				item["anthropic_signature"] = signature
			}
			items = append(items, item)
		case "redacted_thinking":
			flushMessage()
			item := map[string]any{"type": "reasoning", "summary": []any{}}
			if data := BridgeString(block["data"]); data != "" {
				item["encrypted_content"] = data
			}
			items = append(items, item)
		case "tool_use":
			flushMessage()
			arguments, _ := json.Marshal(block["input"])
			if bytes.Equal(arguments, []byte("null")) {
				arguments = []byte("{}")
			}
			items = append(items, map[string]any{
				"type": "function_call", "call_id": BridgeString(block["id"]),
				"name": BridgeString(block["name"]), "arguments": string(arguments),
			})
		case "server_tool_use":
			flushMessage()
			name := BridgeString(block["name"])
			if name == "web_search" {
				input, _ := block["input"].(map[string]any)
				action := CloneAnyMap(input)
				if BridgeString(action["type"]) == "" {
					action["type"] = "search"
				}
				items = append(items, map[string]any{
					"type": "web_search_call", "id": BridgeString(block["id"]),
					"status": "completed", "action": action,
				})
			} else {
				arguments, _ := json.Marshal(block["input"])
				if bytes.Equal(arguments, []byte("null")) {
					arguments = []byte("{}")
				}
				items = append(items, map[string]any{
					"type": "function_call", "call_id": BridgeString(block["id"]),
					"name": name, "arguments": string(arguments),
				})
				warnings = AppendPairwiseWarning(warnings, "server_tool_use_downgraded", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Anthropic server tool use %q was represented as a function call", name))
			}
		case "web_search_tool_result":
			flushMessage()
			encoded, _ := json.Marshal(StripAnthropicEncryptedSearchContent(block["content"]))
			partType := "input_text"
			// 在 Anthropic 中，服务端工具结果属于助手响应块。
			// 同时防御性地支持 user 角色，以兼容不规范的客户端。
			if role == "assistant" {
				partType = "output_text"
			}
			pendingParts = append(pendingParts, map[string]any{
				"type": partType,
				"text": "[web_search_results] " + string(encoded),
			})
			warnings = AppendPairwiseWarning(warnings, "web_search_results_downgraded", fmt.Sprintf("%s[%d]", path, index), "Anthropic web-search result metadata was preserved as Responses text; provider-encrypted content was omitted")
		case "tool_result":
			flushMessage()
			output := block["content"]
			if output == nil {
				output = ""
			} else if _, isArray := output.([]any); isArray {
				converted, outputWarnings := AnthropicToolResultToResponsesOutput(output, fmt.Sprintf("%s[%d].content", path, index))
				output = converted
				warnings = AppendBridgeWarnings(warnings, outputWarnings)
			}
			if isError, _ := block["is_error"].(bool); isError {
				if text, ok := output.(string); ok {
					output = "[tool_error] " + text
				}
			}
			items = append(items, map[string]any{"type": "function_call_output", "call_id": BridgeString(block["tool_use_id"]), "output": output})
		default:
			warnings = AppendPairwiseWarning(warnings, "unsupported_content_block", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Anthropic content block type %q was skipped", blockType))
		}
	}
	flushMessage()
	return items, warnings
}

func AnthropicToolResultToResponsesOutput(content any, path string) (any, []BridgeWarning) {
	var output []any
	var warnings []BridgeWarning
	for index, raw := range BridgeArray(content) {
		block, _ := raw.(map[string]any)
		if block == nil {
			continue
		}
		switch blockType := BridgeString(block["type"]); blockType {
		case "text":
			output = append(output, map[string]any{"type": "input_text", "text": BridgeString(block["text"])})
		case "image", "document":
			if part, ok := AnthropicMediaToResponsesPart(block); ok {
				output = append(output, part)
			} else {
				warnings = AppendPairwiseWarning(warnings, "unsupported_media_source", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Anthropic tool-result %s source could not be represented by Responses", blockType))
			}
		case "tool_reference":
			toolName := BridgeString(block["tool_name"])
			output = append(output, map[string]any{"type": "input_text", "text": "[tool_reference] " + toolName})
			warnings = AppendPairwiseWarning(warnings, "tool_reference_downgraded", fmt.Sprintf("%s[%d]", path, index), "Anthropic tool reference was preserved as Responses tool-output text")
		case "search_result":
			encoded, _ := json.Marshal(block)
			output = append(output, map[string]any{"type": "input_text", "text": "[search_result] " + string(encoded)})
			warnings = AppendPairwiseWarning(warnings, "search_result_downgraded", fmt.Sprintf("%s[%d]", path, index), "Anthropic search result was preserved as Responses tool-output text")
		default:
			warnings = AppendPairwiseWarning(warnings, "unsupported_tool_result_block", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Anthropic tool-result block type %q was skipped", blockType))
		}
	}
	if len(output) == 0 {
		return "", warnings
	}
	return output, warnings
}

func StripAnthropicEncryptedSearchContent(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, nested := range typed {
			if key == "encrypted_content" {
				continue
			}
			result[key] = StripAnthropicEncryptedSearchContent(nested)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, nested := range typed {
			result[index] = StripAnthropicEncryptedSearchContent(nested)
		}
		return result
	default:
		return value
	}
}

func ConvertAnthropicRequestToResponsesDirect(body []byte, model string, forwardReasoning bool, configuredMaps ...map[string]string) ([]byte, []BridgeWarning, error) {
	request, err := DecodeBridgeObject(body)
	if err != nil {
		return nil, nil, err
	}
	result := map[string]any{"model": model}
	for _, key := range []string{"stream", "temperature", "top_p", "metadata"} {
		if value, exists := request[key]; exists {
			result[key] = value
		}
	}
	if value, exists := request["max_tokens"]; exists {
		result["max_output_tokens"] = value
	}
	system, systemWarnings := AnthropicSystemToResponsesInstructions(request["system"])
	warnings := AppendBridgeWarnings(nil, systemWarnings)
	if value, exists := request["service_tier"]; exists && value != nil {
		if mapped, recognized := OpenAIServiceTierFromAnthropic(value); recognized {
			result["service_tier"] = mapped
		} else {
			warnings = AppendPairwiseWarning(warnings, "request_field_ignored", "service_tier", fmt.Sprintf("Anthropic service_tier %q has no Responses equivalent and was ignored", BridgeString(value)))
		}
	}
	if system != "" {
		result["instructions"] = system
	}
	var input []any
	for index, raw := range BridgeArray(request["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			return nil, warnings, fmt.Errorf("messages[%d] must be an object", index)
		}
		role := BridgeString(message["role"])
		items, itemWarnings := AnthropicContentToResponsesItems(role, message["content"], fmt.Sprintf("messages[%d].content", index))
		input = append(input, items...)
		warnings = AppendBridgeWarnings(warnings, itemWarnings)
	}
	result["input"] = input
	hasAnthropicWebSearch := false
	if rawTools := BridgeArray(request["tools"]); len(rawTools) > 0 {
		converted := []any{}
		for index, raw := range rawTools {
			tool, _ := raw.(map[string]any)
			if tool == nil || BridgeString(tool["name"]) == "" {
				return nil, warnings, fmt.Errorf("tools[%d].name is required", index)
			}
			if toolType := BridgeString(tool["type"]); toolType != "" {
				if strings.HasPrefix(strings.ToLower(toolType), "web_search_") {
					webSearch := map[string]any{"type": "web_search"}
					filters := map[string]any{}
					for _, key := range []string{"allowed_domains", "blocked_domains"} {
						if value, exists := tool[key]; exists {
							filters[key] = value
						}
					}
					if len(filters) > 0 {
						webSearch["filters"] = filters
					}
					if value, exists := tool["user_location"]; exists {
						webSearch["user_location"] = value
					}
					converted = append(converted, webSearch)
					hasAnthropicWebSearch = true
					for _, key := range []string{"max_uses", "allowed_callers", "response_inclusion", "defer_loading", "strict"} {
						if _, exists := tool[key]; exists {
							warnings = AppendPairwiseWarning(warnings, "web_search_option_ignored", fmt.Sprintf("tools[%d].%s", index, key), fmt.Sprintf("Anthropic web-search option %q has no Responses equivalent", key))
						}
					}
					if _, exists := tool["cache_control"]; exists {
						warnings = AppendPairwiseWarning(warnings, "tool_cache_control_ignored", fmt.Sprintf("tools[%d].cache_control", index), "Anthropic tool cache breakpoint is unavailable on Responses")
					}
					continue
				}
				warnings = AppendPairwiseWarning(warnings, "unsupported_anthropic_server_tool", fmt.Sprintf("tools[%d]", index), fmt.Sprintf("Anthropic server tool type %q is unavailable on Responses upstream and was skipped", toolType))
				continue
			}
			parameters := tool["input_schema"]
			if parameters == nil {
				parameters = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			functionTool := map[string]any{"type": "function", "name": tool["name"], "description": tool["description"], "parameters": parameters}
			if strict, exists := tool["strict"]; exists {
				functionTool["strict"] = strict
			}
			converted = append(converted, functionTool)
			for _, key := range []string{"allowed_callers", "defer_loading", "input_examples"} {
				if _, exists := tool[key]; exists {
					warnings = AppendPairwiseWarning(warnings, "anthropic_tool_option_ignored", fmt.Sprintf("tools[%d].%s", index, key), fmt.Sprintf("Anthropic tool option %q has no portable Responses equivalent", key))
				}
			}
			if _, exists := tool["cache_control"]; exists {
				warnings = AppendPairwiseWarning(warnings, "tool_cache_control_ignored", fmt.Sprintf("tools[%d].cache_control", index), "Anthropic tool cache breakpoint is unavailable on Responses")
			}
		}
		if len(converted) > 0 {
			result["tools"] = converted
		}
		if hasAnthropicWebSearch {
			result["include"] = []any{"web_search_call.action.sources"}
		}
	}
	if choice, parallel := ClaudeToolChoiceToOpenAI(request["tool_choice"]); choice != nil {
		if choiceMap, ok := choice.(map[string]any); ok {
			if function, ok := choiceMap["function"].(map[string]any); ok {
				choice = map[string]any{"type": "function", "name": function["name"]}
			}
		}
		if hasAnthropicWebSearch {
			if choiceMap, ok := choice.(map[string]any); ok && BridgeString(choiceMap["type"]) == "function" && BridgeString(choiceMap["name"]) == "web_search" {
				choice = map[string]any{"type": "web_search"}
			}
		}
		result["tool_choice"] = choice
		if parallel != nil {
			result["parallel_tool_calls"] = *parallel
		}
	}
	if effort := ReasoningEffortFromAnthropic(request["thinking"], request["output_config"]); forwardReasoning && effort != "" && effort != "none" {
		mappedEffort := MapConfiguredReasoningEffort(effort, configuredMaps...)
		reasoning := map[string]any{"effort": mappedEffort}
		if budget, hasBudget := AnthropicThinkingBudget(request["thinking"]); hasBudget {
			warnings = AppendPairwiseWarning(warnings, "thinking_budget_approximated", "thinking.budget_tokens", fmt.Sprintf("Anthropic thinking budget %.0f was approximated as Responses reasoning effort %q", budget, mappedEffort))
		} else if thinking, ok := request["thinking"].(map[string]any); ok && strings.EqualFold(BridgeString(thinking["type"]), "enabled") {
			warnings = AppendPairwiseWarning(warnings, "thinking_budget_approximated", "thinking", fmt.Sprintf("Anthropic enabled thinking without an explicit budget was approximated as Responses reasoning effort %q", mappedEffort))
		} else if thinking, ok := request["thinking"].(map[string]any); ok && strings.EqualFold(BridgeString(thinking["type"]), "adaptive") {
			outputConfig, _ := request["output_config"].(map[string]any)
			if strings.TrimSpace(BridgeString(outputConfig["effort"])) == "" {
				warnings = AppendPairwiseWarning(warnings, "reasoning_effort_approximated", "thinking", fmt.Sprintf("Anthropic adaptive thinking without an explicit effort was approximated as Responses reasoning effort %q", mappedEffort))
			}
		}
		result["reasoning"] = reasoning
	}
	if stops := BridgeArray(request["stop_sequences"]); len(stops) > 0 {
		warnings = AppendPairwiseWarning(warnings, "stop_sequences_ignored", "stop_sequences", "Anthropic stop sequences are unavailable on the Responses API")
	}
	encoded, err := json.Marshal(result)
	return encoded, warnings, err
}

func DataURLToAnthropicSource(url string) map[string]any {
	if strings.HasPrefix(url, "data:") {
		if comma := strings.IndexByte(url, ','); comma > 5 {
			header := url[5:comma]
			mediaType := strings.TrimSuffix(header, ";base64")
			return map[string]any{"type": "base64", "media_type": mediaType, "data": url[comma+1:]}
		}
	}
	return map[string]any{"type": "url", "url": url}
}

func ResponsesContentToAnthropicBlocks(content any, path string) ([]any, []BridgeWarning) {
	if text, ok := content.(string); ok {
		return []any{map[string]any{"type": "text", "text": text}}, nil
	}
	var blocks []any
	var warnings []BridgeWarning
	for index, raw := range BridgeArray(content) {
		part, ok := raw.(map[string]any)
		if !ok {
			warnings = AppendPairwiseWarning(warnings, "unsupported_content_part", fmt.Sprintf("%s[%d]", path, index), "non-object Responses content part was skipped")
			continue
		}
		switch partType := BridgeString(part["type"]); partType {
		case "input_text", "output_text", "summary_text", "text":
			blocks = append(blocks, map[string]any{"type": "text", "text": BridgeString(part["text"])})
		case "input_image", "image_url":
			image := ResponsesImageURLFromPart(part)
			if image != nil {
				blocks = append(blocks, map[string]any{"type": "image", "source": DataURLToAnthropicSource(BridgeString(image["url"]))})
			} else {
				warnings = AppendPairwiseWarning(warnings, "unsupported_media_source", fmt.Sprintf("%s[%d]", path, index), "Responses image without an image_url was skipped")
			}
		case "input_file":
			url := BridgeString(part["file_data"])
			if url == "" {
				url = BridgeString(part["file_url"])
			}
			if url != "" {
				block := map[string]any{"type": "document", "source": DataURLToAnthropicSource(url)}
				if filename := BridgeString(part["filename"]); filename != "" {
					block["title"] = filename
				}
				blocks = append(blocks, block)
			} else {
				warnings = AppendPairwiseWarning(warnings, "unsupported_file_reference", fmt.Sprintf("%s[%d]", path, index), "Responses file_id cannot be dereferenced by the Anthropic bridge")
			}
		default:
			warnings = AppendPairwiseWarning(warnings, "unsupported_content_part", fmt.Sprintf("%s[%d]", path, index), fmt.Sprintf("Responses content part type %q was skipped", partType))
		}
	}
	return blocks, warnings
}

func AppendAnthropicMessage(messages []any, role string, blocks []any) []any {
	if len(blocks) == 0 {
		return messages
	}
	if len(messages) > 0 {
		if previous, ok := messages[len(messages)-1].(map[string]any); ok && BridgeString(previous["role"]) == role {
			previous["content"] = append(BridgeArray(previous["content"]), blocks...)
			return messages
		}
	}
	return append(messages, map[string]any{"role": role, "content": blocks})
}

func ResponseReasoningToAnthropicBlocks(item map[string]any, path string) ([]any, []BridgeWarning) {
	var blocks []any
	var warnings []BridgeWarning
	summary := ExtractTextFromContentParts(item["summary"])
	if signature := BridgeString(item["anthropic_signature"]); signature != "" {
		blocks = append(blocks, map[string]any{"type": "thinking", "thinking": summary, "signature": signature})
	}
	if encrypted := BridgeString(item["encrypted_content"]); encrypted != "" {
		blocks = append(blocks, map[string]any{"type": "redacted_thinking", "data": encrypted})
	}
	if len(blocks) == 0 && summary != "" {
		// Anthropic 重放 thinking 块时要求携带其专用签名。
		// OpenAI 的可见推理摘要没有此签名，因此将其保留为普通可见文本，
		// 并让该降级行为可被观测。
		blocks = append(blocks, map[string]any{"type": "text", "text": summary})
		warnings = AppendPairwiseWarning(warnings, "reasoning_summary_downgraded", path, "Responses reasoning summary had no Anthropic signature and was represented as visible text")
	}
	return blocks, warnings
}

func ConvertResponsesRequestToAnthropicDirect(body []byte, model string, forwardReasoning bool, configuredMaps ...map[string]string) ([]byte, map[string]ResponseToolNameMapping, []BridgeWarning, error) {
	request, err := DecodeBridgeObject(body)
	if err != nil {
		return nil, nil, nil, err
	}
	result := map[string]any{"model": model, "max_tokens": 4096}
	for _, key := range []string{"stream", "temperature", "top_p", "metadata"} {
		if value, exists := request[key]; exists {
			result[key] = value
		}
	}
	if value, exists := request["max_output_tokens"]; exists {
		result["max_tokens"] = value
	}
	var warnings []BridgeWarning
	if value, exists := request["service_tier"]; exists && value != nil {
		if mapped, recognized, approximated := AnthropicServiceTierFromOpenAI(value); recognized {
			result["service_tier"] = mapped
			if approximated {
				warnings = AppendPairwiseWarning(warnings, "service_tier_approximated", "service_tier", fmt.Sprintf("Responses service_tier %q was mapped to Anthropic auto", BridgeString(value)))
			}
		} else {
			warnings = AppendPairwiseWarning(warnings, "request_field_ignored", "service_tier", fmt.Sprintf("Responses service_tier %q has no Anthropic equivalent and was ignored", BridgeString(value)))
		}
	}
	var responseTools []ResponsesTool
	if rawTools, exists := request["tools"]; exists {
		encoded, _ := json.Marshal(rawTools)
		if err := json.Unmarshal(encoded, &responseTools); err != nil {
			return nil, nil, warnings, fmt.Errorf("invalid tools: %w", err)
		}
	}
	responseTools = append(responseTools, ResponsesLoadedToolDefinitions(request["input"])...)
	convertedTools, mappings, toolWarnings := ConvertResponsesToolsWithMappingsDetailed(responseTools)
	toolWarnings = FilterNativeHostedWebSearchWarnings(toolWarnings, responseTools)
	warnings = AppendBridgeWarnings(warnings, toolWarnings)
	var nativeWebSearchTools []any
	for _, tool := range responseTools {
		if tool.Type != "web_search" && tool.Type != "web_search_preview" {
			continue
		}
		native := map[string]any{"type": "web_search_20250305", "name": "web_search"}
		if filters, ok := tool.Filters.(map[string]any); ok {
			if allowed, exists := filters["allowed_domains"]; exists {
				native["allowed_domains"] = allowed
			}
			if blocked, exists := filters["blocked_domains"]; exists {
				native["blocked_domains"] = blocked
			}
		}
		if tool.UserLocation != nil {
			native["user_location"] = tool.UserLocation
		}
		nativeWebSearchTools = append(nativeWebSearchTools, native)
	}
	systemParts := []string{}
	if instructions := BridgeString(request["instructions"]); instructions != "" {
		systemParts = append(systemParts, instructions)
	}
	var messages []any
	input := request["input"]
	if text, ok := input.(string); ok {
		messages = AppendAnthropicMessage(messages, "user", []any{map[string]any{"type": "text", "text": text}})
	} else {
		for index, raw := range BridgeArray(input) {
			item, ok := raw.(map[string]any)
			if !ok {
				warnings = AppendPairwiseWarning(warnings, "unsupported_input_item", fmt.Sprintf("input[%d]", index), "non-object Responses input item was skipped")
				continue
			}
			switch itemType := BridgeString(item["type"]); itemType {
			case "additional_tools":
				warnings = AppendPairwiseWarning(warnings, "additional_tools_hoisted", fmt.Sprintf("input[%d]", index), "additional_tools were exposed at request scope because Anthropic cannot preserve their input position")
			case "message", "":
				role := BridgeString(item["role"])
				blocks, partWarnings := ResponsesContentToAnthropicBlocks(item["content"], fmt.Sprintf("input[%d].content", index))
				warnings = AppendBridgeWarnings(warnings, partWarnings)
				if role == "developer" || role == "system" {
					if text := ExtractTextFromContentParts(item["content"]); text != "" {
						systemParts = append(systemParts, text)
					}
					for _, rawPart := range BridgeArray(item["content"]) {
						part, _ := rawPart.(map[string]any)
						partType := BridgeString(part["type"])
						if partType != "input_text" && partType != "output_text" && partType != "text" {
							warnings = AppendPairwiseWarning(warnings, "system_content_flattened", fmt.Sprintf("input[%d]", index), "non-text system content was omitted from Anthropic system prompt")
							break
						}
					}
					continue
				}
				if role != "assistant" {
					role = "user"
				}
				messages = AppendAnthropicMessage(messages, role, blocks)
			case "reasoning":
				blocks, reasoningWarnings := ResponseReasoningToAnthropicBlocks(item, fmt.Sprintf("input[%d]", index))
				warnings = AppendBridgeWarnings(warnings, reasoningWarnings)
				messages = AppendAnthropicMessage(messages, "assistant", blocks)
			case "function_call", "custom_tool_call", "tool_search_call":
				var inputObject any = map[string]any{}
				callID := BridgeString(item["call_id"])
				arguments := BridgeString(item["arguments"])
				name := BridgeString(item["name"])
				kind := "function"
				if itemType == "custom_tool_call" {
					kind = "custom"
					argumentsBytes, _ := json.Marshal(map[string]any{"input": item["input"]})
					arguments = string(argumentsBytes)
				} else if itemType == "tool_search_call" {
					kind = "tool_search"
					name = "tool_search"
					if callID == "" && strings.EqualFold(BridgeString(item["execution"]), "server") {
						warnings = AppendPairwiseWarning(warnings, "hosted_tool_search_history_omitted", fmt.Sprintf("input[%d]", index), "server-executed tool_search_call has no call_id and was omitted from Anthropic history")
						continue
					}
					searchArguments := item["arguments"]
					if searchArguments == nil {
						searchArguments = map[string]any{"query": item["query"]}
					}
					argumentsBytes, _ := json.Marshal(searchArguments)
					arguments = string(argumentsBytes)
				}
				if callID == "" {
					warnings = AppendPairwiseWarning(warnings, "invalid_tool_call", fmt.Sprintf("input[%d]", index), fmt.Sprintf("Responses %s without call_id was skipped", itemType))
					continue
				}
				name = ResponseUpstreamToolName(kind, "", name, mappings)
				if arguments != "" {
					if err := json.Unmarshal([]byte(arguments), &inputObject); err != nil {
						return nil, nil, warnings, fmt.Errorf("input[%d].arguments is invalid JSON: %w", index, err)
					}
				}
				messages = AppendAnthropicMessage(messages, "assistant", []any{map[string]any{"type": "tool_use", "id": callID, "name": name, "input": inputObject}})
			case "function_call_output", "custom_tool_call_output", "tool_search_output", "tool_search_call_output":
				callID := BridgeString(item["call_id"])
				if itemType == "tool_search_output" && callID == "" && strings.EqualFold(BridgeString(item["execution"]), "server") {
					warnings = AppendPairwiseWarning(warnings, "hosted_tool_search_history_omitted", fmt.Sprintf("input[%d]", index), "server-executed tool_search_output has no call_id; its loaded tools were retained but its internal history was omitted")
					continue
				}
				if callID == "" {
					warnings = AppendPairwiseWarning(warnings, "invalid_tool_output", fmt.Sprintf("input[%d]", index), fmt.Sprintf("Responses %s without call_id was skipped", itemType))
					continue
				}
				rawOutput := item["output"]
				if itemType == "tool_search_output" {
					rawOutput = map[string]any{"execution": item["execution"], "status": item["status"], "tools": item["tools"]}
				}
				outputBlocks, outputWarnings := ResponsesContentToAnthropicBlocks(rawOutput, fmt.Sprintf("input[%d].output", index))
				warnings = AppendBridgeWarnings(warnings, outputWarnings)
				var output any = rawOutput
				if len(outputBlocks) > 0 {
					output = outputBlocks
				}
				messages = AppendAnthropicMessage(messages, "user", []any{map[string]any{"type": "tool_result", "tool_use_id": callID, "content": output}})
			default:
				warnings = AppendPairwiseWarning(warnings, "unsupported_input_item", fmt.Sprintf("input[%d]", index), fmt.Sprintf("Responses input item type %q was skipped", itemType))
			}
		}
	}
	if len(messages) == 0 {
		messages = []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": ""}}}}
	}
	result["messages"] = messages
	if len(systemParts) > 0 {
		result["system"] = strings.Join(systemParts, "\n\n")
	}
	if len(convertedTools) > 0 || len(nativeWebSearchTools) > 0 {
		tools := make([]any, 0, len(convertedTools)+len(nativeWebSearchTools))
		for _, tool := range convertedTools {
			tools = append(tools, map[string]any{"name": tool.Function.Name, "description": tool.Function.Description, "input_schema": tool.Function.Parameters})
		}
		tools = append(tools, nativeWebSearchTools...)
		result["tools"] = tools
	}
	choiceMap, _ := request["tool_choice"].(map[string]any)
	nativeWebSearchChoice := choiceMap != nil && (BridgeString(choiceMap["type"]) == "web_search" || BridgeString(choiceMap["type"]) == "web_search_preview") && len(nativeWebSearchTools) > 0
	var choice any
	if nativeWebSearchChoice {
		result["tool_choice"] = map[string]any{"type": "tool", "name": "web_search"}
	} else {
		var choiceWarnings []BridgeWarning
		choice, choiceWarnings = ConvertResponsesToolChoiceDetailed(request["tool_choice"], mappings, len(convertedTools)+len(nativeWebSearchTools) > 0)
		warnings = AppendBridgeWarnings(warnings, choiceWarnings)
	}
	if choice != nil && len(convertedTools)+len(nativeWebSearchTools) > 0 {
		switch value := choice.(type) {
		case string:
			switch value {
			case "required":
				result["tool_choice"] = map[string]any{"type": "any"}
			case "none", "auto":
				result["tool_choice"] = map[string]any{"type": value}
			}
		case map[string]any:
			if function, ok := value["function"].(map[string]any); ok {
				result["tool_choice"] = map[string]any{"type": "tool", "name": function["name"]}
			}
		}
	}
	if parallel, ok := request["parallel_tool_calls"].(bool); ok && !parallel {
		choiceMap, _ := result["tool_choice"].(map[string]any)
		if choiceMap == nil {
			choiceMap = map[string]any{"type": "auto"}
		}
		choiceMap["disable_parallel_tool_use"] = true
		result["tool_choice"] = choiceMap
	}
	if reasoning, ok := request["reasoning"].(map[string]any); forwardReasoning && ok {
		if effort := BridgeString(reasoning["effort"]); effort != "" && effort != "none" {
			mappedEffort := MapConfiguredReasoningEffort(effort, configuredMaps...)
			if applied, approximated := ApplyAdaptiveAnthropicReasoning(result, model, mappedEffort); applied {
				if approximated {
					warnings = AppendPairwiseWarning(warnings, "reasoning_effort_approximated", "reasoning.effort", fmt.Sprintf("Responses reasoning effort %q was approximated as Anthropic effort %q", mappedEffort, result["output_config"].(map[string]any)["effort"]))
				}
			} else {
				result["thinking"] = ReasoningEffortToAnthropicThinking(mappedEffort)
				warnings = AppendPairwiseWarning(warnings, "reasoning_effort_approximated", "reasoning.effort", fmt.Sprintf("Responses reasoning effort %q was approximated as an Anthropic manual thinking budget", mappedEffort))
			}
		}
	}
	encoded, err := json.Marshal(result)
	return encoded, mappings, warnings, err
}

func AnthropicResponseOutputItems(content []any, mappings map[string]ResponseToolNameMapping) ([]any, []BridgeWarning) {
	var output []any
	var warnings []BridgeWarning
	messageParts := []any{}
	flushMessage := func() {
		if len(messageParts) == 0 {
			return
		}
		output = append(output, map[string]any{"id": "msg_" + RandomString(16), "type": "message", "status": "completed", "role": "assistant", "content": messageParts})
		messageParts = nil
	}
	for index, raw := range content {
		block, _ := raw.(map[string]any)
		if block == nil {
			continue
		}
		switch blockType := BridgeString(block["type"]); blockType {
		case "text":
			messageParts = append(messageParts, map[string]any{"type": "output_text", "text": BridgeString(block["text"]), "annotations": []any{}, "logprobs": []any{}})
		case "thinking":
			flushMessage()
			item := map[string]any{"id": "rs_" + RandomString(16), "type": "reasoning", "status": "completed", "summary": []any{map[string]any{"type": "summary_text", "text": BridgeString(block["thinking"])}}}
			if signature := BridgeString(block["signature"]); signature != "" {
				item["anthropic_signature"] = signature
			}
			output = append(output, item)
		case "redacted_thinking":
			flushMessage()
			output = append(output, map[string]any{"id": "rs_" + RandomString(16), "type": "reasoning", "status": "completed", "summary": []any{}, "encrypted_content": block["data"]})
		case "tool_use", "server_tool_use":
			flushMessage()
			arguments, _ := json.Marshal(block["input"])
			callID := BridgeString(block["id"])
			name := BridgeString(block["name"])
			output = append(output, ResponseFunctionCallItem(ResponseToolCallItemID(callID, name, mappings), "completed", string(arguments), callID, name, mappings))
		default:
			warnings = AppendPairwiseWarning(warnings, "unsupported_response_block", fmt.Sprintf("content[%d]", index), fmt.Sprintf("Anthropic response block type %q was skipped", blockType))
		}
	}
	flushMessage()
	return output, warnings
}

func ConvertAnthropicResponseToResponsesDirect(body []byte, model string, request map[string]any, mappings map[string]ResponseToolNameMapping, warningGroups ...[]BridgeWarning) ([]byte, []BridgeWarning, error) {
	message, err := DecodeBridgeObject(body)
	if err != nil {
		return nil, nil, err
	}
	output, responseWarnings := AnthropicResponseOutputItems(BridgeArray(message["content"]), mappings)
	var warnings []BridgeWarning
	warnings = AppendBridgeWarnings(warnings, warningGroups...)
	warnings = AppendBridgeWarnings(warnings, responseWarnings)
	status := "completed"
	var incomplete any
	stopReason := BridgeString(message["stop_reason"])
	if stopReason == "max_tokens" || stopReason == "model_context_window_exceeded" {
		status = "incomplete"
		reason := "max_output_tokens"
		if stopReason == "model_context_window_exceeded" {
			reason = stopReason
		}
		incomplete = map[string]any{"reason": reason}
	}
	responseID := BridgeString(message["id"])
	if responseID == "" {
		responseID = "resp_" + RandomString(16)
	} else {
		responseID = "resp_" + strings.TrimPrefix(responseID, "msg_")
	}
	result := map[string]any{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(),
		"status": status, "background": false, "error": nil, "incomplete_details": incomplete,
		"model": model, "output": output, "usage": AnthropicUsageToResponsesUsage(func() map[string]any { usage, _ := message["usage"].(map[string]any); return usage }()),
		"tools": []any{}, "tool_choice": "auto", "parallel_tool_calls": true,
	}
	if tools, exists := request["tools"]; exists {
		result["tools"] = tools
	}
	for _, key := range []string{"tool_choice", "parallel_tool_calls"} {
		if value, exists := request[key]; exists {
			result[key] = value
		}
	}
	ApplyResponsesRequestEcho(result, ResponsesRequestEchoFields(request))
	ApplyBridgeWarnings(result, warnings)
	encoded, err := json.Marshal(result)
	return encoded, responseWarnings, err
}

func ResponsesUsageToAnthropicUsage(usage map[string]any) map[string]any {
	input, _ := GetFloat(usage, "input_tokens", "prompt_tokens")
	output, _ := GetFloat(usage, "output_tokens", "completion_tokens")
	result := map[string]any{"input_tokens": int64(input), "output_tokens": int64(output)}
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		if cached, ok := details["cached_tokens"]; ok {
			result["cache_read_input_tokens"] = cached
		}
	}
	return result
}

func ResponsesWebSearchToAnthropicBlocks(item map[string]any) []any {
	action, _ := item["action"].(map[string]any)
	input := CloneAnyMap(action)
	delete(input, "sources")
	delete(input, "type")
	callID := BridgeString(item["id"])
	if callID == "" {
		callID = "srvtoolu_" + RandomString(16)
	}
	blocks := []any{map[string]any{
		"type": "server_tool_use", "id": callID, "name": "web_search", "input": input,
	}}
	var results []any
	for _, raw := range BridgeArray(action["sources"]) {
		source, _ := raw.(map[string]any)
		url := BridgeString(source["url"])
		if url == "" {
			continue
		}
		title := BridgeString(source["title"])
		if title == "" {
			title = url
		}
		results = append(results, map[string]any{"type": "web_search_result", "url": url, "title": title})
	}
	blocks = append(blocks, map[string]any{
		"type": "web_search_tool_result", "tool_use_id": callID, "content": results,
	})
	return blocks
}

func ConvertResponsesResponseToAnthropicDirect(body []byte, model string, mappings map[string]ResponseToolNameMapping) ([]byte, []BridgeWarning, error) {
	response, err := DecodeBridgeObject(body)
	if err != nil {
		return nil, nil, err
	}
	var content []any
	var warnings []BridgeWarning
	hasToolUse := false
	webSearchRequests := 0
	for index, raw := range BridgeArray(response["output"]) {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		switch itemType := BridgeString(item["type"]); itemType {
		case "message":
			for partIndex, rawPart := range BridgeArray(item["content"]) {
				part, _ := rawPart.(map[string]any)
				if part == nil {
					continue
				}
				switch BridgeString(part["type"]) {
				case "output_text":
					content = append(content, map[string]any{"type": "text", "text": BridgeString(part["text"])})
				case "refusal":
					content = append(content, map[string]any{"type": "text", "text": BridgeString(part["refusal"])})
				default:
					warnings = AppendPairwiseWarning(warnings, "unsupported_response_part", fmt.Sprintf("output[%d].content[%d]", index, partIndex), fmt.Sprintf("Responses output content type %q was skipped", part["type"]))
				}
			}
		case "reasoning":
			blocks, reasoningWarnings := ResponseReasoningToAnthropicBlocks(item, fmt.Sprintf("output[%d]", index))
			content = append(content, blocks...)
			warnings = AppendBridgeWarnings(warnings, reasoningWarnings)
		case "function_call", "custom_tool_call":
			arguments := BridgeString(item["arguments"])
			var input any = map[string]any{}
			if arguments != "" {
				if err := json.Unmarshal([]byte(arguments), &input); err != nil {
					return nil, warnings, fmt.Errorf("output[%d].arguments is invalid JSON: %w", index, err)
				}
			}
			name := BridgeString(item["name"])
			if mapping, ok := LookupResponseToolNameMapping(name, mappings); ok {
				name = mapping.Name
			}
			content = append(content, map[string]any{"type": "tool_use", "id": item["call_id"], "name": name, "input": input})
			hasToolUse = true
		case "web_search_call":
			content = append(content, ResponsesWebSearchToAnthropicBlocks(item)...)
			webSearchRequests++
		default:
			warnings = AppendPairwiseWarning(warnings, "unsupported_response_item", fmt.Sprintf("output[%d]", index), fmt.Sprintf("Responses output item type %q was skipped", itemType))
		}
	}
	stopReason := "end_turn"
	if BridgeString(response["status"]) == "incomplete" {
		stopReason = ResponsesIncompleteToAnthropicStopReason(response)
	} else if hasToolUse {
		stopReason = "tool_use"
	}
	if len(content) == 0 {
		content = []any{map[string]any{"type": "text", "text": ""}}
	}
	messageID := BridgeString(response["id"])
	messageID = "msg_" + strings.TrimPrefix(messageID, "resp_")
	usage, _ := response["usage"].(map[string]any)
	anthropicUsage := ResponsesUsageToAnthropicUsage(usage)
	anthropicUsage = WithAnthropicWebSearchUsage(anthropicUsage, webSearchRequests)
	result := map[string]any{
		"id": messageID, "type": "message", "role": "assistant", "content": content,
		"model": model, "stop_reason": stopReason, "stop_sequence": nil,
		"usage": anthropicUsage,
	}
	encoded, err := json.Marshal(result)
	return encoded, warnings, err
}
