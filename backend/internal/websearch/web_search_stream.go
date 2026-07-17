package websearch

import (
	"encoding/json"
	"net/http"
)

// WriteBufferedResponsesStream 根据已完成的降级循环输出符合规范的 Responses SSE 流。
// 网关必须先完成托管搜索才能返回最终答案，因此此路径会主动缓冲；
// 原生 Responses 上游仍保留原有的流式行为。
func WriteBufferedResponsesStream(w http.ResponseWriter, body []byte) {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		WriteClientAPIError(w, WireResponses, http.StatusBadGateway, "upstream_protocol_error", "invalid buffered Responses result")
		return
	}
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	sequence := 0
	emit := func(event string, payload map[string]any) {
		sequence++
		payload["type"] = event
		payload["sequence_number"] = sequence
		EmitSSEEvent(w, flusher, event, payload)
	}

	created := CloneJSONMap(response)
	created["status"] = "in_progress"
	created["output"] = []any{}
	created["usage"] = nil
	emit("response.created", map[string]any{"response": created})

	output, _ := response["output"].([]any)
	for outputIndex, raw := range output {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		added := CloneJSONMap(item)
		itemType, _ := item["type"].(string)
		if itemType == "message" {
			added["content"] = []any{}
		}
		if _, exists := added["status"]; exists {
			added["status"] = "in_progress"
		}
		emit("response.output_item.added", map[string]any{"output_index": outputIndex, "item": added})

		if itemType == "message" {
			content, _ := item["content"].([]any)
			for contentIndex, rawPart := range content {
				part, _ := rawPart.(map[string]any)
				if part == nil {
					continue
				}
				partType, _ := part["type"].(string)
				emptyPart := CloneJSONMap(part)
				if partType == "output_text" {
					emptyPart["text"] = ""
					emptyPart["annotations"] = []any{}
					emptyPart["logprobs"] = []any{}
				} else if partType == "refusal" {
					emptyPart["refusal"] = ""
				}
				emit("response.content_part.added", map[string]any{
					"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex, "part": emptyPart,
				})
				if partType == "output_text" {
					text, _ := part["text"].(string)
					logprobs := BridgeArray(part["logprobs"])
					if text != "" {
						emit("response.output_text.delta", map[string]any{
							"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex,
							"delta": text, "logprobs": logprobs,
						})
					}
					for annotationIndex, annotation := range BridgeArray(part["annotations"]) {
						emit("response.output_text.annotation.added", map[string]any{
							"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex,
							"annotation_index": annotationIndex, "annotation": annotation,
						})
					}
					emit("response.output_text.done", map[string]any{
						"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex,
						"text": text, "logprobs": logprobs,
					})
				} else if partType == "refusal" {
					refusal, _ := part["refusal"].(string)
					if refusal != "" {
						emit("response.refusal.delta", map[string]any{
							"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex, "delta": refusal,
						})
					}
					emit("response.refusal.done", map[string]any{
						"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex, "refusal": refusal,
					})
				}
				emit("response.content_part.done", map[string]any{
					"item_id": item["id"], "output_index": outputIndex, "content_index": contentIndex, "part": part,
				})
			}
		}
		emit("response.output_item.done", map[string]any{"output_index": outputIndex, "item": item})
	}

	status, _ := response["status"].(string)
	if status == "" {
		status = "completed"
	}
	emit("response."+status, map[string]any{"response": response})
	if flusher != nil {
		flusher.Flush()
	}
}

func CloneJSONMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
