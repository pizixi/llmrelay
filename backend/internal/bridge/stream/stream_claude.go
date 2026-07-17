package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func ClaudeStreamHandler(w http.ResponseWriter, respBody io.ReadCloser, model, usageModel string) {
	defer respBody.Close()
	usageStats := NewRequestUsageAccumulator(usageModel)
	defer usageStats.commit()
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(respBody)

	msgID := fmt.Sprintf("msg_%s", RandomString(24))
	blockIndex := 0
	thinkingBlockOpen := false
	thinkingBlockIndex := -1
	textBlockOpen := false
	textBlockIndex := -1
	toolCallAccumulator := map[int]map[string]string{}
	toolBlockIndexes := map[int]int{}
	toolCallOrder := []int{}
	messageStartSent := false
	fullUsage := map[string]any{}
	webSearchRequests := 0
	terminalFinishSeen := false
	pendingFinishReason := ""
	defer func() { usageStats.observeMap(fullUsage) }()

	emitClaudeEvent := func(event string, data any) {
		jsonData, err := json.Marshal(data)
		if err != nil {
			log.Printf("序列化 Claude SSE 事件失败：%v", err)
			return
		}
		w.Write([]byte("event: " + event + "\n"))
		w.Write([]byte("data: " + string(jsonData) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	emitClaudeError := func(payload map[string]any, fallbackMessage string) {
		errorObj := StreamErrorObject(payload, fallbackMessage)
		anthropicError := map[string]any{
			"type":    errorObj["type"],
			"message": errorObj["message"],
		}
		emitClaudeEvent("error", map[string]any{
			"type":  "error",
			"error": anthropicError,
		})
	}

	closeThinkingBlock := func() {
		if !thinkingBlockOpen {
			return
		}
		emitClaudeEvent("content_block_stop", map[string]any{
			"type":          "content_block_stop",
			"index":         thinkingBlockIndex,
			"content_block": map[string]any{"type": "thinking"},
		})
		thinkingBlockOpen = false
		thinkingBlockIndex = -1
	}

	closeTextBlock := func() {
		if !textBlockOpen {
			return
		}
		emitClaudeEvent("content_block_stop", map[string]any{
			"type":          "content_block_stop",
			"index":         textBlockIndex,
			"content_block": map[string]any{"type": "text"},
		})
		textBlockOpen = false
		textBlockIndex = -1
	}

	closeToolBlocks := func() {
		for _, idx := range toolCallOrder {
			acc := toolCallAccumulator[idx]
			emitClaudeEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": toolBlockIndexes[idx],
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    acc["id"],
					"name":  acc["name"],
					"input": map[string]any{},
				},
			})
		}
	}

	emitMessageTerminal := func(finishReason string) {
		stopReason := "end_turn"
		switch finishReason {
		case "length":
			stopReason = "max_tokens"
		case "tool_calls", "function_call":
			stopReason = "tool_use"
		case "content_filter":
			stopReason = "refusal"
		}
		outputTokens, _ := GetFloat(fullUsage, "completion_tokens", "output_tokens")
		if upstreamRequests := AnthropicWebSearchRequestsFromUsage(fullUsage); upstreamRequests > webSearchRequests {
			webSearchRequests = upstreamRequests
		}
		terminalUsage := map[string]any{"output_tokens": int64(outputTokens)}
		terminalUsage = WithAnthropicWebSearchUsage(terminalUsage, webSearchRequests)
		emitClaudeEvent("message_delta", map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason": stopReason,
			},
			"usage": terminalUsage,
		})
		emitClaudeEvent("message_stop", map[string]any{"type": "message_stop"})
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line == "" {
					break
				}
				err = nil
			} else {
				// 客户端取消请求（例如 Codex 中断当前轮次）会令上游流读取返回
				// context.Canceled；这不是上游故障，直接返回，避免噪声日志。
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				log.Printf("读取流失败：%v", err)
				emitClaudeError(map[string]any{"message": err.Error()}, "failed to read upstream stream")
				return
			}
		}
		payload, isDataLine := SseDataPayload(line)
		if debugMode && debugLogBodies && isDataLine {
			log.Printf("[上游原始数据块] %s", strings.TrimSpace(payload))
		}

		trimmed := strings.TrimSpace(line)
		if (isDataLine && payload == "[DONE]") || trimmed == "[DONE]" {
			if !terminalFinishSeen {
				emitClaudeError(nil, "upstream stream ended before finish_reason")
				return
			}
			break
		}
		if !isDataLine {
			continue
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		chunkType, _ := chunk["type"].(string)
		if chunk["error"] != nil || chunkType == "error" || chunkType == "response.failed" {
			if usage, ok := chunk["usage"].(map[string]any); ok {
				fullUsage = usage
			}
			emitClaudeError(chunk, "upstream stream failed")
			return
		}

		choices, ok := chunk["choices"].([]any)
		if !ok || len(choices) == 0 {
			if usage, ok := chunk["usage"].(map[string]any); ok {
				fullUsage = usage
			}
			continue
		}

		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		NormalizeReasoningContent(delta)
		providerOutput, _ := delta["provider_output"].([]any)
		annotations, _ := delta["annotations"].([]any)
		if requests := ChatWebSearchEvidenceCount(providerOutput, annotations); requests > webSearchRequests {
			webSearchRequests = requests
		}
		finishReason, _ := choice["finish_reason"].(string)

		if !messageStartSent {
			messageStartSent = true
			emitClaudeEvent("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":          msgID,
					"type":        "message",
					"role":        "assistant",
					"content":     []any{},
					"model":       model,
					"stop_reason": nil,
					"usage":       map[string]any{"input_tokens": 0, "output_tokens": 0},
				},
			})
			emitClaudeEvent("ping", map[string]any{"type": "ping"})
		}

		if rc, ok := delta["reasoning_content"]; ok {
			rcStr, _ := rc.(string)
			if rcStr != "" {
				closeTextBlock()
				if !thinkingBlockOpen {
					thinkingBlockIndex = blockIndex
					emitClaudeEvent("content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": thinkingBlockIndex,
						"content_block": map[string]any{
							"type":     "thinking",
							"thinking": "",
						},
					})
					thinkingBlockOpen = true
					blockIndex++
				}
				emitClaudeEvent("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": thinkingBlockIndex,
					"delta": map[string]any{
						"type":     "thinking_delta",
						"thinking": rcStr,
					},
				})
			}
		}

		if signature, ok := delta["reasoning_signature"].(string); ok && signature != "" {
			closeTextBlock()
			if !thinkingBlockOpen {
				thinkingBlockIndex = blockIndex
				emitClaudeEvent("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": thinkingBlockIndex,
					"content_block": map[string]any{
						"type":     "thinking",
						"thinking": "",
					},
				})
				thinkingBlockOpen = true
				blockIndex++
			}
			emitClaudeEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": thinkingBlockIndex,
				"delta": map[string]any{
					"type":      "signature_delta",
					"signature": signature,
				},
			})
		}

		if c, ok := delta["content"]; ok && c != nil {
			contentStr, _ := c.(string)
			if contentStr != "" {
				closeThinkingBlock()
				if !textBlockOpen {
					textBlockIndex = blockIndex
					emitClaudeEvent("content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": textBlockIndex,
						"content_block": map[string]any{
							"type": "text",
							"text": "",
						},
					})
					textBlockOpen = true
					blockIndex++
				}
				emitClaudeEvent("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": textBlockIndex,
					"delta": map[string]any{
						"type": "text_delta",
						"text": contentStr,
					},
				})
			}
		}

		if rawToolCalls, ok := delta["tool_calls"].([]any); ok {
			for _, rawTC := range rawToolCalls {
				tc, ok := rawTC.(map[string]any)
				if !ok {
					continue
				}
				idxFloat, _ := tc["index"].(float64)
				upstreamIndex := int(idxFloat)

				closeThinkingBlock()
				closeTextBlock()

				if _, exists := toolCallAccumulator[upstreamIndex]; !exists {
					callID, _ := tc["id"].(string)
					if callID == "" {
						callID = "toolu_" + RandomString(12)
					}
					fn, _ := tc["function"].(map[string]any)
					name, _ := fn["name"].(string)
					toolCallAccumulator[upstreamIndex] = map[string]string{
						"id":   callID,
						"name": name,
						"args": "",
					}
					toolBlockIndexes[upstreamIndex] = blockIndex
					toolCallOrder = append(toolCallOrder, upstreamIndex)
					emitClaudeEvent("content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": toolBlockIndexes[upstreamIndex],
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    callID,
							"name":  name,
							"input": map[string]any{},
						},
					})
					blockIndex++
				}

				fn, _ := tc["function"].(map[string]any)
				if argDelta, ok := fn["arguments"].(string); ok && argDelta != "" {
					toolCallAccumulator[upstreamIndex]["args"] += argDelta
					emitClaudeEvent("content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": toolBlockIndexes[upstreamIndex],
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": argDelta,
						},
					})
				}
			}
		}

		if usage, ok := chunk["usage"].(map[string]any); ok {
			fullUsage = usage
		}

		if !terminalFinishSeen && (finishReason == "stop" || finishReason == "length" || finishReason == "tool_calls" || finishReason == "function_call" || finishReason == "content_filter") {
			terminalFinishSeen = true
			pendingFinishReason = finishReason
		}
	}

	var toolArgumentsErr error
	if terminalFinishSeen {
		for _, idx := range toolCallOrder {
			acc := toolCallAccumulator[idx]
			if _, err := NormalizeToolCallArguments(acc["args"]); err != nil {
				LogStreamToolCallArgumentsValidationFailure("ClaudeStreamHandler", "", acc["id"], acc["name"], acc["args"], idx, err)
				toolArgumentsErr = fmt.Errorf("tool call %q (%s) ended with invalid arguments: %w", acc["id"], acc["name"], err)
				break
			}
		}
	}
	closeThinkingBlock()
	closeTextBlock()
	closeToolBlocks()
	if toolArgumentsErr != nil {
		emitClaudeError(map[string]any{"message": toolArgumentsErr.Error(), "type": "upstream_protocol_error"}, "upstream tool call arguments were invalid")
		return
	}
	if terminalFinishSeen {
		emitMessageTerminal(pendingFinishReason)
		return
	}
	emitClaudeError(nil, "upstream stream ended before finish_reason")
}

// ======================== Anthropic 流式转换 ========================

// pipeResponseWriter 适配 io.Writer 到 http.ResponseWriter 接口
