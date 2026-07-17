package stream

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type pipeResponseWriter struct {
	w      io.Writer
	header http.Header
}

func (p *pipeResponseWriter) Header() http.Header {
	if p.header == nil {
		p.header = make(http.Header)
	}
	return p.header
}

func (p *pipeResponseWriter) Write(data []byte) (int, error) {
	return p.w.Write(data)
}

func (p *pipeResponseWriter) WriteHeader(code int) {}

func (p *pipeResponseWriter) Flush() {
	// 管道无需执行操作；写入是同步的。
}

// AnthropicStreamToChatHandler 将上游 Anthropic SSE 流实时转为 OpenAI Chat SSE 格式并写入客户端。
// recordUsage 是显式统计标志；链式转换器传入 false，确保只有最终对外处理器
// 对一次请求统计一次。
func AnthropicStreamToChatHandler(w http.ResponseWriter, respBody io.ReadCloser, model, usageModel string, recordUsage bool) {
	defer respBody.Close()
	var usageStats *requestUsageAccumulator
	if recordUsage {
		usageStats = NewRequestUsageAccumulator(usageModel)
		defer usageStats.commit()
	}
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(respBody)

	chunkID := "chatcmpl-" + RandomString(16)
	created := time.Now().Unix()
	roleSent := false
	toolCallAccumulator := map[int]map[string]string{}
	toolCallOrder := []int{}
	anthropicBlockToToolIndex := map[int]int{}
	fullUsage := map[string]any{}
	terminalSeen := false
	streamFailed := false

	defer func() {
		if usageStats != nil {
			usageStats.observeMap(fullUsage)
		}
	}()

	emitChatChunk := func(delta map[string]any, finishReason any, usage map[string]any) {
		// 清理空 content，避免客户端收到 content:"" 的 chunk
		if c, ok := delta["content"].(string); ok && c == "" {
			delete(delta, "content")
		}
		if finishReason == nil || finishReason == "" {
			finishReason = nil
		}
		chunk := map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": finishReason,
				},
			},
		}
		if usage != nil {
			chunk["usage"] = usage
		}
		jsonData, _ := json.Marshal(chunk)
		w.Write([]byte("data: " + string(jsonData) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
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
				log.Printf("读取 Anthropic 流失败：%v", err)
				EmitOpenAIStreamError(w, flusher, map[string]any{"message": err.Error(), "usage": fullUsage}, "failed to read upstream Anthropic stream")
				streamFailed = true
				return
			}
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		payload, isDataLine := SseDataPayload(line)
		if (isDataLine && payload == "[DONE]") || trimmed == "[DONE]" {
			if !terminalSeen {
				EmitOpenAIStreamError(w, flusher, nil, "upstream Anthropic stream ended before a terminal event")
				streamFailed = true
			}
			break
		}

		// 解析 Anthropic SSE 数据行。
		if !isDataLine {
			continue
		}

		var event map[string]any
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		if event["error"] != nil && (eventType == "" || eventType == "error") {
			EmitOpenAIStreamError(w, flusher, event, "upstream Anthropic stream failed")
			return
		}

		switch eventType {
		case "error":
			EmitOpenAIStreamError(w, flusher, event, "upstream Anthropic stream failed")
			streamFailed = true
			return

		case "message_start":
			if msg, ok := event["message"].(map[string]any); ok {
				if id, ok := msg["id"].(string); ok && id != "" {
					chunkID = "chatcmpl-" + id
				}
				if u, ok := msg["usage"].(map[string]any); ok {
					fullUsage = u
				}
			}
			if !roleSent {
				emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
				roleSent = true
			}

		case "content_block_start":
			block, _ := event["content_block"].(map[string]any)
			if block != nil {
				blockType, _ := block["type"].(string)
				switch blockType {
				case "tool_use":
					idx := len(toolCallOrder)
					anthropicIndex, hasAnthropicIndex := GetFloat(event, "index")
					if !hasAnthropicIndex {
						anthropicIndex = float64(idx)
					}
					anthropicBlockToToolIndex[int(anthropicIndex)] = idx
					callID, _ := block["id"].(string)
					name, _ := block["name"].(string)
					toolCallAccumulator[idx] = map[string]string{
						"id":   callID,
						"name": name,
						"args": "",
					}
					toolCallOrder = append(toolCallOrder, idx)
					if !roleSent {
						emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
						roleSent = true
					}
					delta := map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": float64(idx),
								"id":    callID,
								"type":  "function",
								"function": map[string]any{
									"name":      name,
									"arguments": "",
								},
							},
						},
					}
					emitChatChunk(delta, nil, nil)
				}
			}

		case "content_block_delta":
			deltaObj, _ := event["delta"].(map[string]any)
			if deltaObj == nil {
				continue
			}
			deltaType, _ := deltaObj["type"].(string)
			switch deltaType {
			case "thinking_delta":
				thinking, _ := deltaObj["thinking"].(string)
				if thinking != "" {
					if !roleSent {
						emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
						roleSent = true
					}
					emitChatChunk(map[string]any{"reasoning_content": thinking}, nil, nil)
				}
			case "signature_delta":
				signature, _ := deltaObj["signature"].(string)
				if signature != "" {
					if !roleSent {
						emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
						roleSent = true
					}
					emitChatChunk(map[string]any{"reasoning_signature": signature}, nil, nil)
				}
			case "text_delta":
				text, _ := deltaObj["text"].(string)
				if text != "" {
					if !roleSent {
						emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
						roleSent = true
					}
					emitChatChunk(map[string]any{"content": text}, nil, nil)
				}
			case "input_json_delta":
				partialJSON, _ := deltaObj["partial_json"].(string)
				index, _ := event["index"].(float64)
				idx, mapped := anthropicBlockToToolIndex[int(index)]
				if !mapped {
					log.Printf("警告：Anthropic 工具增量引用了未知的内容块索引 %d", int(index))
					continue
				}
				if tc, ok := toolCallAccumulator[idx]; ok {
					tc["args"] += partialJSON
					delta := map[string]any{
						"tool_calls": []map[string]any{
							{
								"index":    float64(idx),
								"function": map[string]any{"arguments": partialJSON},
							},
						},
					}
					emitChatChunk(delta, nil, nil)
				}
			}

		case "message_delta":
			deltaObj, _ := event["delta"].(map[string]any)
			if deltaObj != nil {
				stopReason, _ := deltaObj["stop_reason"].(string)
				finishReason := ""
				switch stopReason {
				case "end_turn", "stop_sequence", "pause_turn":
					finishReason = "stop"
				case "max_tokens", "model_context_window_exceeded":
					finishReason = "length"
				case "tool_use":
					finishReason = "tool_calls"
				case "refusal":
					finishReason = "content_filter"
				default:
					if stopReason != "" {
						finishReason = "stop"
					}
				}
				usage, _ := event["usage"].(map[string]any)
				if usage != nil {
					if ot, ok := usage["output_tokens"].(float64); ok {
						fullUsage["output_tokens"] = ot
					}
				}
				chatUsage := map[string]any{}
				if pt, ok := fullUsage["input_tokens"].(float64); ok {
					chatUsage["prompt_tokens"] = int64(pt)
				}
				if ot, ok := fullUsage["output_tokens"].(float64); ok {
					chatUsage["completion_tokens"] = int64(ot)
				}
				if _, ok := chatUsage["prompt_tokens"]; !ok {
					if u, ok2 := event["usage"].(map[string]any); ok2 {
						if it, ok3 := u["input_tokens"].(float64); ok3 {
							chatUsage["prompt_tokens"] = int64(it)
							fullUsage["input_tokens"] = it
						}
					}
				}
				if _, ok := chatUsage["completion_tokens"]; !ok {
					if u, ok2 := event["usage"].(map[string]any); ok2 {
						if ot, ok3 := u["output_tokens"].(float64); ok3 {
							chatUsage["completion_tokens"] = int64(ot)
							fullUsage["output_tokens"] = ot
						}
					}
				}
				pt := float64(0)
				if v, ok := chatUsage["prompt_tokens"].(int64); ok {
					pt = float64(v)
				}
				ct := float64(0)
				if v, ok := chatUsage["completion_tokens"].(int64); ok {
					ct = float64(v)
				}
				chatUsage["total_tokens"] = int64(pt + ct)
				if !roleSent {
					emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
					roleSent = true
				}
				emitChatChunk(map[string]any{}, finishReason, chatUsage)
				if finishReason != "" {
					terminalSeen = true
				}
			}

		case "message_stop":
			if !terminalSeen {
				if !roleSent {
					emitChatChunk(map[string]any{"role": "assistant", "content": ""}, nil, nil)
					roleSent = true
				}
				emitChatChunk(map[string]any{}, "stop", nil)
			}
			terminalSeen = true
		case "ping":
			// 忽略。
		}
	}

	if !terminalSeen && !streamFailed {
		EmitOpenAIStreamError(w, flusher, nil, "upstream Anthropic stream ended before a terminal event")
		streamFailed = true
	}
	if !streamFailed {
		w.Write([]byte("data: [DONE]\n\n"))
	}
	if flusher != nil {
		flusher.Flush()
	}
}
