package convert

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decodeConversionTestObject(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return object
}

func TestAnthropicChatRoundTripPreservesContentOrder(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "before"},
		map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.com/a.png"}},
		map[string]any{"type": "text", "text": "between"},
		map[string]any{"type": "document", "title": "report.pdf", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "cGRm"}},
		map[string]any{"type": "text", "text": "after"},
	}
	messages := ClaudeToOpenAIMessages([]ClaudeMessage{{Role: "user", Content: content}}, nil)
	body, _ := json.Marshal(map[string]any{"model": "test", "messages": messages})
	converted := decodeConversionTestObject(t, OpenAIToAnthropicRequest(body))
	got := converted["messages"].([]any)[0].(map[string]any)["content"]
	if !reflect.DeepEqual(got, content) {
		t.Fatalf("content reordered or changed: got=%#v want=%#v", got, content)
	}
}

func TestChatToolResultPreservesStructuredMedia(t *testing.T) {
	body := []byte(`{"model":"test","messages":[
		{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"inspect","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"call_1","content":[{"type":"text","text":"see image"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}
	]}`)
	converted := decodeConversionTestObject(t, OpenAIToAnthropicRequest(body))
	result := converted["messages"].([]any)[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	blocks, ok := result["content"].([]any)
	if !ok || len(blocks) != 2 || blocks[0].(map[string]any)["text"] != "see image" || blocks[1].(map[string]any)["type"] != "image" {
		t.Fatalf("structured tool result was flattened: %#v", result)
	}
}

func TestAnthropicStructuredToolErrorPreservesErrorAndImage(t *testing.T) {
	items, warnings := AnthropicContentToResponsesItems("user", []any{
		map[string]any{"type": "tool_result", "tool_use_id": "call_1", "is_error": true, "content": []any{
			map[string]any{"type": "text", "text": "failed"},
			map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.com/error.png"}},
		}},
	}, "messages[0].content")
	if len(warnings) != 0 || len(items) != 1 {
		t.Fatalf("items=%#v warnings=%#v", items, warnings)
	}
	output := items[0].(map[string]any)["output"].([]any)
	if len(output) != 3 || output[0].(map[string]any)["text"] != "[tool_error]" || output[2].(map[string]any)["type"] != "input_image" {
		t.Fatalf("error status or media lost: %#v", output)
	}
}

func TestResponsesFileSourcesToAnthropic(t *testing.T) {
	for _, test := range []struct {
		name, field, value, sourceType, dataKey, data string
	}{
		{"base64", "file_data", "cGRm", "base64", "data", "cGRm"},
		{"data URL", "file_data", "data:application/pdf;base64,cGRm", "base64", "data", "cGRm"},
		{"URL", "file_url", "https://example.com/report.pdf", "url", "url", "https://example.com/report.pdf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			blocks, warnings := ResponsesContentToAnthropicBlocks([]any{map[string]any{"type": "input_file", "filename": "report.pdf", test.field: test.value}}, "input")
			if len(warnings) != 0 || len(blocks) != 1 {
				t.Fatalf("blocks=%#v warnings=%#v", blocks, warnings)
			}
			block := blocks[0].(map[string]any)
			source := block["source"].(map[string]any)
			if block["title"] != "report.pdf" || source["type"] != test.sourceType || source[test.dataKey] != test.data {
				t.Fatalf("incorrect file conversion: %#v", block)
			}
		})
	}
}

func TestRefusalHistorySurvivesChatResponsesRoundTrip(t *testing.T) {
	for _, content := range []string{`null,"refusal":"cannot comply"`, `[{"type":"refusal","refusal":"cannot comply"}]`} {
		body := []byte(`{"model":"test","messages":[{"role":"assistant","content":` + content + `}]}`)
		request := decodeConversionTestObject(t, OpenAIToResponsesRequest(body, nil))
		messages, warnings := ResponsesInputToMessagesWithWarnings(request["input"], "", nil)
		if len(warnings) != 0 || len(messages) != 1 || messages[0].AdditionalFields["refusal"] != "cannot comply" {
			t.Fatalf("refusal history lost: messages=%#v warnings=%#v", messages, warnings)
		}
	}
}

func TestChatArrayResponseToAnthropicPreservesRefusal(t *testing.T) {
	body, err := OpenAIToClaudeResponseWithError([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":[{"type":"text","text":"explanation"},{"type":"refusal","refusal":"cannot comply"}]}}]}`), "model")
	if err != nil {
		t.Fatal(err)
	}
	message := decodeConversionTestObject(t, body)
	blocks := message["content"].([]any)
	if len(blocks) != 2 || blocks[0].(map[string]any)["text"] != "explanation" || blocks[1].(map[string]any)["text"] != "cannot comply" || message["stop_reason"] != "refusal" {
		t.Fatalf("array content lost: %s", body)
	}
}
