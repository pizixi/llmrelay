package gateway

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatRefusalToAnthropicJSONAndStream(t *testing.T) {
	jsonBody := []byte(`{"choices":[{"finish_reason":"stop","message":{"content":null,"refusal":"cannot comply"}}]}`)
	converted, err := openAIToClaudeResponseWithError(jsonBody, "model")
	if err != nil {
		t.Fatal(err)
	}
	message := decodePairwiseTestObject(t, converted)
	requireTestEqual(t, "stop_reason", message["stop_reason"], "refusal")
	content := testArray(t, message["content"], "content")
	requireTestEqual(t, "refusal text", testObject(t, content[0], "text")["text"], "cannot comply")
	stream := "data: {\"choices\":[{\"delta\":{\"refusal\":\"cannot comply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	claudeStreamHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model", "usage")
	if !strings.Contains(recorder.Body.String(), `"text":"cannot comply"`) || !strings.Contains(recorder.Body.String(), `"stop_reason":"refusal"`) {
		t.Fatalf("refusal lost: %s", recorder.Body.String())
	}
}

func TestResponsesRefusalToChatStream(t *testing.T) {
	stream := testNamedSSE("response.refusal.delta", `{"type":"response.refusal.delta","item_id":"msg_1","delta":"cannot comply"}`) +
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"status":"completed"}}`)
	recorder := httptest.NewRecorder()
	responsesStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model", "usage", false)
	if !strings.Contains(recorder.Body.String(), `"refusal":"cannot comply"`) || strings.Contains(recorder.Body.String(), `"error"`) {
		t.Fatalf("refusal lost: %s", recorder.Body.String())
	}
}

func TestResponsesContentPartsToAnthropicRemainSeparate(t *testing.T) {
	stream := testNamedSSE("response.content_part.added", `{"type":"response.content_part.added","item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`) +
		testNamedSSE("response.output_text.delta", `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"first"}`) +
		testNamedSSE("response.output_text.done", `{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"first"}`) +
		testNamedSSE("response.content_part.done", `{"type":"response.content_part.done","item_id":"msg_1","output_index":0,"content_index":0}`) +
		testNamedSSE("response.content_part.added", `{"type":"response.content_part.added","item_id":"msg_1","output_index":0,"content_index":1,"part":{"type":"refusal","refusal":""}}`) +
		testNamedSSE("response.refusal.delta", `{"type":"response.refusal.delta","item_id":"msg_1","output_index":0,"content_index":1,"delta":"cannot "}`) +
		testNamedSSE("response.refusal.done", `{"type":"response.refusal.done","item_id":"msg_1","output_index":0,"content_index":1,"refusal":"cannot comply"}`) +
		testNamedSSE("response.content_part.done", `{"type":"response.content_part.done","item_id":"msg_1","output_index":0,"content_index":1}`) +
		testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_1","type":"message"}}`) +
		testNamedSSE("response.completed", `{"type":"response.completed","response":{"status":"completed"}}`)
	recorder := httptest.NewRecorder()
	responsesStreamToAnthropicDirectHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model", "usage")
	events := parseTestSSE(t, recorder.Body.String())
	opened, stopped := map[float64]bool{}, map[float64]bool{}
	texts := map[float64]string{}
	for _, event := range events {
		index, _ := event.data["index"].(float64)
		switch event.name {
		case "error":
			t.Fatalf("unexpected error: %s", recorder.Body.String())
		case "content_block_start":
			if opened[index] {
				t.Fatalf("duplicate start for block %v", index)
			}
			opened[index] = true
		case "content_block_delta":
			if !opened[index] || stopped[index] {
				t.Fatalf("delta outside open block %v", index)
			}
			texts[index] += testObject(t, event.data["delta"], "delta")["text"].(string)
		case "content_block_stop":
			if !opened[index] || stopped[index] {
				t.Fatalf("invalid stop for block %v", index)
			}
			stopped[index] = true
		}
	}
	if len(stopped) != 2 || texts[0] != "first" || texts[1] != "cannot comply" {
		t.Fatalf("incorrect block lifecycle: opened=%v stopped=%v texts=%v", opened, stopped, texts)
	}
	terminal := findTestSSEEvent(t, events, "message_delta")
	requireTestEqual(t, "stop reason", testObject(t, terminal.data["delta"], "delta")["stop_reason"], "refusal")
}

func TestResponsesFilteredStreamUsesChatContentFilterFinish(t *testing.T) {
	stream := testNamedSSE("response.incomplete", `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`)
	recorder := httptest.NewRecorder()
	responsesStreamToChatHandler(recorder, io.NopCloser(strings.NewReader(stream)), "model", "usage", false)
	if !strings.Contains(recorder.Body.String(), `"finish_reason":"content_filter"`) {
		t.Fatalf("incorrect finish reason: %s", recorder.Body.String())
	}
}
