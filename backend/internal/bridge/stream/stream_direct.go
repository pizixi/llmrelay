package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type directStreamBlock struct {
	kind              string
	outputIndex       int
	contentIndex      int
	itemID            string
	callID            string
	name              string
	text              string
	arguments         string
	argumentDeltaSent bool
	signature         string
	encryptedContent  string
	started           bool
	done              bool
}

func AnthropicUsageToResponsesUsage(usage map[string]any) map[string]any {
	input, _ := GetFloat(usage, "input_tokens", "prompt_tokens")
	output, _ := GetFloat(usage, "output_tokens", "completion_tokens")
	cached, _ := GetFloat(usage, "cache_read_input_tokens", "cached_tokens")
	return map[string]any{
		"input_tokens":  int64(input),
		"output_tokens": int64(output),
		"total_tokens":  int64(input + output),
		"input_tokens_details": map[string]any{
			"cached_tokens": int64(cached),
		},
	}
}

func ResponsesIncompleteToAnthropicStopReason(response map[string]any) string {
	details, _ := response["incomplete_details"].(map[string]any)
	reason, _ := details["reason"].(string)
	switch reason {
	case "max_output_tokens", "max_tokens":
		return "max_tokens"
	case "model_context_window_exceeded":
		return "model_context_window_exceeded"
	case "content_filter", "refusal":
		return "refusal"
	default:
		// Anthropic 没有通用的 "incomplete" 停止原因。
		// max_tokens 是误导性最小的可移植降级值，它会告知客户端助手轮次被截断，
		// 而不是已成功完成。
		return "max_tokens"
	}
}

// AnthropicStreamToResponsesDirectHandler 将 Anthropic 内容块事件直接映射为
// Responses 项目事件，刻意不生成中间 Chat Completion 流。
func AnthropicStreamToResponsesDirectHandler(
	w http.ResponseWriter,
	respBody io.ReadCloser,
	model, usageModel string,
	bridgeMode BridgeMode,
	tools, toolChoice any,
	parallelToolCalls *bool,
	toolNameMappings map[string]ResponseToolNameMapping,
	responseEcho map[string]any,
	warningGroups ...[]BridgeWarning,
) {
	defer respBody.Close()
	usageStats := NewRequestUsageAccumulator(usageModel)
	defer usageStats.commit()
	var bridgeWarnings []BridgeWarning
	bridgeWarnings = AppendBridgeWarnings(bridgeWarnings, warningGroups...)

	SetSSEHeaders(w.Header())
	WriteBridgeWarningHeaders(w.Header(), bridgeWarnings)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(respBody)

	responseID := "resp_" + RandomString(24)
	createdAt := time.Now().Unix()
	sequence := 0
	created := false
	terminal := false
	failed := false
	incompleteReason := ""
	totalUsage := map[string]any{}
	blocks := map[int]*directStreamBlock{}
	output := map[int]any{}
	nextOutputIndex := 0

	emit := func(name string, payload map[string]any) {
		sequence++
		payload["sequence_number"] = sequence
		EmitSSEEvent(w, flusher, name, payload)
	}
	responseObject := func(status string, responseError, incomplete any) map[string]any {
		ordered := make([]any, 0, len(output))
		for index := 0; index < nextOutputIndex; index++ {
			if item, ok := output[index]; ok {
				ordered = append(ordered, item)
			}
		}
		result := map[string]any{
			"id": responseID, "object": "response", "created_at": createdAt,
			"status": status, "background": false, "error": responseError,
			"incomplete_details": incomplete, "model": model, "output": ordered,
			"tools": []any{}, "tool_choice": "auto", "parallel_tool_calls": true,
			"usage": AnthropicUsageToResponsesUsage(totalUsage),
		}
		if normalizedTools, ok := ResponsesToolsForOutput(tools); ok {
			result["tools"] = normalizedTools
		}
		if toolChoice != nil {
			result["tool_choice"] = toolChoice
		}
		if parallelToolCalls != nil {
			result["parallel_tool_calls"] = *parallelToolCalls
		}
		ApplyResponsesRequestEcho(result, responseEcho)
		return result
	}
	emitCreated := func() {
		if created {
			return
		}
		base := responseObject("in_progress", nil, nil)
		emit("response.created", map[string]any{"type": "response.created", "response": base})
		emit("response.in_progress", map[string]any{"type": "response.in_progress", "response": base})
		created = true
	}
	failUnsupported := func(message string) {
		emitCreated()
		errorObject := map[string]any{"type": "upstream_protocol_error", "message": message}
		emit("error", map[string]any{"type": "error", "error": errorObject})
		emit("response.failed", map[string]any{
			"type": "response.failed", "response": responseObject("failed", errorObject, nil),
		})
		failed = true
	}
	blockItem := func(block *directStreamBlock, status string) map[string]any {
		switch block.kind {
		case "reasoning":
			item := map[string]any{
				"id": block.itemID, "type": "reasoning", "status": status,
				"summary": []any{map[string]any{"type": "summary_text", "text": block.text}},
			}
			if block.signature != "" {
				// Anthropic 签名用于验证 thinking 块，并非 OpenAI 加密推理令牌。
				// 因此将其保存在带命名空间的桥接载体中，以便无损重放回 Anthropic。
				item["anthropic_signature"] = block.signature
			}
			return item
		case "redacted_reasoning":
			item := map[string]any{
				"id": block.itemID, "type": "reasoning", "status": status,
				"summary": []any{},
			}
			if block.encryptedContent != "" {
				item["encrypted_content"] = block.encryptedContent
			}
			return item
		case "function_call":
			return ResponseFunctionCallItem(
				block.itemID, status, block.arguments, block.callID, block.name, toolNameMappings,
			)
		default:
			return map[string]any{
				"id": block.itemID, "type": "message", "status": status, "role": "assistant",
				"content": []any{map[string]any{
					"type": "output_text", "text": block.text,
					"annotations": []any{}, "logprobs": []any{},
				}},
			}
		}
	}
	startBlock := func(sourceIndex int, native map[string]any) *directStreamBlock {
		if existing := blocks[sourceIndex]; existing != nil {
			return existing
		}
		emitCreated()
		nativeType, _ := native["type"].(string)
		block := &directStreamBlock{contentIndex: 0}
		switch nativeType {
		case "thinking":
			block.kind = "reasoning"
			block.outputIndex = nextOutputIndex
			nextOutputIndex++
			block.itemID = fmt.Sprintf("rs_%s_%d", responseID, sourceIndex)
			block.text, _ = native["thinking"].(string)
			block.signature, _ = native["signature"].(string)
			block.started = true
			emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": block.outputIndex,
				"item": blockItem(block, "in_progress"),
			})
			emit("response.reasoning_summary_part.added", map[string]any{
				"type": "response.reasoning_summary_part.added", "item_id": block.itemID,
				"output_index": block.outputIndex, "summary_index": 0,
				"part": map[string]any{"type": "summary_text", "text": ""},
			})
		case "redacted_thinking":
			block.kind = "redacted_reasoning"
			block.outputIndex = nextOutputIndex
			nextOutputIndex++
			block.itemID = fmt.Sprintf("rs_%s_%d", responseID, sourceIndex)
			block.encryptedContent, _ = native["data"].(string)
			block.started = true
			emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": block.outputIndex,
				"item": blockItem(block, "in_progress"),
			})
		case "tool_use", "server_tool_use":
			block.kind = "function_call"
			block.outputIndex = nextOutputIndex
			nextOutputIndex++
			block.callID, _ = native["id"].(string)
			if block.callID == "" {
				block.callID = "call_" + RandomString(16)
			}
			block.name, _ = native["name"].(string)
			block.itemID = ResponseToolCallItemID(block.callID, block.name, toolNameMappings)
			if input, exists := native["input"]; exists {
				encoded, _ := json.Marshal(input)
				if string(encoded) != "{}" && string(encoded) != "null" {
					block.arguments = string(encoded)
				}
			}
			block.started = true
			emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": block.outputIndex,
				"item": blockItem(block, "in_progress"),
			})
		case "text":
			block.kind = "message"
			block.outputIndex = nextOutputIndex
			nextOutputIndex++
			block.itemID = fmt.Sprintf("msg_%s_%d", responseID, sourceIndex)
			block.text, _ = native["text"].(string)
			block.started = true
			emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": block.outputIndex,
				"item": map[string]any{"id": block.itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}},
			})
			emit("response.content_part.added", map[string]any{
				"type": "response.content_part.added", "item_id": block.itemID,
				"output_index": block.outputIndex, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}},
			})
		default:
			block.kind = "unsupported"
			log.Printf("[协议桥接警告] Anthropic->Responses 不支持内容块类型=%q 索引=%d；已跳过", nativeType, sourceIndex)
			if bridgeMode == BridgeModeStrict {
				failUnsupported(fmt.Sprintf("unsupported Anthropic content block type %q", nativeType))
			} else {
				incompleteReason = "unsupported_content_block"
			}
		}
		blocks[sourceIndex] = block
		return block
	}
	finishBlock := func(block *directStreamBlock) {
		if block == nil || block.done || !block.started {
			return
		}
		switch block.kind {
		case "reasoning":
			emit("response.reasoning_summary_text.done", map[string]any{
				"type": "response.reasoning_summary_text.done", "item_id": block.itemID,
				"output_index": block.outputIndex, "summary_index": 0, "text": block.text,
			})
			emit("response.reasoning_summary_part.done", map[string]any{
				"type": "response.reasoning_summary_part.done", "item_id": block.itemID,
				"output_index": block.outputIndex, "summary_index": 0,
				"part": map[string]any{"type": "summary_text", "text": block.text},
			})
		case "function_call":
			if normalized, err := NormalizeToolCallArguments(block.arguments); err == nil {
				block.arguments = normalized
			} else {
				incompleteReason = "tool_call_arguments_incomplete"
			}
			emit("response.function_call_arguments.done", map[string]any{
				"type": "response.function_call_arguments.done", "item_id": block.itemID,
				"output_index": block.outputIndex, "arguments": block.arguments,
			})
		case "redacted_reasoning":
			// encrypted_content 由推理输出项目自身携带，
			// 无需合成可见的摘要增量。
		default:
			emit("response.output_text.done", map[string]any{
				"type": "response.output_text.done", "item_id": block.itemID,
				"output_index": block.outputIndex, "content_index": 0,
				"text": block.text, "logprobs": []any{},
			})
			emit("response.content_part.done", map[string]any{
				"type": "response.content_part.done", "item_id": block.itemID,
				"output_index": block.outputIndex, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": block.text, "annotations": []any{}, "logprobs": []any{}},
			})
		}
		output[block.outputIndex] = blockItem(block, "completed")
		emit("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "output_index": block.outputIndex,
			"item": output[block.outputIndex],
		})
		block.done = true
	}

	currentEvent := ""
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if payload, ok := SseDataPayload(line); ok && payload != "" {
				var event map[string]any
				if json.Unmarshal([]byte(payload), &event) == nil {
					eventType, _ := event["type"].(string)
					if eventType == "" {
						eventType = currentEvent
					}
					switch eventType {
					case "message_start":
						if message, ok := event["message"].(map[string]any); ok {
							if id, _ := message["id"].(string); id != "" {
								responseID = "resp_" + strings.TrimPrefix(id, "msg_")
							}
							if usage, ok := message["usage"].(map[string]any); ok {
								totalUsage = usage
							}
						}
						emitCreated()
					case "content_block_start":
						index, _ := GetFloat(event, "index")
						if native, ok := event["content_block"].(map[string]any); ok {
							startBlock(int(index), native)
						}
					case "content_block_delta":
						index, _ := GetFloat(event, "index")
						delta, _ := event["delta"].(map[string]any)
						deltaType, _ := delta["type"].(string)
						block := blocks[int(index)]
						if block == nil {
							fallbackType := ""
							switch deltaType {
							case "thinking_delta", "signature_delta":
								fallbackType = "thinking"
							case "input_json_delta":
								fallbackType = "tool_use"
							case "text_delta":
								fallbackType = "text"
							}
							block = startBlock(int(index), map[string]any{"type": fallbackType})
						}
						if block == nil || !block.started {
							log.Printf("[协议桥接警告] Anthropic->Responses 忽略不支持内容块的增量 类型=%q 索引=%d", deltaType, int(index))
							if bridgeMode == BridgeModeStrict && !failed {
								failUnsupported(fmt.Sprintf("unsupported Anthropic content delta type %q", deltaType))
							}
							continue
						}
						switch deltaType {
						case "thinking_delta":
							text, _ := delta["thinking"].(string)
							block.text += text
							emit("response.reasoning_summary_text.delta", map[string]any{
								"type": "response.reasoning_summary_text.delta", "item_id": block.itemID,
								"output_index": block.outputIndex, "summary_index": 0, "delta": text,
							})
						case "signature_delta":
							signature, _ := delta["signature"].(string)
							block.signature += signature
						case "input_json_delta":
							fragment, _ := delta["partial_json"].(string)
							block.arguments += fragment
							emit("response.function_call_arguments.delta", map[string]any{
								"type": "response.function_call_arguments.delta", "item_id": block.itemID,
								"output_index": block.outputIndex, "delta": fragment,
							})
						case "text_delta":
							text, _ := delta["text"].(string)
							block.text += text
							emit("response.output_text.delta", map[string]any{
								"type": "response.output_text.delta", "item_id": block.itemID,
								"output_index": block.outputIndex, "content_index": 0,
								"delta": text, "logprobs": []any{},
							})
						default:
							log.Printf("[协议桥接警告] Anthropic->Responses 不支持内容增量类型=%q 索引=%d；已跳过", deltaType, int(index))
							if bridgeMode == BridgeModeStrict {
								failUnsupported(fmt.Sprintf("unsupported Anthropic content delta type %q", deltaType))
							} else {
								incompleteReason = "unsupported_content_delta"
							}
						}
					case "content_block_stop":
						index, _ := GetFloat(event, "index")
						finishBlock(blocks[int(index)])
					case "message_delta":
						if usage, ok := event["usage"].(map[string]any); ok {
							for key, value := range usage {
								totalUsage[key] = value
							}
						}
						if delta, ok := event["delta"].(map[string]any); ok {
							reason, _ := delta["stop_reason"].(string)
							if reason == "max_tokens" || reason == "model_context_window_exceeded" {
								incompleteReason = "max_output_tokens"
							}
						}
					case "message_stop":
						terminal = true
					case "error":
						emitCreated()
						errorObject := StreamErrorObject(event, "upstream Anthropic stream failed")
						emit("error", map[string]any{"type": "error", "error": errorObject})
						emit("response.failed", map[string]any{
							"type": "response.failed", "response": responseObject("failed", errorObject, nil),
						})
						failed = true
					}
				}
			}
		}
		if failed || terminal {
			break
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("读取 Anthropic->Responses 直连流失败：%v", err)
			}
			break
		}
	}
	if failed {
		return
	}
	emitCreated()
	sourceIndexes := make([]int, 0, len(blocks))
	for sourceIndex := range blocks {
		sourceIndexes = append(sourceIndexes, sourceIndex)
	}
	sort.Ints(sourceIndexes)
	for _, sourceIndex := range sourceIndexes {
		finishBlock(blocks[sourceIndex])
	}
	usageStats.observeMap(totalUsage)
	status := "completed"
	var incomplete any
	if !terminal || incompleteReason != "" {
		status = "incomplete"
		if incompleteReason == "" {
			incompleteReason = "stream_ended_early"
		}
		incomplete = map[string]any{"reason": incompleteReason}
	}
	emit("response."+status, map[string]any{
		"type":     "response." + status,
		"response": responseObject(status, nil, incomplete),
	})
}

// ResponsesStreamToAnthropicDirectHandler 将 Responses 项目事件直接映射为
// Anthropic 内容块事件，无需经 Chat Completion 中转，并保留流错误和最终未完成状态。
func ResponsesStreamToAnthropicDirectHandler(w http.ResponseWriter, respBody io.ReadCloser, model, usageModel string) {
	defer respBody.Close()
	usageStats := NewRequestUsageAccumulator(usageModel)
	defer usageStats.commit()
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(respBody)

	messageID := "msg_" + RandomString(24)
	messageStarted := false
	terminal := false
	failed := false
	nextBlockIndex := 0
	totalUsage := map[string]any{}
	blocksByItem := map[string]*directStreamBlock{}
	blocksByOutput := map[int]*directStreamBlock{}
	hasToolUse := false
	webSearchRequests := 0
	completedWebSearchCalls := map[string]bool{}

	emit := func(name string, data map[string]any) {
		EmitSSEEvent(w, flusher, name, data)
	}
	emitMessageStart := func() {
		if messageStarted {
			return
		}
		input, _ := GetFloat(totalUsage, "input_tokens", "prompt_tokens")
		emit("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": messageID, "type": "message", "role": "assistant",
				"content": []any{}, "model": model, "stop_reason": nil,
				"stop_sequence": nil,
				"usage":         map[string]any{"input_tokens": int64(input), "output_tokens": 0},
			},
		})
		messageStarted = true
	}
	startBlock := func(itemID, kind, callID, name string, outputIndex int, carrier ...string) *directStreamBlock {
		block := blocksByItem[itemID]
		if block != nil && block.started {
			return block
		}
		emitMessageStart()
		if block == nil {
			block = &directStreamBlock{itemID: itemID, outputIndex: outputIndex}
		}
		block.kind = kind
		block.callID = callID
		block.name = name
		block.contentIndex = nextBlockIndex
		block.started = true
		if len(carrier) > 0 {
			if kind == "redacted_reasoning" {
				block.encryptedContent = carrier[0]
			} else if kind == "reasoning" {
				block.signature = carrier[0]
			}
		}
		nextBlockIndex++
		var native map[string]any
		switch kind {
		case "reasoning":
			native = map[string]any{"type": "thinking", "thinking": "", "signature": ""}
		case "redacted_reasoning":
			native = map[string]any{"type": "redacted_thinking", "data": block.encryptedContent}
		case "function_call":
			hasToolUse = true
			if callID == "" {
				callID = itemID
				block.callID = callID
			}
			native = map[string]any{"type": "tool_use", "id": callID, "name": name, "input": map[string]any{}}
		default:
			native = map[string]any{"type": "text", "text": ""}
		}
		emit("content_block_start", map[string]any{
			"type": "content_block_start", "index": block.contentIndex, "content_block": native,
		})
		blocksByItem[itemID] = block
		blocksByOutput[outputIndex] = block
		return block
	}
	finishBlock := func(block *directStreamBlock) {
		if block == nil || block.done || !block.started {
			return
		}
		if block.kind == "reasoning" && block.signature != "" {
			emit("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": block.contentIndex,
				"delta": map[string]any{"type": "signature_delta", "signature": block.signature},
			})
		}
		if block.kind == "function_call" && block.arguments != "" && !block.argumentDeltaSent {
			emit("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": block.contentIndex,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": block.arguments},
			})
			block.arguments = ""
		}
		emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": block.contentIndex})
		block.done = true
	}
	registerPendingReasoning := func(itemID string, outputIndex int) *directStreamBlock {
		if existing := blocksByItem[itemID]; existing != nil {
			return existing
		}
		block := &directStreamBlock{kind: "pending_reasoning", itemID: itemID, outputIndex: outputIndex}
		blocksByItem[itemID] = block
		blocksByOutput[outputIndex] = block
		return block
	}
	emitWebSearchCall := func(item map[string]any, outputIndex int) *directStreamBlock {
		itemID, _ := item["id"].(string)
		callKey := itemID
		if callKey == "" {
			callKey = fmt.Sprintf("output:%d", outputIndex)
		}
		if !completedWebSearchCalls[callKey] {
			completedWebSearchCalls[callKey] = true
			webSearchRequests++
		}
		emitMessageStart()
		block := blocksByItem[itemID]
		if block == nil {
			block = &directStreamBlock{itemID: itemID, outputIndex: outputIndex}
		}
		block.kind = "web_search_call"
		block.contentIndex = nextBlockIndex
		block.started = true
		for _, raw := range ResponsesWebSearchToAnthropicBlocks(item) {
			native, _ := raw.(map[string]any)
			start := native
			var inputJSON []byte
			if BridgeString(native["type"]) == "server_tool_use" {
				start = CloneAnyMap(native)
				inputJSON, _ = json.Marshal(start["input"])
				start["input"] = map[string]any{}
			}
			emit("content_block_start", map[string]any{
				"type": "content_block_start", "index": nextBlockIndex, "content_block": start,
			})
			if len(inputJSON) > 0 && string(inputJSON) != "{}" {
				emit("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": nextBlockIndex,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": string(inputJSON)},
				})
			}
			emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": nextBlockIndex})
			nextBlockIndex++
		}
		block.done = true
		blocksByItem[itemID] = block
		blocksByOutput[outputIndex] = block
		return block
	}
	terminalMessage := func(stopReason string) bool {
		orderedBlocks := make([]*directStreamBlock, 0, len(blocksByItem))
		for _, block := range blocksByItem {
			if block != nil && !block.started {
				return false
			}
			orderedBlocks = append(orderedBlocks, block)
		}
		sort.Slice(orderedBlocks, func(i, j int) bool {
			return orderedBlocks[i].contentIndex < orderedBlocks[j].contentIndex
		})
		for _, block := range orderedBlocks {
			finishBlock(block)
		}
		outputTokens, _ := GetFloat(totalUsage, "output_tokens", "completion_tokens")
		terminalUsage := map[string]any{"output_tokens": int64(outputTokens)}
		terminalUsage = WithAnthropicWebSearchUsage(terminalUsage, webSearchRequests)
		emit("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
			"usage": terminalUsage,
		})
		emit("message_stop", map[string]any{"type": "message_stop"})
		return true
	}
	emitError := func(payload map[string]any, fallback string) {
		errorObject := StreamErrorObject(payload, fallback)
		emit("error", map[string]any{
			"type":  "error",
			"error": map[string]any{"type": errorObject["type"], "message": errorObject["message"]},
		})
	}

	currentEvent := ""
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if payload, ok := SseDataPayload(line); ok && payload != "" && payload != "[DONE]" {
				var event map[string]any
				if json.Unmarshal([]byte(payload), &event) == nil {
					eventType, _ := event["type"].(string)
					if eventType == "" {
						eventType = currentEvent
					}
					switch eventType {
					case "response.created", "response.in_progress":
						if response, ok := event["response"].(map[string]any); ok {
							if id, _ := response["id"].(string); id != "" {
								messageID = "msg_" + strings.TrimPrefix(id, "resp_")
							}
							if usage, ok := response["usage"].(map[string]any); ok {
								totalUsage = usage
							}
						}
						emitMessageStart()
					case "response.output_item.added":
						item, _ := event["item"].(map[string]any)
						itemID, _ := item["id"].(string)
						kind, _ := item["type"].(string)
						index, _ := GetFloat(event, "output_index")
						if kind == "function_call" || kind == "custom_tool_call" {
							callID, _ := item["call_id"].(string)
							name, _ := item["name"].(string)
							block := startBlock(itemID, "function_call", callID, name, int(index))
							block.arguments, _ = item["arguments"].(string)
						} else if kind == "reasoning" {
							signature, _ := item["anthropic_signature"].(string)
							encrypted, _ := item["encrypted_content"].(string)
							switch {
							case signature != "":
								startBlock(itemID, "reasoning", "", "", int(index), signature)
							case encrypted != "":
								startBlock(itemID, "redacted_reasoning", "", "", int(index), encrypted)
							default:
								// completed 项目可能是首个携带 encrypted_content 的事件。
								// 延迟到 delta 或 done 事件再选择 thinking 或 redacted_thinking。
								registerPendingReasoning(itemID, int(index))
							}
						} else if kind == "web_search_call" {
							block := &directStreamBlock{kind: "pending_web_search", itemID: itemID, outputIndex: int(index)}
							blocksByItem[itemID] = block
							blocksByOutput[int(index)] = block
						} else if kind != "message" {
							emitMessageStart()
							log.Printf("[协议桥接警告] Responses->Anthropic 不支持输出项目类型=%q ID=%q", kind, itemID)
							emitError(event, "unsupported Responses output item type: "+kind)
							failed = true
						}
					case "response.content_part.added":
						itemID, _ := event["item_id"].(string)
						index, _ := GetFloat(event, "output_index")
						part, _ := event["part"].(map[string]any)
						partType, _ := part["type"].(string)
						if partType != "" && partType != "output_text" {
							emitMessageStart()
							log.Printf("[协议桥接警告] Responses->Anthropic 不支持内容分段类型=%q 项目=%q", partType, itemID)
							emitError(event, "unsupported Responses content part type: "+partType)
							failed = true
						} else {
							startBlock(itemID, "message", "", "", int(index))
						}
					case "response.output_text.delta":
						itemID, _ := event["item_id"].(string)
						index, _ := GetFloat(event, "output_index")
						block := startBlock(itemID, "message", "", "", int(index))
						text, _ := event["delta"].(string)
						block.text += text
						emit("content_block_delta", map[string]any{
							"type": "content_block_delta", "index": block.contentIndex,
							"delta": map[string]any{"type": "text_delta", "text": text},
						})
					case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
						itemID, _ := event["item_id"].(string)
						index, _ := GetFloat(event, "output_index")
						block := blocksByItem[itemID]
						if block != nil && block.kind == "redacted_reasoning" {
							log.Printf("[协议桥接警告] Responses->Anthropic 已跳过加密项目的可见推理增量 项目=%q", itemID)
							continue
						}
						block = startBlock(itemID, "reasoning", "", "", int(index))
						text, _ := event["delta"].(string)
						block.text += text
						emit("content_block_delta", map[string]any{
							"type": "content_block_delta", "index": block.contentIndex,
							"delta": map[string]any{"type": "thinking_delta", "thinking": text},
						})
					case "response.function_call_arguments.delta":
						itemID, _ := event["item_id"].(string)
						block := blocksByItem[itemID]
						if block == nil {
							index, _ := GetFloat(event, "output_index")
							block = startBlock(itemID, "function_call", itemID, "", int(index))
						}
						fragment, _ := event["delta"].(string)
						block.argumentDeltaSent = true
						emit("content_block_delta", map[string]any{
							"type": "content_block_delta", "index": block.contentIndex,
							"delta": map[string]any{"type": "input_json_delta", "partial_json": fragment},
						})
					case "response.output_item.done":
						item, _ := event["item"].(map[string]any)
						itemID, _ := item["id"].(string)
						block := blocksByItem[itemID]
						kind, _ := item["type"].(string)
						if kind == "web_search_call" {
							index, _ := GetFloat(event, "output_index")
							block = emitWebSearchCall(item, int(index))
						} else if kind == "reasoning" && (block == nil || !block.started) {
							index, _ := GetFloat(event, "output_index")
							signature, _ := item["anthropic_signature"].(string)
							encrypted, _ := item["encrypted_content"].(string)
							if signature != "" {
								block = startBlock(itemID, "reasoning", "", "", int(index), signature)
							} else if encrypted != "" {
								block = startBlock(itemID, "redacted_reasoning", "", "", int(index), encrypted)
							} else {
								block = startBlock(itemID, "reasoning", "", "", int(index))
							}
						}
						if block != nil {
							if block.kind == "function_call" && !block.argumentDeltaSent && block.arguments == "" {
								block.arguments, _ = item["arguments"].(string)
							}
							if block.kind == "reasoning" && block.signature == "" {
								block.signature, _ = item["anthropic_signature"].(string)
							}
							finishBlock(block)
						}
					case "response.completed", "response.incomplete":
						var terminalResponse map[string]any
						if response, ok := event["response"].(map[string]any); ok {
							terminalResponse = response
							if usage, ok := response["usage"].(map[string]any); ok {
								totalUsage = usage
							}
						}
						emitMessageStart()
						stopReason := "end_turn"
						if eventType == "response.incomplete" {
							stopReason = ResponsesIncompleteToAnthropicStopReason(terminalResponse)
						} else if hasToolUse {
							stopReason = "tool_use"
						}
						if terminalMessage(stopReason) {
							terminal = true
						} else {
							emitError(event, "Responses stream completed before an output item was finalized")
							failed = true
						}
					case "response.failed", "response.error", "error":
						emitMessageStart()
						emitError(event, "upstream Responses stream failed")
						failed = true
					}
				}
			}
		}
		if terminal || failed {
			break
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("读取 Responses->Anthropic 直连流失败：%v", err)
			}
			break
		}
	}
	usageStats.observeMap(totalUsage)
	if !terminal && !failed {
		emitMessageStart()
		emitError(nil, "upstream Responses stream ended before a terminal event")
	}
}
