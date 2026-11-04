package convert

// Anthropic 的 input_tokens 仅表示未命中、未写入缓存的输入；OpenAI 表示
// 全部输入。缓存读写必须在协议边界计入或扣除，不能直接复制 input_tokens。
func AnthropicUsageToChatUsage(usage map[string]any) map[string]any {
	input, _ := GetFloat(usage, "input_tokens", "prompt_tokens")
	output, _ := GetFloat(usage, "output_tokens", "completion_tokens")
	cached, hasCached := GetFloat(usage, "cache_read_input_tokens")
	created, hasCreated := GetFloat(usage, "cache_creation_input_tokens")
	input += cached + created
	result := map[string]any{
		"prompt_tokens": int64(input), "completion_tokens": int64(output),
		"total_tokens": int64(input + output),
	}
	details := map[string]any{}
	if hasCached {
		details["cached_tokens"] = int64(cached)
	}
	if hasCreated {
		details["cache_write_tokens"] = int64(created)
		// 保留旧版桥接载体，兼容仍通过顶层字段读取缓存写入用量的客户端。
		result["cache_creation_input_tokens"] = int64(created)
	}
	if len(details) > 0 {
		result["prompt_tokens_details"] = details
	}
	if outputDetails, ok := usage["output_tokens_details"].(map[string]any); ok {
		if thinking, ok := GetFloat(outputDetails, "thinking_tokens"); ok {
			result["completion_tokens_details"] = map[string]any{"reasoning_tokens": int64(thinking)}
		}
	}
	return result
}

// OpenAIUsageToAnthropicUsage 同时接受 Chat 与 Responses 的用量对象。
func OpenAIUsageToAnthropicUsage(usage map[string]any) map[string]any {
	input, _ := GetFloat(usage, "prompt_tokens", "input_tokens")
	output, _ := GetFloat(usage, "completion_tokens", "output_tokens")
	details, _ := usage["prompt_tokens_details"].(map[string]any)
	if details == nil {
		details, _ = usage["input_tokens_details"].(map[string]any)
	}
	cached, hasCached := GetFloat(details, "cached_tokens")
	created, hasCreated := GetFloat(usage, "cache_creation_input_tokens")
	if !hasCreated {
		created, hasCreated = GetFloat(details, "cache_write_tokens")
	}
	result := map[string]any{
		"input_tokens": int64(max(0, input-cached-created)), "output_tokens": int64(output),
	}
	if hasCached {
		result["cache_read_input_tokens"] = int64(cached)
	}
	if hasCreated {
		result["cache_creation_input_tokens"] = int64(created)
	}
	return result
}
