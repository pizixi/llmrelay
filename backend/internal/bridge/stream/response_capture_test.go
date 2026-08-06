package stream

import (
	"net/http/httptest"
	"testing"
)

func TestResponseCaptureWriterObservesTerminalEventWithoutRewriting(t *testing.T) {
	recorder := httptest.NewRecorder()
	var captured map[string]any
	capture := newResponseCaptureWriter(recorder, func(response map[string]any) {
		captured = response
	})
	first := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":")
	second := []byte("{\"id\":\"resp_1\",\"output\":[]}}\n\n")
	if _, err := capture.Write(first); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Write(second); err != nil {
		t.Fatal(err)
	}
	capture.FlushObserved()
	if recorder.Body.String() != string(append(first, second...)) {
		t.Fatalf("captured stream changed: %q", recorder.Body.String())
	}
	if captured == nil || captured["id"] != "resp_1" {
		t.Fatalf("captured response=%#v", captured)
	}
}
