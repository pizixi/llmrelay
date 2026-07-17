package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func ResponsesStreamHandler(w http.ResponseWriter, _ *http.Request, resp *http.Response, model, usageModel string, tools any, toolChoice any, parallelToolCalls *bool, toolNameMappings map[string]ResponseToolNameMapping, responseEcho map[string]any, warningGroups ...[]BridgeWarning) {
	usageStats := NewRequestUsageAccumulator(usageModel)
	defer usageStats.commit()
	var bridgeWarnings []BridgeWarning
	bridgeWarnings = AppendBridgeWarnings(bridgeWarnings, warningGroups...)
	SetSSEHeaders(w.Header())
	WriteBridgeWarningHeaders(w.Header(), bridgeWarnings)
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	responseID := "resp_" + time.Now().Format("20060102150405") + "_" + RandomString(8)
	reasoningID := "rs_" + responseID
	msgID := "msg_" + responseID + "_0"
	createdAt := time.Now().Unix()
	seq := 0

	reasoningStarted := false
	reasoningDone := false
	messageStarted := false
	textPartStarted := false
	refusalPartStarted := false
	messageDone := false
	isIncomplete := false
	incompleteReason := ""
	terminalFinishSeen := false
	streamFailed := false
	streamFailure := map[string]any(nil)
	fullReasoning := ""
	fullText := ""
	fullRefusal := ""
	fullAnnotations := []any{}
	fullLogprobs := []any{}
	totalUsage := map[string]any{}
	var serviceTier any
	createdSent := false
	toolCalls := map[int]map[string]any{}
	toolOrder := []int{}
	nextOutputIndex := 0
	reasoningOutputIndex := -1
	messageOutputIndex := -1
	textContentIndex := -1
	refusalContentIndex := -1
	nextContentIndex := 0
	allocateOutputIndex := func() int {
		idx := nextOutputIndex
		nextOutputIndex++
		return idx
	}

	reasoningItem := func(status string) map[string]any {
		item := map[string]any{
			"id":      reasoningID,
			"type":    "reasoning",
			"summary": []any{},
		}
		if status != "" {
			item["status"] = status
		}
		if status == "completed" {
			item["encrypted_content"] = ""
		}
		if fullReasoning != "" {
			item["summary"] = []any{map[string]any{"type": "summary_text", "text": fullReasoning}}
		}
		return item
	}

	messageItem := func(status string) map[string]any {
		content := make([]any, nextContentIndex)
		if textPartStarted {
			content[textContentIndex] = map[string]any{
				"type": "output_text", "annotations": fullAnnotations,
				"logprobs": fullLogprobs, "text": fullText,
			}
		}
		if refusalPartStarted {
			content[refusalContentIndex] = map[string]any{"type": "refusal", "refusal": fullRefusal}
		}
		return map[string]any{
			"id":      msgID,
			"type":    "message",
			"status":  status,
			"content": content,
			"role":    "assistant",
		}
	}
	emitMessageStart := func() {
		if messageStarted {
			return
		}
		messageOutputIndex = allocateOutputIndex()
		seq++
		EmitSSEEvent(w, flusher, "response.output_item.added", map[string]any{
			"type":            "response.output_item.added",
			"sequence_number": seq,
			"output_index":    messageOutputIndex,
			"item":            map[string]any{"id": msgID, "type": "message", "status": "in_progress", "content": []any{}, "role": "assistant"},
		})
		messageStarted = true
	}
	emitTextPartStart := func() {
		if textPartStarted {
			return
		}
		emitMessageStart()
		textContentIndex = nextContentIndex
		nextContentIndex++
		seq++
		EmitSSEEvent(w, flusher, "response.content_part.added", map[string]any{
			"type":            "response.content_part.added",
			"sequence_number": seq,
			"item_id":         msgID,
			"output_index":    messageOutputIndex,
			"content_index":   textContentIndex,
			"part":            map[string]any{"type": "output_text", "annotations": []any{}, "logprobs": []any{}, "text": ""},
		})
		textPartStarted = true
	}
	emitRefusalPartStart := func() {
		if refusalPartStarted {
			return
		}
		emitMessageStart()
		refusalContentIndex = nextContentIndex
		nextContentIndex++
		seq++
		EmitSSEEvent(w, flusher, "response.content_part.added", map[string]any{
			"type":            "response.content_part.added",
			"sequence_number": seq,
			"item_id":         msgID,
			"output_index":    messageOutputIndex,
			"content_index":   refusalContentIndex,
			"part":            map[string]any{"type": "refusal", "refusal": ""},
		})
		refusalPartStarted = true
	}

	emitReasoningDone := func() {
		if !reasoningStarted || reasoningDone {
			return
		}
		seq++
		EmitSSEEvent(w, flusher, "response.reasoning_summary_text.done", map[string]any{
			"type":            "response.reasoning_summary_text.done",
			"sequence_number": seq,
			"item_id":         reasoningID,
			"output_index":    reasoningOutputIndex,
			"summary_index":   0,
			"text":            fullReasoning,
		})
		seq++
		EmitSSEEvent(w, flusher, "response.reasoning_summary_part.done", map[string]any{
			"type":            "response.reasoning_summary_part.done",
			"sequence_number": seq,
			"item_id":         reasoningID,
			"output_index":    reasoningOutputIndex,
			"summary_index":   0,
			"part":            map[string]any{"type": "summary_text", "text": fullReasoning},
		})
		seq++
		EmitSSEEvent(w, flusher, "response.output_item.done", map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": seq,
			"output_index":    reasoningOutputIndex,
			"item":            reasoningItem("completed"),
		})
		reasoningDone = true
	}

	emitMessageDone := func() {
		if !messageStarted || messageDone {
			return
		}
		idx := messageOutputIndex
		if textPartStarted {
			seq++
			EmitSSEEvent(w, flusher, "response.output_text.done", map[string]any{
				"type":            "response.output_text.done",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    idx,
				"content_index":   textContentIndex,
				"text":            fullText,
				"logprobs":        fullLogprobs,
			})
			seq++
			EmitSSEEvent(w, flusher, "response.content_part.done", map[string]any{
				"type":            "response.content_part.done",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    idx,
				"content_index":   textContentIndex,
				"part":            map[string]any{"type": "output_text", "annotations": fullAnnotations, "logprobs": fullLogprobs, "text": fullText},
			})
		}
		if refusalPartStarted {
			seq++
			EmitSSEEvent(w, flusher, "response.refusal.done", map[string]any{
				"type":            "response.refusal.done",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    idx,
				"content_index":   refusalContentIndex,
				"refusal":         fullRefusal,
			})
			seq++
			EmitSSEEvent(w, flusher, "response.content_part.done", map[string]any{
				"type":            "response.content_part.done",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    idx,
				"content_index":   refusalContentIndex,
				"part":            map[string]any{"type": "refusal", "refusal": fullRefusal},
			})
		}
		seq++
		EmitSSEEvent(w, flusher, "response.output_item.done", map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": seq,
			"output_index":    idx,
			"item":            messageItem("completed"),
		})
		messageDone = true
	}

	emitToolCallDone := func(idx int, call map[string]any) {
		if done, _ := call["done"].(bool); done {
			return
		}
		itemID, _ := call["item_id"].(string)
		callID, _ := call["call_id"].(string)
		name, _ := call["name"].(string)
		args, _ := call["arguments"].(string)
		normalizedArgs, err := NormalizeToolCallArguments(args)
		if err != nil {
			LogStreamToolCallArgumentsValidationFailure("ResponsesStreamHandler.emitToolCallDone", itemID, callID, name, args, idx, err)
			isIncomplete = true
			if incompleteReason == "" {
				incompleteReason = "tool_call_arguments_incomplete"
			}
			return
		}
		call["arguments"] = normalizedArgs
		call["done"] = true
		mapping, mapped := LookupResponseToolNameMapping(name, toolNameMappings)
		if mapped && mapping.Kind == "custom" {
			seq++
			EmitSSEEvent(w, flusher, "response.custom_tool_call_input.done", map[string]any{
				"type":            "response.custom_tool_call_input.done",
				"sequence_number": seq,
				"item_id":         itemID,
				"output_index":    idx,
				"input":           CustomToolInputFromArguments(normalizedArgs),
			})
		} else if !mapped || mapping.Kind != "tool_search" {
			seq++
			EmitSSEEvent(w, flusher, "response.function_call_arguments.done", map[string]any{
				"type":            "response.function_call_arguments.done",
				"sequence_number": seq,
				"item_id":         itemID,
				"output_index":    idx,
				"arguments":       normalizedArgs,
			})
		}
		seq++
		EmitSSEEvent(w, flusher, "response.output_item.done", map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": seq,
			"output_index":    idx,
			"item":            ResponseFunctionCallItem(itemID, "completed", normalizedArgs, callID, name, toolNameMappings),
		})
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line == "" {
					break
				}
			}
			if err != io.EOF {
				// 客户端取消请求（例如 Codex 中断当前轮次）会令上游流读取返回
				// context.Canceled；这不是上游故障，按正常结束处理即可，避免噪声日志。
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					break
				}
				log.Printf("读取流失败：%v", err)
				streamFailed = true
				streamFailure = StreamErrorObject(map[string]any{"message": err.Error()}, "failed to read upstream stream")
				break
			}
		}
		payload, isDataLine := SseDataPayload(line)
		if debugMode && debugLogBodies && isDataLine {
			log.Printf("[上游原始数据块] %s", strings.TrimSpace(payload))
		}

		trimmed := strings.TrimSpace(line)
		if (isDataLine && payload == "[DONE]") || trimmed == "[DONE]" {
			break
		}
		if !isDataLine {
			continue
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if value, exists := chunk["service_tier"]; exists && value != nil {
			serviceTier = value
		}
		if !createdSent {
			if id, ok := chunk["id"].(string); ok && id != "" {
				responseID = id
				reasoningID = "rs_" + responseID + "_0"
				msgID = "msg_" + responseID + "_0"
			}
			if created, ok := chunk["created"].(float64); ok {
				createdAt = int64(created)
			}
			createdResponse := map[string]any{
				"id": responseID, "object": "response", "created_at": createdAt,
				"status": "in_progress", "background": false, "error": nil,
				"incomplete_details": nil, "model": model, "output": []any{},
				"tools": []any{}, "tool_choice": "auto", "parallel_tool_calls": true,
			}
			if normalizedTools, ok := ResponsesToolsForOutput(tools); ok {
				createdResponse["tools"] = normalizedTools
			}
			if toolChoice != nil {
				createdResponse["tool_choice"] = toolChoice
			}
			if parallelToolCalls != nil {
				createdResponse["parallel_tool_calls"] = *parallelToolCalls
			}
			if serviceTier != nil {
				createdResponse["service_tier"] = serviceTier
			}
			ApplyResponsesRequestEcho(createdResponse, responseEcho)
			ApplyBridgeWarnings(createdResponse, bridgeWarnings)
			seq++
			EmitSSEEvent(w, flusher, "response.created", map[string]any{
				"type":            "response.created",
				"sequence_number": seq,
				"response":        createdResponse,
			})
			seq++
			EmitSSEEvent(w, flusher, "response.in_progress", map[string]any{
				"type":            "response.in_progress",
				"sequence_number": seq,
				"response":        createdResponse,
			})
			createdSent = true
		}
		chunkType, _ := chunk["type"].(string)
		if chunk["error"] != nil || chunkType == "error" || chunkType == "response.failed" {
			if usage, ok := chunk["usage"].(map[string]any); ok {
				totalUsage = usage
			}
			streamFailed = true
			streamFailure = StreamErrorObject(chunk, "upstream stream failed")
			break
		}
		choices, ok := chunk["choices"].([]any)
		if !ok || len(choices) == 0 {
			if usage, ok := chunk["usage"].(map[string]any); ok {
				totalUsage = usage
			}
			continue
		}

		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		NormalizeReasoningContent(delta)
		annotations, _ := delta["annotations"].([]any)
		choiceLogprobs, _ := choice["logprobs"].(map[string]any)
		chunkLogprobs, _ := choiceLogprobs["content"].([]any)
		finishReason, _ := choice["finish_reason"].(string)

		if rc, ok := delta["reasoning_content"]; ok {
			rcStr, _ := rc.(string)
			if rcStr != "" {
				if !reasoningStarted {
					reasoningOutputIndex = allocateOutputIndex()
					seq++
					EmitSSEEvent(w, flusher, "response.output_item.added", map[string]any{
						"type":            "response.output_item.added",
						"sequence_number": seq,
						"output_index":    reasoningOutputIndex,
						"item":            reasoningItem("in_progress"),
					})
					seq++
					EmitSSEEvent(w, flusher, "response.reasoning_summary_part.added", map[string]any{
						"type":            "response.reasoning_summary_part.added",
						"sequence_number": seq,
						"item_id":         reasoningID,
						"output_index":    reasoningOutputIndex,
						"summary_index":   0,
						"part":            map[string]any{"type": "summary_text", "text": ""},
					})
					reasoningStarted = true
				}
				fullReasoning += rcStr
				seq++
				EmitSSEEvent(w, flusher, "response.reasoning_summary_text.delta", map[string]any{
					"type":            "response.reasoning_summary_text.delta",
					"sequence_number": seq,
					"item_id":         reasoningID,
					"output_index":    reasoningOutputIndex,
					"summary_index":   0,
					"delta":           rcStr,
				})
			}
		}

		contentStr := ""
		if c, ok := delta["content"]; ok && c != nil {
			contentStr, _ = c.(string)
		}
		refusalStr, _ := delta["refusal"].(string)
		if contentStr != "" {
			emitReasoningDone()
			emitTextPartStart()
			fullText += contentStr
			fullLogprobs = append(fullLogprobs, chunkLogprobs...)
			seq++
			EmitSSEEvent(w, flusher, "response.output_text.delta", map[string]any{
				"type":            "response.output_text.delta",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    messageOutputIndex,
				"content_index":   textContentIndex,
				"delta":           contentStr,
				"logprobs":        chunkLogprobs,
			})
		}
		if refusalStr != "" {
			emitReasoningDone()
			emitRefusalPartStart()
			fullRefusal += refusalStr
			seq++
			EmitSSEEvent(w, flusher, "response.refusal.delta", map[string]any{
				"type":            "response.refusal.delta",
				"sequence_number": seq,
				"item_id":         msgID,
				"output_index":    messageOutputIndex,
				"content_index":   refusalContentIndex,
				"delta":           refusalStr,
			})
		}
		if len(annotations) > 0 {
			emitReasoningDone()
			emitTextPartStart()
			for _, annotation := range annotations {
				annotationIndex := len(fullAnnotations)
				fullAnnotations = append(fullAnnotations, annotation)
				seq++
				EmitSSEEvent(w, flusher, "response.output_text.annotation.added", map[string]any{
					"type":             "response.output_text.annotation.added",
					"sequence_number":  seq,
					"item_id":          msgID,
					"output_index":     messageOutputIndex,
					"content_index":    textContentIndex,
					"annotation_index": annotationIndex,
					"annotation":       annotation,
				})
			}
		}

		rawToolCalls, _ := delta["tool_calls"].([]any)
		for _, rawToolCall := range rawToolCalls {
			tc, ok := rawToolCall.(map[string]any)
			if !ok {
				continue
			}
			idxFloat, _ := tc["index"].(float64)
			upstreamIndex := int(idxFloat)
			call, exists := toolCalls[upstreamIndex]
			if !exists {
				outputIndex := allocateOutputIndex()
				callID, _ := tc["id"].(string)
				if callID == "" {
					callID = "call_" + RandomString(12)
				}
				fn, _ := tc["function"].(map[string]any)
				name, _ := fn["name"].(string)
				call = map[string]any{
					"output_index": outputIndex,
					"item_id":      ResponseToolCallItemID(callID, name, toolNameMappings),
					"call_id":      callID,
					"name":         name,
					"arguments":    "",
					"done":         false,
				}
				toolCalls[upstreamIndex] = call
				toolOrder = append(toolOrder, upstreamIndex)
				seq++
				EmitSSEEvent(w, flusher, "response.output_item.added", map[string]any{
					"type":            "response.output_item.added",
					"sequence_number": seq,
					"output_index":    outputIndex,
					"item":            ResponseFunctionCallItem(call["item_id"].(string), "in_progress", "", callID, name, toolNameMappings),
				})
			}
			fn, _ := tc["function"].(map[string]any)
			if name, _ := fn["name"].(string); name != "" {
				call["name"] = name
			}
			if argDelta, _ := fn["arguments"].(string); argDelta != "" {
				call["arguments"] = call["arguments"].(string) + argDelta
				mapping, mapped := LookupResponseToolNameMapping(call["name"].(string), toolNameMappings)
				if !mapped || (mapping.Kind != "custom" && mapping.Kind != "tool_search") {
					seq++
					EmitSSEEvent(w, flusher, "response.function_call_arguments.delta", map[string]any{
						"type":            "response.function_call_arguments.delta",
						"sequence_number": seq,
						"item_id":         call["item_id"],
						"output_index":    call["output_index"],
						"delta":           argDelta,
					})
				}
			}
		}

		if usage, ok := chunk["usage"].(map[string]any); ok {
			totalUsage = usage
		}
		if finishReason == "stop" || finishReason == "length" || finishReason == "tool_calls" || finishReason == "function_call" || finishReason == "content_filter" {
			terminalFinishSeen = true
			if finishReason == "length" {
				isIncomplete = true
				incompleteReason = "max_output_tokens"
			} else if finishReason == "content_filter" {
				isIncomplete = true
				incompleteReason = "content_filter"
			}
			emitReasoningDone()
			if !messageStarted && len(toolCalls) == 0 {
				emitTextPartStart()
			}
			emitMessageDone()
			for _, idx := range toolOrder {
				emitToolCallDone(toolCalls[idx]["output_index"].(int), toolCalls[idx])
			}
		}
	}

	if !terminalFinishSeen && !streamFailed {
		isIncomplete = true
		if incompleteReason == "" {
			incompleteReason = "stream_ended_early"
		}
		log.Printf("[Responses 流不完整] 模型=%q 原因=%s 消息已开始=%t 工具调用数=%d", model, incompleteReason, messageStarted, len(toolOrder))
	}

	if !streamFailed {
		emitReasoningDone()
		emitMessageDone()
	}
	if terminalFinishSeen && !streamFailed {
		for _, idx := range toolOrder {
			emitToolCallDone(toolCalls[idx]["output_index"].(int), toolCalls[idx])
		}
	}

	outputByIndex := map[int]any{}
	itemStatus := "completed"
	if streamFailed {
		itemStatus = "in_progress"
	}
	if reasoningStarted {
		outputByIndex[reasoningOutputIndex] = reasoningItem(itemStatus)
	}
	if messageStarted {
		outputByIndex[messageOutputIndex] = messageItem(itemStatus)
	}
	for _, idx := range toolOrder {
		call := toolCalls[idx]
		args, _ := call["arguments"].(string)
		normalizedArgs, err := NormalizeToolCallArguments(args)
		toolItemStatus := "completed"
		if err != nil {
			itemID, _ := call["item_id"].(string)
			callID, _ := call["call_id"].(string)
			name, _ := call["name"].(string)
			LogStreamToolCallArgumentsValidationFailure("ResponsesStreamHandler.output", itemID, callID, name, args, call["output_index"].(int), err)
			isIncomplete = true
			if incompleteReason == "" {
				incompleteReason = "tool_call_arguments_incomplete"
			}
			toolItemStatus = "incomplete"
			normalizedArgs = args
		}
		call["arguments"] = normalizedArgs
		if toolItemStatus == "completed" && (!terminalFinishSeen || streamFailed) {
			toolItemStatus = "in_progress"
		}
		outputByIndex[call["output_index"].(int)] = ResponseFunctionCallItem(
			call["item_id"].(string),
			toolItemStatus,
			normalizedArgs,
			call["call_id"].(string),
			call["name"].(string),
			toolNameMappings,
		)
	}
	output := make([]any, 0, len(outputByIndex))
	for outputIndex := 0; outputIndex < nextOutputIndex; outputIndex++ {
		if item, exists := outputByIndex[outputIndex]; exists {
			output = append(output, item)
		}
	}

	responseStatus := "completed"
	incompleteDetails := any(nil)
	responseError := any(nil)
	if streamFailed {
		responseStatus = "failed"
		if streamFailure == nil {
			streamFailure = StreamErrorObject(nil, "upstream stream failed")
		}
		if _, ok := streamFailure["code"]; !ok {
			streamFailure["code"] = streamFailure["type"]
		}
		responseError = streamFailure
	} else if isIncomplete {
		responseStatus = "incomplete"
		reason := incompleteReason
		if reason == "" {
			reason = "max_output_tokens"
		}
		incompleteDetails = map[string]any{"reason": reason}
	}
	completedResponse := map[string]any{
		"id":                  responseID,
		"object":              "response",
		"created_at":          createdAt,
		"status":              responseStatus,
		"background":          false,
		"error":               responseError,
		"incomplete_details":  incompleteDetails,
		"model":               model,
		"output":              output,
		"tools":               []any{},
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
	}
	if normalizedTools, ok := ResponsesToolsForOutput(tools); ok {
		completedResponse["tools"] = normalizedTools
	}
	if toolChoice != nil {
		completedResponse["tool_choice"] = toolChoice
	}
	if parallelToolCalls != nil {
		completedResponse["parallel_tool_calls"] = *parallelToolCalls
	}
	if serviceTier != nil {
		completedResponse["service_tier"] = serviceTier
	}
	ApplyResponsesRequestEcho(completedResponse, responseEcho)
	ApplyBridgeWarnings(completedResponse, bridgeWarnings)

	usage := map[string]any{}
	if len(totalUsage) > 0 {
		if v, ok := totalUsage["prompt_tokens"]; ok {
			usage["input_tokens"] = v
		}
		if v, ok := totalUsage["prompt_tokens_details"]; ok {
			usage["input_tokens_details"] = v
		} else {
			usage["input_tokens_details"] = map[string]any{"cached_tokens": 0}
		}
		if v, ok := totalUsage["completion_tokens"]; ok {
			usage["output_tokens"] = v
		}
		if v, ok := totalUsage["completion_tokens_details"]; ok {
			usage["output_tokens_details"] = v
		}
		if v, ok := totalUsage["total_tokens"]; ok {
			usage["total_tokens"] = v
		}
		if v, ok := totalUsage["input_tokens"]; ok && usage["input_tokens"] == nil {
			usage["input_tokens"] = v
		}
		if v, ok := totalUsage["output_tokens"]; ok && usage["output_tokens"] == nil {
			usage["output_tokens"] = v
		}
	}
	// 始终确保存在 total_tokens。
	if _, ok := usage["total_tokens"]; !ok {
		pt := float64(0)
		ct := float64(0)
		if v, ok := usage["input_tokens"].(float64); ok {
			pt = v
		} else if v, ok := usage["input_tokens"].(int64); ok {
			pt = float64(v)
		}
		if v, ok := usage["output_tokens"].(float64); ok {
			ct = v
		} else if v, ok := usage["output_tokens"].(int64); ok {
			ct = float64(v)
		}
		usage["total_tokens"] = pt + ct
	}
	// 确保 usage 字段完整
	if _, ok := usage["input_tokens"]; !ok {
		usage["input_tokens"] = float64(0)
	}
	if _, ok := usage["output_tokens"]; !ok {
		usage["output_tokens"] = float64(0)
	}
	completedResponse["usage"] = usage

	usageStats.observeMap(totalUsage)

	seq++
	EmitSSEEvent(w, flusher, "response."+responseStatus, map[string]any{
		"type":            "response." + responseStatus,
		"sequence_number": seq,
		"response":        completedResponse,
	})

	if flusher != nil {
		flusher.Flush()
	}
}

// convertChatToResponsesForRequest 回显请求配置时使用原始 Responses 请求。
// 这样可避免丢失内置工具或命名空间工具，并保留客户端扁平的函数工具及 tool_choice 格式。
