package stream

import (
	"encoding/json"
	"net/http"

	"llmrelay/backend/internal/sse"
)

// responseCaptureWriter observes converted Responses SSE events after the
// bytes have been written. It never buffers or rewrites the client stream.
type responseCaptureWriter struct {
	http.ResponseWriter
	parser *sse.Parser
	onDone func(map[string]any)
	fired  bool
}

func newResponseCaptureWriter(writer http.ResponseWriter, onDone func(map[string]any)) *responseCaptureWriter {
	return &responseCaptureWriter{
		ResponseWriter: writer,
		parser:         sse.NewParser(sse.DefaultMaxEventBytes),
		onDone:         onDone,
	}
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.observe(w.parser.Feed(data[:n]))
	}
	return n, err
}

func (w *responseCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseCaptureWriter) FlushObserved() {
	if w == nil || w.parser == nil {
		return
	}
	w.observe(w.parser.Flush())
}

func (w *responseCaptureWriter) observe(events []sse.Event) {
	if w == nil || w.fired || w.onDone == nil {
		return
	}
	for _, event := range events {
		if event.Data == "" || event.Data == "[DONE]" {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(event.Data), &payload) != nil {
			continue
		}
		eventType, _ := payload["type"].(string)
		if eventType == "" {
			eventType = event.Name
		}
		if eventType != "response.completed" && eventType != "response.incomplete" {
			continue
		}
		response, ok := payload["response"].(map[string]any)
		if !ok || response == nil {
			continue
		}
		w.fired = true
		w.onDone(response)
		return
	}
}
