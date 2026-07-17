package convert

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// ======================== Anthropic 格式兼容 ========================

func IsAnthropicFormat(body []byte) bool {
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		if typ, _ := obj["type"].(string); typ == "message" {
			return true
		}
	}
	lines := bytes.Split(body, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		typ, _ := event["type"].(string)
		switch typ {
		case "message_start", "content_block_start", "content_block_delta",
			"content_block_stop", "message_delta", "message_stop", "ping":
			return true
		}
		return false
	}
	return false
}

func ParseAnthropicSSE(body []byte) (map[string]any, string, string, []map[string]any) {
	lines := bytes.Split(body, []byte("\n"))
	var anthropicMsg map[string]any
	var textBuilder, thinkingBuilder, currentToolInputBuilder strings.Builder
	var currentToolUse map[string]any
	var toolUseBlocks []map[string]any
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		typ, _ := event["type"].(string)
		switch typ {
		case "message_start":
			if m, ok := event["message"].(map[string]any); ok {
				anthropicMsg = m
			}
		case "content_block_start":
			if cb, ok := event["content_block"].(map[string]any); ok {
				if cbType, _ := cb["type"].(string); cbType == "tool_use" {
					currentToolUse = cb
					currentToolInputBuilder.Reset()
				}
			}
		case "content_block_delta":
			if delta, ok := event["delta"].(map[string]any); ok {
				if t, ok := delta["text"].(string); ok {
					textBuilder.WriteString(t)
				}
				if dt, _ := delta["type"].(string); dt == "thinking_delta" {
					if th, ok := delta["thinking"].(string); ok {
						thinkingBuilder.WriteString(th)
					}
				}
				if dt, _ := delta["type"].(string); dt == "input_json_delta" {
					if partial, ok := delta["partial_json"].(string); ok {
						currentToolInputBuilder.WriteString(partial)
					}
				}
			}
		case "content_block_stop":
			if currentToolUse != nil {
				inputStr := currentToolInputBuilder.String()
				var input any = inputStr
				var parsed any
				if json.Unmarshal([]byte(inputStr), &parsed) == nil {
					input = parsed
				}
				currentToolUse["input"] = input
				toolUseBlocks = append(toolUseBlocks, currentToolUse)
				currentToolUse = nil
			}
		case "message_delta":
			if delta, ok := event["delta"].(map[string]any); ok {
				if anthropicMsg == nil {
					anthropicMsg = map[string]any{}
				}
				if stop, ok := delta["stop_reason"].(string); ok {
					anthropicMsg["stop_reason"] = stop
				}
				if usage, ok := delta["usage"].(map[string]any); ok {
					anthropicMsg["usage"] = usage
				}
			}
		case "message_stop":
		case "error":
			return nil, "", "", nil
		}
	}
	return anthropicMsg, textBuilder.String(), thinkingBuilder.String(), toolUseBlocks
}

func BuildOpenAIResponse(anthropicMsg map[string]any, text string, reasoning string, toolUseBlocks []map[string]any, modelID string) []byte {
	if anthropicMsg == nil {
		return nil
	}
	now := time.Now().Unix()
	role, _ := anthropicMsg["role"].(string)
	if role == "" {
		role = "assistant"
	}
	finishReason, _ := anthropicMsg["stop_reason"].(string)
	switch finishReason {
	case "tool_use":
		finishReason = "tool_calls"
	case "end_turn", "stop_sequence", "pause_turn", "":
		finishReason = "stop"
	case "max_tokens", "model_context_window_exceeded":
		finishReason = "length"
	case "refusal":
		finishReason = "content_filter"
	default:
		// Chat 客户端通常会校验有限的 finish_reason 枚举。
		// 供应商特有的停止信息保留在日志中，但对外输出有效的终止值。
		finishReason = "stop"
	}
	message := map[string]any{"role": role, "content": text}
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	if content, ok := anthropicMsg["content"].([]any); ok {
		for _, rawBlock := range content {
			block, _ := rawBlock.(map[string]any)
			switch blockType, _ := block["type"].(string); blockType {
			case "thinking":
				if signature, _ := block["signature"].(string); signature != "" {
					message["reasoning_signature"] = signature
				}
			case "redacted_thinking":
				if encrypted, _ := block["data"].(string); encrypted != "" {
					message["reasoning_encrypted_content"] = encrypted
				}
			}
		}
	}
	choice := map[string]any{
		"index":         0,
		"message":       message,
		"finish_reason": finishReason,
	}
	if len(toolUseBlocks) > 0 {
		var toolCalls []map[string]any
		for _, tb := range toolUseBlocks {
			toolInput := tb["input"]
			argsJSON, _ := json.Marshal(toolInput)
			toolCalls = append(toolCalls, map[string]any{
				"id":   tb["id"],
				"type": "function",
				"function": map[string]any{
					"name":      tb["name"],
					"arguments": string(argsJSON),
				},
			})
		}
		choice["message"].(map[string]any)["tool_calls"] = toolCalls
		if text == "" {
			choice["message"].(map[string]any)["content"] = nil
		}
	}
	resp := map[string]any{
		"id":      anthropicMsg["id"],
		"object":  "chat.completion",
		"created": now,
		"model":   modelID,
		"choices": []map[string]any{choice},
	}
	if usage, ok := anthropicMsg["usage"].(map[string]any); ok {
		openAIUsage := map[string]any{}
		if v, ok := usage["input_tokens"]; ok {
			openAIUsage["prompt_tokens"] = v
		}
		if v, ok := usage["output_tokens"]; ok {
			openAIUsage["completion_tokens"] = v
		}
		if cached, ok := usage["cache_read_input_tokens"]; ok {
			openAIUsage["prompt_tokens_details"] = map[string]any{"cached_tokens": cached}
		}
		if created, ok := usage["cache_creation_input_tokens"]; ok {
			openAIUsage["cache_creation_input_tokens"] = created
		}
		if pt, ok1 := openAIUsage["prompt_tokens"]; ok1 {
			if ct, ok2 := openAIUsage["completion_tokens"]; ok2 {
				ptF, _ := pt.(float64)
				ctF, _ := ct.(float64)
				openAIUsage["total_tokens"] = int64(ptF + ctF)
			}
		}
		resp["usage"] = openAIUsage
	}
	result, _ := json.Marshal(resp)
	return result
}

func ConvertAnthropicMessageToOpenAI(msg map[string]any, modelID string) []byte {
	if msg["model"] == nil {
		msg["model"] = modelID
	}
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	var toolUses []map[string]any
	if content, ok := msg["content"].([]any); ok {
		for _, c := range content {
			if block, ok := c.(map[string]any); ok {
				switch block["type"] {
				case "text":
					if t, ok := block["text"].(string); ok {
						textBuilder.WriteString(t)
					}
				case "thinking":
					if t, ok := block["thinking"].(string); ok {
						thinkingBuilder.WriteString(t)
					}
				case "tool_use":
					toolUses = append(toolUses, block)
				}
			}
		}
	}
	return BuildOpenAIResponse(msg, textBuilder.String(), thinkingBuilder.String(), toolUses, modelID)
}

func ConvertAnthropicToOpenAI(body []byte, modelID string) []byte {
	var singleMsg map[string]any
	if json.Unmarshal(body, &singleMsg) == nil {
		if typ, _ := singleMsg["type"].(string); typ == "message" {
			if _, ok := singleMsg["content"].([]any); !ok {
				return nil
			}
			return ConvertAnthropicMessageToOpenAI(singleMsg, modelID)
		}
	}
	msg, text, reasoning, toolUses := ParseAnthropicSSE(body)
	if msg == nil {
		return body
	}
	if msg["model"] == nil {
		msg["model"] = modelID
	}
	return BuildOpenAIResponse(msg, text, reasoning, toolUses, modelID)
}

// ======================== 响应清理 ========================

func CleanNulls(m map[string]any) {
	for k, v := range m {
		if v == nil {
			delete(m, k)
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			delete(m, k)
		}
	}
}

func HasNonEmptyString(value any) bool {
	s, ok := value.(string)
	return ok && s != ""
}

func NormalizeReasoningContent(m map[string]any) {
	if m == nil || HasNonEmptyString(m["reasoning_content"]) {
		return
	}
	if v, ok := m["reasoning"]; ok {
		m["reasoning_content"] = v
	}
}

func CleanStreamDelta(delta map[string]any) {
	NormalizeReasoningContent(delta)
	if v, ok := delta["content"]; ok && v == nil {
		delete(delta, "content")
	}
	if s, ok := delta["content"].(string); ok && s == "" {
		delete(delta, "content")
	}
	if v, ok := delta["reasoning_content"]; ok && v == nil {
		delete(delta, "reasoning_content")
	}
	if s, ok := delta["reasoning_content"].(string); ok && s == "" {
		delete(delta, "reasoning_content")
	}
	if s, ok := delta["role"].(string); ok && s == "" {
		delete(delta, "role")
	}
}

// ConvertStreamChunkWithUsage 转换流式 chunk 并同时提取 usage，避免二次解析
func ConvertStreamChunkWithUsage(line string) (string, map[string]any) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "data: [DONE]" || trimmed == "[DONE]" {
		return line, nil
	}
	if !strings.HasPrefix(line, "data: ") {
		return line, nil
	}
	data := line[6:]
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return line, nil
	}

	// 提取 usage
	var usage map[string]any
	if u, ok := raw["usage"].(map[string]any); ok {
		usage = u
	}

	choices, ok := raw["choices"].([]any)
	if !ok || len(choices) == 0 {
		// 启用 stream_options.include_usage 后，仅包含 Usage 的数据块属于 OpenAI
		// 流式协议的一部分。应将其保留给客户端，不能只用于本地统计。
		// 同理，不能仅因流内错误没有 choices 数组就将其吞掉。
		eventType, _ := raw["type"].(string)
		if usage != nil || raw["error"] != nil || eventType == "error" || eventType == "response.failed" {
			delete(raw, "cost")
			converted, err := json.Marshal(raw)
			if err == nil {
				return "data: " + string(converted), usage
			}
			return line, usage
		}
		return "", usage
	}
	for i, c := range choices {
		choice, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if delta, ok := choice["delta"].(map[string]any); ok {
			CleanStreamDelta(delta)
			choice["delta"] = delta
		}
		if msg, ok := choice["message"].(map[string]any); ok {
			NormalizeReasoningContent(msg)
			CleanNulls(msg)
			choice["message"] = msg
		}
		if v, ok := choice["logprobs"]; ok && v == nil {
			delete(choice, "logprobs")
		}
		if v, ok := choice["finish_reason"]; ok && v == nil {
			delete(choice, "finish_reason")
		}
		if s, ok := choice["finish_reason"].(string); ok && s == "" {
			delete(choice, "finish_reason")
		}
		choices[i] = choice
	}
	raw["choices"] = choices
	if v, ok := raw["usage"]; ok && v == nil {
		delete(raw, "usage")
	}
	delete(raw, "cost")
	converted, err := json.Marshal(raw)
	if err != nil {
		return line, usage
	}
	return "data: " + string(converted), usage
}

func ConvertResponse(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("警告：解析待转换响应失败：%v", err)
		return data, nil
	}
	if choices, ok := raw["choices"].([]any); ok {
		for i, c := range choices {
			if choice, ok := c.(map[string]any); ok {
				if msg, ok := choice["message"].(map[string]any); ok {
					NormalizeReasoningContent(msg)
					CleanNulls(msg)
					choice["message"] = msg
				}
				if v, ok := choice["logprobs"]; ok && v == nil {
					delete(choice, "logprobs")
				}
				choices[i] = choice
			}
		}
		raw["choices"] = choices
	}
	if usage, ok := raw["usage"].(map[string]any); ok {
		cleanU := map[string]any{
			"prompt_tokens":     usage["prompt_tokens"],
			"completion_tokens": usage["completion_tokens"],
			"total_tokens":      usage["total_tokens"],
		}
		raw["usage"] = cleanU
	}
	delete(raw, "cost")
	delete(raw, "system_fingerprint")
	return json.Marshal(raw)
}

func CloneBodyMap(bodyMap map[string]any) map[string]any {
	cp := make(map[string]any, len(bodyMap)+1)
	for k, v := range bodyMap {
		cp[k] = v
	}
	if thinking, ok := cp["thinking"].(map[string]any); ok {
		clone := make(map[string]any, len(thinking))
		for k, v := range thinking {
			clone[k] = v
		}
		cp["thinking"] = clone
	}
	if thinking, ok := cp["thinking"].(map[string]string); ok {
		clone := make(map[string]any, len(thinking))
		for k, v := range thinking {
			clone[k] = v
		}
		cp["thinking"] = clone
	}
	if messages, ok := cp["messages"].([]map[string]any); ok {
		clone := make([]map[string]any, 0, len(messages))
		for _, msg := range messages {
			msgClone := make(map[string]any, len(msg))
			for k, v := range msg {
				msgClone[k] = v
			}
			clone = append(clone, msgClone)
		}
		cp["messages"] = clone
	}
	if messages, ok := cp["messages"].([]any); ok {
		clone := make([]any, 0, len(messages))
		for _, item := range messages {
			if msg, ok := item.(map[string]any); ok {
				msgClone := make(map[string]any, len(msg))
				for k, v := range msg {
					msgClone[k] = v
				}
				clone = append(clone, msgClone)
			} else {
				clone = append(clone, item)
			}
		}
		cp["messages"] = clone
	}
	return cp
}
