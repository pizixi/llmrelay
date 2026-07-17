package convert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ======================== Claude 消息接口 ========================

func ExtractClaudeSystemText(system any) string {
	if system == nil {
		return ""
	}
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func ClaudeMediaBlockToChat(block map[string]any) (map[string]any, bool) {
	blockType, _ := block["type"].(string)
	source, _ := block["source"].(map[string]any)
	if source == nil {
		return nil, false
	}
	sourceType, _ := source["type"].(string)
	mediaType, _ := source["media_type"].(string)
	data, _ := source["data"].(string)
	url, _ := source["url"].(string)
	switch blockType {
	case "image":
		if mediaType == "" {
			mediaType = "image/png"
		}
		if sourceType == "base64" && data != "" {
			url = "data:" + mediaType + ";base64," + data
		}
		if url == "" {
			return nil, false
		}
		return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}, true
	case "document":
		filename, _ := block["title"].(string)
		if filename == "" {
			filename, _ = source["filename"].(string)
		}
		if filename == "" {
			filename = "document"
		}
		if sourceType == "base64" && data != "" {
			if mediaType == "" {
				mediaType = "application/pdf"
			}
			return map[string]any{
				"type": "file",
				"file": map[string]any{"filename": filename, "file_data": "data:" + mediaType + ";base64," + data},
			}, true
		}
		if url != "" {
			return map[string]any{"type": "input_file", "file_url": url, "filename": filename}, true
		}
	}
	return nil, false
}

func CleanJsonSchema(schema any) any {
	// 完整保留 JSON Schema 约束。旧版兼容清理会删除 additionalProperties、format
	// 等有意义的校验关键字，即使 Claude 到 Claude 的请求也会改变工具语义。
	// 此处复制数据，避免修改调用方持有的值。
	switch value := schema.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			cloned[key] = CleanJsonSchema(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = CleanJsonSchema(item)
		}
		return cloned
	default:
		return value
	}
}

func ClaudeToOpenAIMessages(claudeMsgs []ClaudeMessage, system any) []Message {
	var messages []Message
	if sysText := ExtractClaudeSystemText(system); sysText != "" {
		messages = append(messages, Message{Role: "system", Content: sysText})
	}
	for _, msg := range claudeMsgs {
		switch content := msg.Content.(type) {
		case string:
			messages = append(messages, Message{Role: msg.Role, Content: content})
		case []any:
			var textParts []string
			var reasoningParts []string
			var toolCalls []ToolCall
			var toolResults []Message
			var imageParts []map[string]any
			var fileParts []map[string]any
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, ok := block["text"].(string); ok && text != "" {
						textParts = append(textParts, text)
					}
				case "image":
					source, _ := block["source"].(map[string]any)
					if source != nil {
						srcType, _ := source["type"].(string)
						mediaType, _ := source["media_type"].(string)
						data, _ := source["data"].(string)
						if srcType == "base64" && data != "" {
							if mediaType == "" {
								mediaType = "image/png"
							}
							imageParts = append(imageParts, map[string]any{
								"type": "image_url",
								"image_url": map[string]string{
									"url": "data:" + mediaType + ";base64," + data,
								},
							})
						} else if srcType == "url" {
							if url, ok := source["url"].(string); ok && url != "" {
								imageParts = append(imageParts, map[string]any{
									"type": "image_url",
									"image_url": map[string]string{
										"url": url,
									},
								})
							}
						}
					}
				case "document":
					if filePart, ok := ClaudeMediaBlockToChat(block); ok {
						fileParts = append(fileParts, filePart)
					}
				case "thinking":
					if thinking, ok := block["thinking"].(string); ok && thinking != "" {
						reasoningParts = append(reasoningParts, thinking)
					}
				case "tool_use", "server_tool_use":
					id, _ := block["id"].(string)
					name, _ := block["name"].(string)
					if blockType == "server_tool_use" && name == "web_search" {
						name = internalWebSearchToolName
					}
					if id == "" {
						encoded, _ := json.Marshal(block)
						sum := sha256.Sum256(encoded)
						id = fmt.Sprintf("srvtool_history_%x", sum[:8])
					}
					var args string
					switch input := block["input"].(type) {
					case string:
						args = input
					default:
						if input != nil {
							b, _ := json.Marshal(input)
							args = string(b)
						}
					}
					if args == "" {
						args = "{}"
					}
					toolCalls = append(toolCalls, ToolCall{
						ID:   id,
						Type: "function",
						Function: FunctionCall{
							Name:      name,
							Arguments: args,
						},
					})
				case "tool_result", "web_search_tool_result":
					toolUseID, _ := block["tool_use_id"].(string)
					isError, _ := block["is_error"].(bool)
					if blockType == "web_search_tool_result" && toolUseID == "" {
						encoded, _ := json.Marshal(block)
						sum := sha256.Sum256(encoded)
						toolUseID = fmt.Sprintf("srvtool_history_%x", sum[:8])
					}
					var resultContent any
					if blockType == "web_search_tool_result" {
						encoded, _ := json.Marshal(block["content"])
						resultContent = string(encoded)
					} else {
						switch c := block["content"].(type) {
						case string:
							resultContent = c
						case []any:
							var parts []any
							for _, p := range c {
								if pb, ok := p.(map[string]any); ok {
									if pb["type"] == "text" {
										if text, ok := pb["text"].(string); ok {
											parts = append(parts, map[string]any{"type": "text", "text": text})
										}
									} else if mediaPart, ok := ClaudeMediaBlockToChat(pb); ok {
										parts = append(parts, mediaPart)
									}
								}
							}
							if len(parts) > 0 {
								resultContent = parts
							}
						default:
							if c != nil {
								b, _ := json.Marshal(c)
								resultContent = string(b)
							}
						}
					}
					if isError {
						const errorPrefix = "[tool_error] "
						switch value := resultContent.(type) {
						case string:
							resultContent = errorPrefix + value
						case []any:
							resultContent = append([]any{map[string]any{"type": "text", "text": strings.TrimSpace(errorPrefix)}}, value...)
						case nil:
							resultContent = strings.TrimSpace(errorPrefix)
						}
					}
					toolResult := Message{
						Role:       "tool",
						ToolCallID: toolUseID,
						Content:    resultContent,
					}
					toolResults = append(toolResults, toolResult)
				}
			}
			om := Message{Role: msg.Role}
			if len(imageParts) > 0 || len(fileParts) > 0 {
				var contentArr []any
				for _, img := range imageParts {
					contentArr = append(contentArr, img)
				}
				for _, file := range fileParts {
					contentArr = append(contentArr, file)
				}
				if len(textParts) > 0 {
					contentArr = append(contentArr, map[string]any{
						"type": "text",
						"text": strings.Join(textParts, "\n"),
					})
				}
				om.Content = contentArr
			} else if len(textParts) > 0 {
				om.Content = strings.Join(textParts, "\n")
			} else if len(toolCalls) == 0 {
				om.Content = ""
			}
			if len(reasoningParts) > 0 {
				rc := strings.Join(reasoningParts, "\n")
				om.ReasoningContent = &rc
			}
			if len(toolCalls) > 0 {
				om.ToolCalls = toolCalls
			}
			hasPrimaryContent := len(textParts) > 0 || len(imageParts) > 0 || len(fileParts) > 0 || len(reasoningParts) > 0 || len(toolCalls) > 0
			if msg.Role == "user" && len(toolResults) > 0 {
				// Chat 要求工具结果紧跟助手的 tool_calls 消息。Claude 的单个用户轮次
				// 可能同时包含 tool_result 和普通文本，因此先输出全部工具消息，
				// 仅在确有内容时再追加真正的用户消息。
				messages = append(messages, toolResults...)
				if hasPrimaryContent {
					messages = append(messages, om)
				}
			} else {
				messages = append(messages, om)
				messages = append(messages, toolResults...)
			}
		default:
			b, _ := json.Marshal(content)
			messages = append(messages, Message{Role: msg.Role, Content: string(b)})
		}
	}
	return messages
}

func ClaudeToOpenAITools(claudeTools []ClaudeTool) ([]Tool, error) {
	tools, _, err := ClaudeToOpenAIToolsDetailed(claudeTools)
	return tools, err
}

func ClaudeToOpenAIToolsDetailed(claudeTools []ClaudeTool, enableHostedSearchFallback ...bool) ([]Tool, []BridgeWarning, error) {
	tools := make([]Tool, 0, len(claudeTools))
	var warnings []BridgeWarning
	allowHostedSearchFallback := len(enableHostedSearchFallback) > 0 && enableHostedSearchFallback[0]
	for i, ct := range claudeTools {
		if strings.TrimSpace(ct.Type) != "" {
			if allowHostedSearchFallback && strings.Contains(strings.ToLower(ct.Type), "web_search") {
				tools = append(tools, Tool{Type: "function", Function: WebSearchFallbackToolFunction(ResponsesTool{Type: "web_search"})})
				warnings = AppendBridgeWarning(warnings, BridgeWarning{
					Code: "hosted_web_search_fallback", Path: fmt.Sprintf("tools[%d]", i),
					Message: "Anthropic hosted web search will be executed by the gateway because the selected upstream has no native hosted search capability",
				})
				continue
			}
			warnings = AppendBridgeWarning(warnings, BridgeWarning{
				Code: "unsupported_anthropic_server_tool", Path: fmt.Sprintf("tools[%d]", i),
				Message: fmt.Sprintf("Anthropic server tool type %q is unavailable on the selected non-Anthropic upstream and was skipped", ct.Type),
			})
			continue
		}
		if strings.TrimSpace(ct.Name) == "" {
			return nil, warnings, fmt.Errorf("tools[%d].name is required", i)
		}
		params := ct.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		params = CleanJsonSchema(params)
		paramsMap, ok := params.(map[string]any)
		if !ok {
			return nil, warnings, fmt.Errorf("tools[%d].input_schema must be a JSON object", i)
		}
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        ct.Name,
				Description: ct.Description,
				Parameters:  paramsMap,
			},
		})
	}
	return tools, warnings, nil
}

func ClaudeToolChoiceToOpenAI(choice any) (any, *bool) {
	if choice == nil {
		return nil, nil
	}
	if text, ok := choice.(string); ok {
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "auto":
			return "auto", nil
		case "any", "required":
			return "required", nil
		case "none":
			return "none", nil
		default:
			return choice, nil
		}
	}
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return choice, nil
	}
	var parallel *bool
	if disabled, ok := choiceMap["disable_parallel_tool_use"].(bool); ok {
		enabled := !disabled
		parallel = &enabled
	}
	switch choiceType, _ := choiceMap["type"].(string); strings.ToLower(choiceType) {
	case "auto", "none":
		return strings.ToLower(choiceType), parallel
	case "any", "required":
		return "required", parallel
	case "tool":
		if name, _ := choiceMap["name"].(string); name != "" {
			return map[string]any{
				"type":     "function",
				"function": map[string]any{"name": name},
			}, parallel
		}
	}
	return choice, parallel
}

func OpenAIToClaudeResponse(chatBody []byte, model string) []byte {
	result, err := OpenAIToClaudeResponseWithError(chatBody, model)
	if err != nil {
		log.Printf("警告：OpenAI 响应转换为 Claude 响应失败：%v", err)
		return nil
	}
	return result
}

func OpenAIToClaudeResponseWithError(chatBody []byte, model string) ([]byte, error) {
	var chat struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Created int64  `json:"created"`
		Choices []struct {
			Message struct {
				Content                   string     `json:"content"`
				ReasoningContent          string     `json:"reasoning_content"`
				Reasoning                 string     `json:"reasoning"`
				ReasoningSignature        string     `json:"reasoning_signature"`
				ReasoningEncryptedContent string     `json:"reasoning_encrypted_content"`
				ProviderOutput            []any      `json:"provider_output"`
				Annotations               []any      `json:"annotations"`
				ToolCalls                 []ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(chatBody, &chat); err != nil {
		return nil, fmt.Errorf("invalid Chat response JSON: %w", err)
	}
	if len(chat.Choices) == 0 {
		return nil, fmt.Errorf("invalid Chat response: choices is empty")
	}

	content := []ClaudeContent{}
	stopReason := "end_turn"

	if len(chat.Choices) > 0 {
		msg := chat.Choices[0].Message
		fr := chat.Choices[0].FinishReason
		webSearchBlocks, _ := ChatWebSearchBlocks(msg.ProviderOutput)
		content = append(content, webSearchBlocks...)
		reasoning := msg.ReasoningContent
		if reasoning == "" {
			reasoning = msg.Reasoning
		}
		if reasoning != "" {
			content = append(content, ClaudeContent{
				Type:      "thinking",
				Thinking:  reasoning,
				Signature: msg.ReasoningSignature,
			})
		}
		if msg.ReasoningEncryptedContent != "" {
			content = append(content, ClaudeContent{
				Type: "redacted_thinking",
				Data: msg.ReasoningEncryptedContent,
			})
		}
		if msg.Content != "" {
			content = append(content, ClaudeContent{
				Type: "text",
				Text: msg.Content,
			})
		}
		for _, tc := range msg.ToolCalls {
			normalizedArguments, err := NormalizeToolCallArguments(tc.Function.Arguments)
			if err != nil {
				return nil, fmt.Errorf("tool call %q (%s) has invalid arguments: %w", tc.ID, tc.Function.Name, err)
			}
			var input any
			if err := json.Unmarshal([]byte(normalizedArguments), &input); err != nil {
				return nil, fmt.Errorf("tool call %q (%s) arguments decode failed: %w", tc.ID, tc.Function.Name, err)
			}
			content = append(content, ClaudeContent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		switch fr {
		case "stop":
			stopReason = "end_turn"
		case "length":
			stopReason = "max_tokens"
		case "tool_calls", "function_call":
			stopReason = "tool_use"
		case "content_filter":
			stopReason = "refusal"
		}
	}

	if len(content) == 0 {
		content = append(content, ClaudeContent{Type: "text", Text: ""})
	}

	resp := ClaudeResponse{
		ID:         fmt.Sprintf("msg_%s", RandomString(24)),
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      model,
		StopReason: stopReason,
		Usage:      &ClaudeUsage{},
	}
	if chat.Usage != nil {
		inputTokens, _ := GetFloat(chat.Usage, "input_tokens", "prompt_tokens")
		outputTokens, _ := GetFloat(chat.Usage, "output_tokens", "completion_tokens")
		resp.Usage.InputTokens = int(ToFloat64(inputTokens))
		resp.Usage.OutputTokens = int(ToFloat64(outputTokens))
		if promptDetails, ok := chat.Usage["prompt_tokens_details"].(map[string]any); ok {
			cached, _ := GetFloat(promptDetails, "cached_tokens")
			resp.Usage.CacheReadInputTokens = int(cached)
		}
		created, _ := GetFloat(chat.Usage, "cache_creation_input_tokens")
		resp.Usage.CacheCreationInputTokens = int(created)
	}
	webSearchRequests := ChatWebSearchEvidenceCount(
		chat.Choices[0].Message.ProviderOutput,
		chat.Choices[0].Message.Annotations,
	)
	if existing := AnthropicWebSearchRequestsFromUsage(chat.Usage); existing > webSearchRequests {
		webSearchRequests = existing
	}
	if webSearchRequests > 0 {
		resp.Usage.ServerToolUse = &ClaudeServerToolUsage{WebSearchRequests: webSearchRequests}
	}
	result, _ := json.Marshal(resp)
	return result, nil
}

func ToFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func GetFloat(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return n, true
			case float32:
				return float64(n), true
			case int:
				return float64(n), true
			case int64:
				return float64(n), true
			case int32:
				return float64(n), true
			}
		}
	}
	return 0, false
}
