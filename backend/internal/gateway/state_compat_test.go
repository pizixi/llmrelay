package gateway

import (
	"testing"

	bridgestate "llmrelay/backend/internal/bridge/state"
)

func TestStoreResponsesStreamResponseCapturesTerminalEvent(t *testing.T) {
	bridgestate.Default().Reset()
	t.Cleanup(bridgestate.Default().Reset)

	body := "event: response.created\ndata: {\"type\":\"response.created\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fallback\",\"output\":[]}}\n\n" +
		"data: [DONE]\n\n"
	if !storeResponsesStreamResponse([]byte(body)) {
		t.Fatal("terminal Responses event was not stored")
	}
	if _, ok := bridgestate.Default().Get("resp_fallback"); !ok {
		t.Fatal("stored terminal response cannot be read")
	}
}

func TestStoreResponsesStreamResponseIgnoresMissingTerminalID(t *testing.T) {
	bridgestate.Default().Reset()
	t.Cleanup(bridgestate.Default().Reset)
	body := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n"
	if storeResponsesStreamResponse([]byte(body)) {
		t.Fatal("response without an id must not be stored")
	}
}
