package convert

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// ======================== Thinking/Reasoning 判断 ========================

func NumberFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func ReasoningEffortFromThinking(value any) string {
	switch v := value.(type) {
	case map[string]any:
		t, _ := v["type"].(string)
		switch strings.ToLower(t) {
		case "disabled":
			return "none"
		case "adaptive":
			return "high"
		case "enabled":
			if budget, ok := NumberFromAny(v["budget_tokens"]); ok && budget > 0 {
				switch {
				case budget <= 4000:
					return "low"
				case budget <= 16000:
					return "medium"
				case budget <= 32000:
					return "high"
				default:
					return "xhigh"
				}
			}
			return "high"
		}
	case map[string]string:
		switch strings.ToLower(v["type"]) {
		case "disabled":
			return "none"
		case "adaptive":
			return "high"
		case "enabled":
			return "high"
		}
	case bool:
		if v {
			return "high"
		}
		return "none"
	}
	return ""
}

// ReasoningEffortFromAnthropic 同时考虑 Anthropic 的 thinking 与
// output_config.effort。adaptive thinking 的显式 effort 比默认值更精确。
func ReasoningEffortFromAnthropic(thinking, outputConfig any) string {
	typeOfThinking := ""
	if object, ok := thinking.(map[string]any); ok {
		typeOfThinking, _ = object["type"].(string)
	} else if object, ok := thinking.(map[string]string); ok {
		typeOfThinking = object["type"]
	}
	typeOfThinking = strings.ToLower(strings.TrimSpace(typeOfThinking))
	if typeOfThinking == "disabled" {
		return "none"
	}
	if typeOfThinking == "adaptive" || typeOfThinking == "" {
		if object, ok := outputConfig.(map[string]any); ok {
			if effort, _ := object["effort"].(string); strings.TrimSpace(effort) != "" {
				switch strings.ToLower(strings.TrimSpace(effort)) {
				case "low", "medium", "high", "xhigh", "max":
					return strings.ToLower(strings.TrimSpace(effort))
				}
			}
		}
		if typeOfThinking == "adaptive" {
			return "high"
		}
	}
	return ReasoningEffortFromThinking(thinking)
}

func AnthropicThinkingBudget(value any) (float64, bool) {
	object, ok := value.(map[string]any)
	if !ok || !strings.EqualFold(BridgeString(object["type"]), "enabled") {
		return 0, false
	}
	budget, ok := NumberFromAny(object["budget_tokens"])
	return budget, ok && budget > 0
}

func AnthropicSupportsAdaptiveThinking(model string) bool {
	normalized := strings.NewReplacer(".", "-", "_", "-").Replace(strings.ToLower(strings.TrimSpace(model)))
	for _, marker := range []string{
		"fable-5", "mythos-5", "mythos-preview",
		"opus-4-6", "opus-4-7", "opus-4-8",
		"sonnet-4-6", "sonnet-5",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// ApplyAdaptiveAnthropicReasoning uses Anthropic's current reasoning shape on
// models where manual budget_tokens is deprecated or rejected. It reports
// whether the mapping was applied and whether an effort value was approximated.
func ApplyAdaptiveAnthropicReasoning(target map[string]any, model, effort string) (bool, bool) {
	if target == nil || !AnthropicSupportsAdaptiveThinking(model) {
		return false, false
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "none" || effort == "disabled" {
		return false, false
	}
	approximated := false
	if effort == "minimal" {
		effort = "low"
		approximated = true
	}
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return false, false
	}
	target["thinking"] = map[string]any{"type": "adaptive"}
	outputConfig, _ := target["output_config"].(map[string]any)
	outputConfig = CloneAnyMap(outputConfig)
	outputConfig["effort"] = effort
	target["output_config"] = outputConfig
	return true, approximated
}

func EnsureReasoningEffort(req *OpenAIRequest, alias ModelAlias) {
	if req == nil || req.ReasoningEffort != "" {
		return
	}
	if effort := ReasoningEffortFromThinking(req.Thinking); effort != "" {
		if effort != "none" {
			req.ReasoningEffort = effort
		}
		return
	}
	if req.ExtraBody != nil {
		if effort := ReasoningEffortFromThinking(req.ExtraBody["thinking"]); effort != "" {
			if effort != "none" {
				req.ReasoningEffort = effort
			}
			return
		}
	}
}

func ShouldUseLegacyResponsesReasoningEffort(upstream *UpstreamConfig) bool {
	if upstream == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(upstream.ResponsesReasoningFormat))
	return v == "reasoning_effort" || v == "legacy" || v == "legacy_reasoning_effort"
}

func SetResponsesReasoningEffort(req map[string]any, effort string, upstream *UpstreamConfig) {
	if effort == "" || effort == "none" {
		return
	}
	if ShouldUseLegacyResponsesReasoningEffort(upstream) {
		req["reasoning_effort"] = effort
		return
	}
	req["reasoning"] = map[string]any{"effort": effort}
}

func MapConfiguredReasoningEffort(effort string, configuredMaps ...map[string]string) string {
	if effort == "" {
		return ""
	}
	var effortMap map[string]string
	if len(configuredMaps) > 0 {
		effortMap = configuredMaps[0]
	} else {
		effortMap = GetReasoningEffortMap()
	}
	if mapped, ok := effortMap[effort]; ok {
		return mapped
	}
	return effort
}

// ======================== 上游格式转换 ========================

// ReasoningEffortToAnthropicThinking 将兼容 OpenAI 的 reasoning_effort 映射为
// Anthropic thinking，并设置 Anthropic API 要求的默认 budget_tokens。
func ReasoningEffortToAnthropicThinking(effort string) map[string]any {
	switch strings.ToLower(effort) {
	case "none", "disabled":
		return nil
	case "minimal":
		return map[string]any{"type": "enabled", "budget_tokens": 1024}
	case "low":
		return map[string]any{"type": "enabled", "budget_tokens": 4000}
	case "medium":
		return map[string]any{"type": "enabled", "budget_tokens": 16000}
	case "high":
		return map[string]any{"type": "enabled", "budget_tokens": 32000}
	case "xhigh", "max":
		return map[string]any{"type": "enabled", "budget_tokens": 64000}
	case "adaptive":
		return map[string]any{"type": "adaptive"}
	case "":
		return nil
	default:
		return map[string]any{"type": "enabled", "budget_tokens": 16000}
	}
}

// BuildAnthropicThinking 从 OpenAI 风格请求体中选择 Anthropic thinking 配置：
// 优先使用显式 thinking，否则使用 reasoning_effort。
func BuildAnthropicThinking(req map[string]any) any {
	if thinking, ok := req["thinking"]; ok {
		if tm, ok := thinking.(map[string]any); ok {
			if t, _ := tm["type"].(string); strings.EqualFold(t, "enabled") {
				if _, has := tm["budget_tokens"]; !has {
					tm["budget_tokens"] = 16000
				}
			}
			return tm
		}
		if tm, ok := thinking.(map[string]string); ok {
			switch strings.ToLower(tm["type"]) {
			case "disabled":
				return nil
			case "enabled":
				return map[string]any{"type": "enabled", "budget_tokens": 16000}
			default:
				return map[string]any{"type": tm["type"]}
			}
		}
		return thinking
	}
	if re, ok := req["reasoning_effort"].(string); ok && re != "" && re != "none" {
		return ReasoningEffortToAnthropicThinking(re)
	}
	return nil
}

// OpenAIToAnthropicRequest 将 OpenAI Chat 请求转为 Anthropic Messages 格式
func OpenAIToAnthropicRequest(body []byte) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	model, _ := req["model"].(string)
	// Claude 客户端已使用原生 Anthropic 协议。同协议路径应保留原始请求，
	// 避免经由信息更少的 Chat 中间格式往返转换；仅覆盖路由和传输控制字段。
	if original, ok := req[internalAnthropicRequestKey].(map[string]any); ok {
		encoded, err := json.Marshal(original)
		if err == nil {
			var native map[string]any
			if json.Unmarshal(encoded, &native) == nil {
				native["model"] = model
				if stream, exists := req["stream"]; exists {
					native["stream"] = stream
				}
				if system, ok := native["system"].(string); ok {
					native["system"] = StripBillingHeaderText(system)
				}
				result, marshalErr := json.Marshal(native)
				if marshalErr == nil {
					return result
				}
			}
		}
	}
	msgs, _ := req["messages"].([]any)

	var systemTexts []string
	var anthropicMsgs []map[string]any
	handleContent := func(content any, role string) []map[string]any {
		var blocks []map[string]any
		switch c := content.(type) {
		case string:
			if c != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": c})
			}
		case []any:
			for _, item := range c {
				if p, ok := item.(map[string]any); ok {
					switch p["type"] {
					case "text":
						if t, ok := p["text"].(string); ok && t != "" {
							blocks = append(blocks, map[string]any{"type": "text", "text": t})
						}
					case "image_url":
						blocks = append(blocks, ConvertOpenAIImageToAnthropic(p))
					case "image_file":
						if imageFile, ok := p["image_file"].(map[string]any); ok {
							if fileID, _ := imageFile["file_id"].(string); fileID != "" {
								blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "file", "file_id": fileID}})
							}
						}
					case "file", "input_file":
						if document, ok := ConvertOpenAIFileToAnthropic(p); ok {
							blocks = append(blocks, document)
						}
					}
				}
			}
		}
		return blocks
	}

	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		content := msg["content"]

		if role == "system" || role == "developer" {
			if s, ok := content.(string); ok {
				systemTexts = append(systemTexts, s)
			} else if parts, ok := content.([]any); ok {
				for _, part := range parts {
					if block, ok := part.(map[string]any); ok {
						if text, ok := block["text"].(string); ok && text != "" {
							systemTexts = append(systemTexts, text)
						}
					}
				}
			}
			continue
		}

		if role == "assistant" {
			var blocks []map[string]any

			if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
				thinkingBlock := map[string]any{"type": "thinking", "thinking": rc}
				if signature, _ := msg["reasoning_signature"].(string); signature != "" {
					thinkingBlock["signature"] = signature
				}
				blocks = append(blocks, thinkingBlock)
			}
			if encrypted, _ := msg["reasoning_encrypted_content"].(string); encrypted != "" {
				blocks = append(blocks, map[string]any{"type": "redacted_thinking", "data": encrypted})
			}

			blocks = append(blocks, handleContent(content, role)...)

			if tcs, ok := msg["tool_calls"].([]any); ok && len(tcs) > 0 {
				for _, tc := range tcs {
					tcMap, _ := tc.(map[string]any)
					id, _ := tcMap["id"].(string)
					fn, _ := tcMap["function"].(map[string]any)
					name, _ := fn["name"].(string)
					var args any = map[string]any{}
					if rawArgs, ok := fn["arguments"]; ok && rawArgs != nil {
						switch v := rawArgs.(type) {
						case string:
							if v != "" {
								var parsed any
								if json.Unmarshal([]byte(v), &parsed) == nil {
									args = parsed
								}
							}
						default:
							b, _ := json.Marshal(v)
							var parsed any
							if json.Unmarshal(b, &parsed) == nil {
								args = parsed
							}
						}
					}
					blocks = append(blocks, map[string]any{
						"type": "tool_use", "id": id, "name": name, "input": args,
					})
				}
			}

			if len(blocks) == 0 {
				blocks = append(blocks, map[string]any{"type": "text", "text": ""})
			}
			anthropicMsgs = append(anthropicMsgs, map[string]any{"role": "assistant", "content": blocks})
			continue
		}

		if role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			var resultText string
			if s, ok := content.(string); ok {
				resultText = s
			} else {
				b, _ := json.Marshal(content)
				resultText = string(b)
			}
			anthropicMsgs = append(anthropicMsgs, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{"type": "tool_result", "tool_use_id": toolCallID, "content": resultText},
				},
			})
			continue
		}

		if role == "user" {
			blocks := handleContent(content, role)
			if len(blocks) == 0 {
				continue
			}
			anthropicMsgs = append(anthropicMsgs, map[string]any{"role": "user", "content": blocks})
		}
	}

	if len(anthropicMsgs) == 0 {
		return body
	}

	claudeReq := map[string]any{
		"model":      model,
		"messages":   anthropicMsgs,
		"max_tokens": 4096,
	}
	if len(systemTexts) > 0 {
		claudeReq["system"] = strings.Join(systemTexts, "\n")
	}
	if stream, _ := req["stream"].(bool); stream {
		claudeReq["stream"] = true
	}
	if temp, ok := req["temperature"]; ok && temp != nil {
		claudeReq["temperature"] = temp
	}
	if topP, ok := req["top_p"]; ok && topP != nil {
		claudeReq["top_p"] = topP
	}
	if mt, _ := req["max_tokens"].(float64); mt > 0 {
		claudeReq["max_tokens"] = int(mt)
	} else if mt, _ := req["max_completion_tokens"].(float64); mt > 0 {
		claudeReq["max_tokens"] = int(mt)
	}
	if stop, ok := req["stop"]; ok && stop != nil {
		switch value := stop.(type) {
		case string:
			if value != "" {
				claudeReq["stop_sequences"] = []string{value}
			}
		case []any:
			sequences := make([]string, 0, len(value))
			for _, item := range value {
				if sequence, ok := item.(string); ok && sequence != "" {
					sequences = append(sequences, sequence)
				}
			}
			if len(sequences) > 0 {
				claudeReq["stop_sequences"] = sequences
			}
		}
	}
	if stopSequences, ok := req["stop_sequences"].([]any); ok && len(stopSequences) > 0 {
		claudeReq["stop_sequences"] = stopSequences
	}
	if metadata, ok := req["metadata"]; ok && metadata != nil {
		claudeReq["metadata"] = metadata
	}
	if serviceTier, ok := req["service_tier"]; ok && serviceTier != nil {
		if mapped, recognized, _ := AnthropicServiceTierFromOpenAI(serviceTier); recognized {
			claudeReq["service_tier"] = mapped
		}
	}
	convertedToolCount := 0
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		convertedTools := ConvertOpenAIToolsToAnthropic(tools)
		if len(convertedTools) > 0 {
			claudeReq["tools"] = convertedTools
			convertedToolCount = len(convertedTools)
		}
	}
	if tc, ok := req["tool_choice"]; ok && convertedToolCount > 0 {
		switch v := tc.(type) {
		case string:
			// 将 OpenAI 的 "auto"、"none"、"required" 映射为 Anthropic 的 {type: ...}。
			switch v {
			case "auto":
				claudeReq["tool_choice"] = map[string]any{"type": "auto"}
			case "none":
				claudeReq["tool_choice"] = map[string]any{"type": "none"}
			case "required":
				claudeReq["tool_choice"] = map[string]any{"type": "any"}
			default:
				claudeReq["tool_choice"] = tc
			}
		case map[string]any:
			// OpenAI 格式：{"type": "function", "function": {"name": "xxx"}}
			// Anthropic 格式：{"type": "tool", "name": "xxx"}
			if fn, ok := v["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && name != "" {
					claudeReq["tool_choice"] = map[string]any{"type": "tool", "name": name}
				} else {
					claudeReq["tool_choice"] = map[string]any{"type": "auto"}
				}
			} else {
				claudeReq["tool_choice"] = tc
			}
		default:
			claudeReq["tool_choice"] = tc
		}
	}
	if parallel, ok := req["parallel_tool_calls"].(bool); ok && !parallel {
		choice, _ := claudeReq["tool_choice"].(map[string]any)
		if choice == nil {
			choice = map[string]any{"type": "auto"}
		}
		choice["disable_parallel_tool_use"] = true
		claudeReq["tool_choice"] = choice
	}
	adaptiveApplied := false
	if _, hasExplicitThinking := req["thinking"]; !hasExplicitThinking {
		if effort, _ := req["reasoning_effort"].(string); effort != "" {
			model, _ := claudeReq["model"].(string)
			adaptiveApplied, _ = ApplyAdaptiveAnthropicReasoning(claudeReq, model, effort)
		}
	}
	if t := BuildAnthropicThinking(req); t != nil && !adaptiveApplied {
		if thinking, ok := t.(map[string]any); ok {
			if thinkingType, _ := thinking["type"].(string); strings.EqualFold(thinkingType, "enabled") {
				budget, _ := NumberFromAny(thinking["budget_tokens"])
				maxTokens, _ := NumberFromAny(claudeReq["max_tokens"])
				_, explicitMaxTokens := req["max_tokens"]
				if _, exists := req["max_completion_tokens"]; exists {
					explicitMaxTokens = true
				}
				if budget < 1024 {
					budget = 1024
				}
				if !explicitMaxTokens {
					maxTokens = budget + 4096
					if maxTokens > 65536 {
						maxTokens = 65536
					}
				}
				if maxTokens <= budget {
					if maxTokens < 2048 {
						maxTokens = 2048
					}
					budget = maxTokens - 1024
				}
				thinking["budget_tokens"] = int(budget)
				claudeReq["max_tokens"] = int(maxTokens)
				// 启用扩展思考时，Anthropic 只接受默认 temperature。
				// 省略该字段可让上游应用有效默认值，避免显式的不兼容值被拒绝。
				delete(claudeReq, "temperature")
			}
		}
		claudeReq["thinking"] = t
		if thinking, ok := t.(map[string]any); ok && strings.EqualFold(BridgeString(thinking["type"]), "adaptive") {
			if outputConfig, exists := req["output_config"]; exists {
				claudeReq["output_config"] = outputConfig
			}
		}
	}

	result, _ := json.Marshal(claudeReq)
	return result
}

func ConvertOpenAIImageToAnthropic(part map[string]any) map[string]any {
	imgURL, _ := part["image_url"].(map[string]any)
	if imgURL == nil {
		return part
	}
	url, _ := imgURL["url"].(string)
	if strings.HasPrefix(url, "data:") {
		parts := strings.SplitN(url, ",", 2)
		if len(parts) == 2 {
			mediaType := strings.TrimPrefix(parts[0], "data:")
			if idx := strings.Index(mediaType, ";"); idx >= 0 {
				mediaType = mediaType[:idx]
			}
			return map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": mediaType,
					"data":       parts[1],
				},
			}
		}
	}
	return map[string]any{
		"type": "image",
		"source": map[string]any{
			"type": "url",
			"url":  url,
		},
	}
}

func ConvertOpenAIFileToAnthropic(part map[string]any) (map[string]any, bool) {
	file, _ := part["file"].(map[string]any)
	if file == nil {
		file = part
	}
	title, _ := file["filename"].(string)
	document := map[string]any{"type": "document"}
	if title != "" {
		document["title"] = title
	}
	if fileID, _ := file["file_id"].(string); fileID != "" {
		document["source"] = map[string]any{"type": "file", "file_id": fileID}
		return document, true
	}
	if fileURL, _ := file["file_url"].(string); fileURL != "" {
		document["source"] = map[string]any{"type": "url", "url": fileURL}
		return document, true
	}
	fileData, _ := file["file_data"].(string)
	if fileData == "" {
		return nil, false
	}
	if strings.HasPrefix(fileData, "data:") {
		parts := strings.SplitN(fileData, ",", 2)
		if len(parts) != 2 {
			return nil, false
		}
		mediaType := strings.TrimPrefix(parts[0], "data:")
		if semicolon := strings.Index(mediaType, ";"); semicolon >= 0 {
			mediaType = mediaType[:semicolon]
		}
		document["source"] = map[string]any{
			"type": "base64", "media_type": mediaType, "data": parts[1],
		}
		return document, true
	}
	document["source"] = map[string]any{"type": "base64", "media_type": "application/pdf", "data": fileData}
	return document, true
}

func ConvertOpenAIToolsToAnthropic(tools []any) []map[string]any {
	var result []map[string]any
	for _, t := range tools {
		tc, _ := t.(map[string]any)
		if tc == nil {
			continue
		}
		fn, _ := tc["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params := fn["parameters"]
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		result = append(result, map[string]any{
			"name": name, "description": desc, "input_schema": params,
		})
	}
	return result
}

// OpenAIToResponsesRequest 将 OpenAI Chat 请求转为 OpenAI Responses API 格式
func OpenAIToResponsesRequest(body []byte, upstream *UpstreamConfig) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	if err := NormalizeRawMessagesToolCallArguments(req["messages"]); err != nil {
		log.Printf("警告：规范化原始消息中的工具调用参数失败：%v", err)
	}

	msgs, _ := req["messages"].([]any)
	var instructions []string
	var input []any

	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		content := msg["content"]

		if role == "system" || role == "developer" {
			if text := ExtractTextFromContentParts(content); text != "" {
				instructions = append(instructions, text)
			}
			continue
		}

		switch role {
		case "assistant":
			if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
				input = append(input, map[string]any{
					"type":    "reasoning",
					"summary": []any{map[string]any{"type": "summary_text", "text": rc}},
				})
			}
			if responsesContent, ok := ChatContentToResponsesContent(content); ok {
				input = append(input, map[string]any{
					"role":    "assistant",
					"content": responsesContent,
				})
			}
			if tcs, ok := msg["tool_calls"].([]any); ok && len(tcs) > 0 {
				for _, tc := range tcs {
					tcMap, _ := tc.(map[string]any)
					if tcMap == nil {
						continue
					}
					id, _ := tcMap["id"].(string)
					fn, _ := tcMap["function"].(map[string]any)
					if fn == nil {
						continue
					}
					name, _ := fn["name"].(string)
					if name == "" {
						continue
					}
					args := JsonStringValue(fn["arguments"], "{}")
					if id == "" {
						id = "call_" + RandomString(16)
					}
					input = append(input, map[string]any{
						"type":      "function_call",
						"call_id":   id,
						"name":      name,
						"arguments": args,
					})
				}
			}
		case "tool":
			toolCallID, _ := msg["tool_call_id"].(string)
			if toolCallID == "" {
				continue
			}
			output, ok := ChatContentToResponsesContent(content)
			if !ok {
				output = ""
			}
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": toolCallID,
				"output":  output,
			})
		default:
			if role == "" {
				role = "user"
			}
			responsesContent, ok := ChatContentToResponsesContent(content)
			if !ok {
				responsesContent = ""
			}
			input = append(input, map[string]any{
				"role":    role,
				"content": responsesContent,
			})
		}
	}

	respReq := map[string]any{
		"model": req["model"],
	}
	if len(instructions) > 0 {
		respReq["instructions"] = strings.Join(instructions, "\n")
	}
	if len(input) > 0 {
		respReq["input"] = input
	}
	if stream, _ := req["stream"].(bool); stream {
		respReq["stream"] = true
	}
	if temp, ok := req["temperature"]; ok && temp != nil {
		respReq["temperature"] = temp
	}
	if topP, ok := req["top_p"]; ok && topP != nil {
		respReq["top_p"] = topP
	}
	if mt, _ := req["max_tokens"].(float64); mt > 0 {
		respReq["max_output_tokens"] = int(mt)
	} else if mt, _ := req["max_completion_tokens"].(float64); mt > 0 {
		respReq["max_output_tokens"] = int(mt)
	}
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		respReq["tools"] = ChatToolsToResponsesTools(tools)
	}
	if tc, ok := req["tool_choice"]; ok {
		respReq["tool_choice"] = ChatToolChoiceToResponses(tc)
	}
	if ptc, ok := req["parallel_tool_calls"]; ok {
		respReq["parallel_tool_calls"] = ptc
	}
	for _, key := range []string{
		"store", "include", "metadata", "service_tier", "truncation",
		"background", "max_tool_calls", "previous_response_id", "prompt",
		"prompt_cache_key", "prompt_cache_options", "prompt_cache_retention",
		"safety_identifier", "moderation",
	} {
		if value, ok := req[key]; ok && value != nil {
			respReq[key] = value
		}
	}
	if _, exists := respReq["safety_identifier"]; !exists {
		if user, _ := req["user"].(string); user != "" {
			respReq["safety_identifier"] = user
		}
	}
	if streamOptions, ok := req["stream_options"].(map[string]any); ok {
		if includeObfuscation, exists := streamOptions["include_obfuscation"]; exists {
			respReq["stream_options"] = map[string]any{"include_obfuscation": includeObfuscation}
		}
	}
	if responseFormat, ok := req["response_format"].(map[string]any); ok {
		format := CloneAnyMap(responseFormat)
		if jsonSchema, ok := format["json_schema"].(map[string]any); ok {
			flattened := CloneAnyMap(jsonSchema)
			flattened["type"] = "json_schema"
			format = flattened
		}
		respReq["text"] = map[string]any{"format": format}
	} else if textConfig, ok := req["text"]; ok && textConfig != nil {
		respReq["text"] = textConfig
	}
	if re, ok := req["reasoning_effort"].(string); ok && re != "" {
		SetResponsesReasoningEffort(respReq, re, upstream)
	} else if effort := ReasoningEffortFromThinking(req["thinking"]); effort != "" && effort != "none" {
		SetResponsesReasoningEffort(respReq, effort, upstream)
	}

	result, _ := json.Marshal(respReq)
	return result
}

// ChatContentToResponsesContent 转换 Chat 内容分段且不丢弃图片。
// 仅当源数据完全没有可表示内容时返回 false。
func ChatContentToResponsesContent(content any) (any, bool) {
	if content == nil {
		return nil, false
	}
	if text, ok := content.(string); ok {
		return text, true
	}
	parts, ok := content.([]any)
	if !ok {
		return JsonStringValue(content, ""), true
	}

	converted := make([]any, 0, len(parts))
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		partType, _ := part["type"].(string)
		switch partType {
		case "text", "input_text", "output_text":
			if text, ok := part["text"].(string); ok {
				converted = append(converted, map[string]any{"type": "input_text", "text": text})
			}
		case "image_url", "input_image":
			if imageURL := ResponsesImageURLFromPart(part); imageURL != nil {
				image := map[string]any{
					"type":      "input_image",
					"image_url": imageURL["url"],
				}
				if detail, ok := imageURL["detail"]; ok {
					image["detail"] = detail
				}
				converted = append(converted, image)
			}
		case "image_file":
			imageFile, _ := part["image_file"].(map[string]any)
			if fileID, _ := imageFile["file_id"].(string); fileID != "" {
				converted = append(converted, map[string]any{"type": "input_image", "file_id": fileID})
			}
		case "input_file":
			// input_file 已是原生 Responses 内容分段。
			converted = append(converted, CloneAnyMap(part))
		case "file":
			file, _ := part["file"].(map[string]any)
			if file == nil {
				continue
			}
			inputFile := map[string]any{"type": "input_file"}
			for _, key := range []string{"file_id", "file_data", "file_url", "filename"} {
				if value, exists := file[key]; exists {
					inputFile[key] = value
				}
			}
			if len(inputFile) > 1 {
				converted = append(converted, inputFile)
			}
		}
	}
	if len(converted) == 0 {
		return nil, false
	}
	return converted, true
}

func ChatToolsToResponsesTools(tools []any) []any {
	converted := make([]any, 0, len(tools))
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		if tool["type"] != "function" {
			converted = append(converted, CloneAnyMap(tool))
			continue
		}
		fn, nested := tool["function"].(map[string]any)
		if !nested {
			// 已经是扁平的 Responses 函数工具格式。
			converted = append(converted, CloneAnyMap(tool))
			continue
		}
		flat := map[string]any{"type": "function"}
		for _, key := range []string{"name", "description", "parameters", "strict"} {
			if value, exists := fn[key]; exists {
				flat[key] = value
			}
		}
		if _, exists := flat["parameters"]; !exists {
			flat["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		converted = append(converted, flat)
	}
	return converted
}

func ChatToolChoiceToResponses(choice any) any {
	choiceMap, ok := choice.(map[string]any)
	if !ok || choiceMap["type"] != "function" {
		return choice
	}
	fn, ok := choiceMap["function"].(map[string]any)
	if !ok {
		return CloneAnyMap(choiceMap)
	}
	flat := map[string]any{"type": "function"}
	if name, ok := fn["name"]; ok {
		flat["name"] = name
	}
	return flat
}

func CloneAnyMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func JsonStringValue(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(encoded)
}

// ConvertResponsesToChat 将 OpenAI Responses API 响应转为 OpenAI Chat 格式
func ConvertResponsesToChat(body []byte, modelID string) []byte {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	output, ok := resp["output"].([]any)
	if !ok {
		return nil
	}

	totalText := ""
	totalReasoning := ""
	encryptedReasoning := ""
	totalRefusal := ""
	var toolCalls []map[string]any
	var providerOutput []any
	for _, item := range output {
		if m, ok := item.(map[string]any); ok {
			switch m["type"] {
			case "reasoning":
				reasoning := ""
				if summary, ok := m["summary"].([]any); ok {
					for _, s := range summary {
						if sm, ok := s.(map[string]any); ok {
							if t, ok := sm["text"].(string); ok {
								reasoning += t
							}
						}
					}
				}
				if reasoning != "" {
					totalReasoning += reasoning
				}
				if encrypted, _ := m["encrypted_content"].(string); encrypted != "" {
					encryptedReasoning = encrypted
				}
			case "message":
				if content, ok := m["content"].(string); ok {
					totalText += content
				} else if content, ok := m["content"].([]any); ok {
					for _, block := range content {
						if b, ok := block.(map[string]any); ok {
							switch b["type"] {
							case "output_text", "text":
								if t, ok := b["text"].(string); ok {
									totalText += t
								}
							case "refusal":
								if refusal, ok := b["refusal"].(string); ok {
									totalRefusal += refusal
								}
							}
						}
					}
				}
			case "function_call":
				callID, _ := m["call_id"].(string)
				if callID == "" {
					callID, _ = m["id"].(string)
				}
				name, _ := m["name"].(string)
				if callID == "" || name == "" {
					continue
				}
				args := JsonStringValue(m["arguments"], "{}")
				toolCalls = append(toolCalls, map[string]any{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": args,
					},
				})
			case "custom_tool_call":
				callID, _ := m["call_id"].(string)
				if callID == "" {
					callID, _ = m["id"].(string)
				}
				name, _ := m["name"].(string)
				inputText, _ := m["input"].(string)
				if callID == "" || name == "" {
					providerOutput = append(providerOutput, CloneAnyMap(m))
					continue
				}
				args, _ := json.Marshal(map[string]any{"input": inputText})
				toolCalls = append(toolCalls, map[string]any{
					"id":   callID,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(args),
					},
				})
			default:
				providerOutput = append(providerOutput, CloneAnyMap(m))
			}
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if status, _ := resp["status"].(string); status == "incomplete" {
		finishReason = "length"
		if details, ok := resp["incomplete_details"].(map[string]any); ok {
			if reason, _ := details["reason"].(string); reason == "content_filter" {
				finishReason = "content_filter"
			}
		}
	}
	var messageContent any = totalText
	if totalText == "" && len(toolCalls) > 0 {
		messageContent = nil
	}
	message := map[string]any{
		"role":    "assistant",
		"content": messageContent,
	}
	if totalReasoning != "" {
		message["reasoning_content"] = totalReasoning
	}
	if encryptedReasoning != "" {
		message["reasoning_encrypted_content"] = encryptedReasoning
	}
	if totalRefusal != "" {
		message["refusal"] = totalRefusal
	}
	if len(providerOutput) > 0 {
		message["provider_output"] = providerOutput
	}
	choice := map[string]any{
		"index":         0,
		"message":       message,
		"finish_reason": finishReason,
	}
	responseID, _ := resp["id"].(string)
	if responseID == "" {
		responseID = "resp_" + RandomString(16)
	}
	if len(toolCalls) > 0 {
		choice["message"].(map[string]any)["tool_calls"] = toolCalls
	}
	createdAt := time.Now().Unix()
	if created, ok := GetFloat(resp, "created_at"); ok && created > 0 {
		createdAt = int64(created)
	}
	if modelID == "" {
		modelID, _ = resp["model"].(string)
	}

	chatResp := map[string]any{
		"id":      responseID,
		"object":  "chat.completion",
		"created": createdAt,
		"model":   modelID,
		"choices": []map[string]any{choice},
	}
	if usage, ok := resp["usage"].(map[string]any); ok {
		chatResp["usage"] = ResponsesUsageToChatUsage(usage)
	}

	result, _ := json.Marshal(chatResp)
	return result
}

func ResponsesUsageToChatUsage(usage map[string]any) map[string]any {
	if usage == nil {
		return nil
	}
	converted := map[string]any{}
	if value, ok := usage["input_tokens"]; ok {
		converted["prompt_tokens"] = value
	} else if value, ok := usage["prompt_tokens"]; ok {
		converted["prompt_tokens"] = value
	}
	if value, ok := usage["output_tokens"]; ok {
		converted["completion_tokens"] = value
	} else if value, ok := usage["completion_tokens"]; ok {
		converted["completion_tokens"] = value
	}
	if details, ok := usage["input_tokens_details"]; ok {
		converted["prompt_tokens_details"] = details
	} else if details, ok := usage["prompt_tokens_details"]; ok {
		converted["prompt_tokens_details"] = details
	}
	if details, ok := usage["output_tokens_details"]; ok {
		converted["completion_tokens_details"] = details
	} else if details, ok := usage["completion_tokens_details"]; ok {
		converted["completion_tokens_details"] = details
	}
	if total, ok := usage["total_tokens"]; ok {
		converted["total_tokens"] = total
	} else {
		prompt, _ := GetFloat(converted, "prompt_tokens")
		completion, _ := GetFloat(converted, "completion_tokens")
		converted["total_tokens"] = int64(prompt + completion)
	}
	return converted
}

// ======================== 消息处理 ========================
// NormalizeContent 仅作透明管道透传：保留 string 与 []any 两种入参形状
// （其他非常规类型使用 json.Marshal 兜底），不解析或过滤任何多模态分段。
// 能力协商由 opencode 客户端 + 上游负责；这里既不"硬降级"也不"补全"。
func NormalizeContent(content any) any {
	if content == nil {
		return nil
	}
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]any); ok {
		return arr
	}
	b, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return string(b)
}

func FixToolCallGaps(messages []Message) []Message {
	// 不得伪造或重排对话历史。缺失工具结果本身是有意义的应用状态；
	// 虚构结果会改变提示词，可能让模型依据工具从未返回的数据执行操作。
	// 保留此辅助函数作为调用点兼容层，同时原样保留请求。
	return messages
}

func NormalizeToolCallArguments(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", err
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return "", fmt.Errorf("must decode to JSON object, got %T", parsed)
	}
	normalized, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func ToolCallArgumentsPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return TruncatePreview(raw, 160)
}

func TruncatePreview(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	runes := []rune(raw)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return raw
}

func MessageContentPreview(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return TruncatePreview(v, 160)
	case []any:
		return TruncatePreview(ExtractTextFromContentParts(v), 160)
	default:
		b, _ := json.Marshal(v)
		return TruncatePreview(string(b), 160)
	}
}

func LogToolCallArgumentsValidationFailure(source string, messageIndex, toolCallIndex int, toolCallID, toolName, rawArgs string, content any, err error) {
	argumentsPreview := "[redacted]"
	contentPreview := "[redacted]"
	if debugMode && debugLogBodies {
		argumentsPreview = ToolCallArgumentsPreview(rawArgs)
		contentPreview = MessageContentPreview(content)
	}
	log.Printf("[工具调用参数无效] 来源=%s 消息索引=%d 工具调用索引=%d 调用ID=%q 工具名=%q 参数长度=%d 参数预览=%q 内容预览=%q 错误=%v",
		source,
		messageIndex,
		toolCallIndex,
		toolCallID,
		toolName,
		len(rawArgs),
		argumentsPreview,
		contentPreview,
		err,
	)
}

func LogStreamToolCallArgumentsValidationFailure(source, itemID, callID, toolName, rawArgs string, outputIndex int, err error) {
	argumentsPreview := "[redacted]"
	if debugMode && debugLogBodies {
		argumentsPreview = ToolCallArgumentsPreview(rawArgs)
	}
	log.Printf("[工具调用流无效] 来源=%s 项目ID=%q 输出索引=%d 调用ID=%q 工具名=%q 参数长度=%d 参数预览=%q 错误=%v",
		source,
		itemID,
		outputIndex,
		callID,
		toolName,
		len(rawArgs),
		argumentsPreview,
		err,
	)
}

func NormalizeMessagesToolCallArguments(messages []Message) ([]Message, error) {
	for i := range messages {
		if messages[i].Role != "assistant" || len(messages[i].ToolCalls) == 0 {
			continue
		}
		for j := range messages[i].ToolCalls {
			normalized, err := NormalizeToolCallArguments(messages[i].ToolCalls[j].Function.Arguments)
			if err != nil {
				LogToolCallArgumentsValidationFailure(
					"NormalizeMessagesToolCallArguments",
					i,
					j,
					messages[i].ToolCalls[j].ID,
					messages[i].ToolCalls[j].Function.Name,
					messages[i].ToolCalls[j].Function.Arguments,
					messages[i].Content,
					err,
				)
				return nil, fmt.Errorf("messages[%d].tool_calls[%d].function.arguments invalid JSON object string: %w; preview=%q", i, j, err, ToolCallArgumentsPreview(messages[i].ToolCalls[j].Function.Arguments))
			}
			messages[i].ToolCalls[j].Function.Arguments = normalized
		}
	}
	return messages, nil
}

func NormalizeRawMessagesToolCallArguments(rawMessages any) error {
	msgs, ok := rawMessages.([]any)
	if !ok {
		return nil
	}
	for i, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		rawToolCalls, ok := msg["tool_calls"].([]any)
		if !ok {
			continue
		}
		for j, rawToolCall := range rawToolCalls {
			tc, ok := rawToolCall.(map[string]any)
			if !ok || tc == nil {
				continue
			}
			toolCallID, _ := tc["id"].(string)
			fn, ok := tc["function"].(map[string]any)
			if !ok || fn == nil {
				continue
			}
			toolName, _ := fn["name"].(string)
			var rawArgs string
			switch v := fn["arguments"].(type) {
			case string:
				rawArgs = v
			case nil:
				rawArgs = ""
			default:
				b, _ := json.Marshal(v)
				rawArgs = string(b)
			}
			normalized, err := NormalizeToolCallArguments(rawArgs)
			if err != nil {
				LogToolCallArgumentsValidationFailure(
					"NormalizeRawMessagesToolCallArguments",
					i,
					j,
					toolCallID,
					toolName,
					rawArgs,
					msg["content"],
					err,
				)
				return fmt.Errorf("messages[%d].tool_calls[%d].function.arguments invalid JSON object string: %w; preview=%q", i, j, err, ToolCallArgumentsPreview(rawArgs))
			}
			fn["arguments"] = normalized
		}
	}
	return nil
}

func EnsureReasoningContent(messages []Message, withReasoning bool) []Message {
	// 仅在启用 WithReasoning（DeepSeek 上游）时注入空 reasoning_content。
	// 其他上游不需要该字段，并且可能拒绝未知字段。
	if !withReasoning {
		return messages
	}
	for i := range messages {
		if messages[i].Role == "assistant" && messages[i].ReasoningContent == nil {
			empty := ""
			messages[i].ReasoningContent = &empty
		}
	}
	return messages
}

func ConvertMessagesForUpstream(messages []Message, withReasoning bool) []map[string]any {
	converted := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		clean := make(map[string]any, len(msg.AdditionalFields)+6)
		for key, value := range msg.AdditionalFields {
			clean[key] = value
		}
		if msg.Role != "" {
			clean["role"] = msg.Role
		}
		content := NormalizeContent(msg.Content)
		reasoningContent := msg.ReasoningContent
		// 从系统消息中移除 x-anthropic-billing-header。
		if msg.Role == "system" {
			if s, ok := content.(string); ok {
				content = strings.TrimSpace(reBillingHeader.ReplaceAllString(s, ""))
				if content == "" {
					continue
				}
			} else if s, ok := content.([]any); ok {
				// 处理系统消息中的多分段内容。
				var cleaned []any
				for _, part := range s {
					p, ok := part.(map[string]any)
					if !ok {
						continue
					}
					if txt, ok := p["text"].(string); ok {
						txt = strings.TrimSpace(reBillingHeader.ReplaceAllString(txt, ""))
						if txt != "" {
							p["text"] = txt
							cleaned = append(cleaned, p)
						}
					}
				}
				if len(cleaned) == 0 {
					continue
				}
				content = cleaned
			}
		}
		shouldSendContent := content != nil
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			switch v := content.(type) {
			case string:
				shouldSendContent = strings.TrimSpace(v) != ""
			case []any:
				shouldSendContent = len(v) > 0
			}
		}
		if shouldSendContent {
			clean["content"] = content
		} else if (msg.Role == "assistant" && len(msg.ToolCalls) > 0) || msg.Role == "tool" {
			// 一些兼容 OpenAI 的供应商将 content 设为必填字段，尽管 OpenAI Schema
			// 允许助手工具调用消息不含文本。显式空字符串可保留纯工具调用语义，
			// 并避免供应商侧反序列化错误。
			clean["content"] = ""
		}
		if reasoningContent != nil && *reasoningContent != "" {
			clean["reasoning_content"] = *reasoningContent
		}
		if msg.ReasoningSignature != "" {
			clean["reasoning_signature"] = msg.ReasoningSignature
		}
		if msg.ReasoningEncryptedContent != "" {
			clean["reasoning_encrypted_content"] = msg.ReasoningEncryptedContent
		}
		if len(msg.ToolCalls) > 0 {
			clean["tool_calls"] = msg.ToolCalls
		}
		if msg.ToolCallID != "" {
			clean["tool_call_id"] = msg.ToolCallID
		}
		if msg.Name != "" {
			clean["name"] = msg.Name
		}
		converted = append(converted, clean)
	}
	return converted
}

// ======================== 完整请求转换（含 thinking/reasoning_effort/ExtraBody） ========================

func ConvertRequest(req *OpenAIRequest, withReasoning bool) map[string]any {
	converted := make(map[string]any, len(req.AdditionalFields)+10)
	for key, value := range req.AdditionalFields {
		converted[key] = value
	}
	converted["model"] = req.Model
	converted["messages"] = ConvertMessagesForUpstream(req.Messages, withReasoning)
	converted["stream"] = req.Stream
	if req.Temperature != nil {
		converted["temperature"] = *req.Temperature
	}
	if req.MaxTokens != 0 {
		converted["max_tokens"] = req.MaxTokens
	}
	// 为流式请求注入 stream_options.include_usage。
	if req.Stream {
		streamOptions := map[string]any{"include_usage": true}
		if existing, ok := req.StreamOptions.(map[string]any); ok {
			for k, v := range existing {
				streamOptions[k] = v
			}
			streamOptions["include_usage"] = true
		}
		converted["stream_options"] = streamOptions
	}
	if req.TopP != nil {
		converted["top_p"] = *req.TopP
	}
	if len(req.RawTools) > 0 {
		converted["tools"] = req.RawTools
	} else if len(req.Tools) > 0 {
		converted["tools"] = req.Tools
	}
	if req.ToolChoice != nil {
		converted["tool_choice"] = req.ToolChoice
	}

	// 模型映射中的 WithReasoning 控制是否向上游发送推理参数。
	// 部分兼容上游会严格校验 thinking.type，关闭后必须完整移除这些字段。
	if withReasoning {
		if req.Thinking != nil {
			converted["thinking"] = req.Thinking
		} else if req.ExtraBody != nil {
			if thinking, ok := req.ExtraBody["thinking"]; ok && thinking != nil {
				converted["thinking"] = thinking
			}
		}
		if req.ReasoningEffort != "" {
			if req.ConfiguredReasoningEffortMap != nil {
				converted["reasoning_effort"] = MapConfiguredReasoningEffort(req.ReasoningEffort, req.ConfiguredReasoningEffortMap)
			} else {
				converted["reasoning_effort"] = MapConfiguredReasoningEffort(req.ReasoningEffort)
			}
		}
	} else {
		delete(converted, "thinking")
		delete(converted, "reasoning_effort")
	}

	if req.ExtraBody != nil {
		for k, v := range req.ExtraBody {
			if !withReasoning && (k == "thinking" || k == "reasoning_effort") {
				continue
			}
			if _, exists := converted[k]; !exists {
				converted[k] = v
			}
		}
	}
	return converted
}
func BuildUpstreamBody(req *OpenAIRequest, withReasoning ...bool) []byte {
	wr := len(withReasoning) > 0 && withReasoning[0]
	converted := ConvertRequest(req, wr)
	b, err := json.Marshal(converted)
	if err != nil {
		log.Printf("序列化上游请求体失败：%v", err)
	}
	return b
}
