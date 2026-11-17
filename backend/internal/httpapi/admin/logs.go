package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"llmrelay/backend/internal/logs"
)

// AdminLogsHandler serves the live console log tab.
//
//   - GET /api/logs?limit=200[&after=123] returns buffered lines as JSON.
//   - GET /api/logs?stream=1 streams new lines as SSE events.
func AdminLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if r.URL.Query().Get("stream") == "1" {
		streamAdminLogs(w, r, after)
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"items": logs.Recent(limit, after),
	})
}

func streamAdminLogs(w http.ResponseWriter, r *http.Request, after int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
		return
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	enqueue := func(entry logs.Entry) bool {
		payload, err := json.Marshal(entry)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return false
		}
		return true
	}

	// Replay buffered lines first so a fresh page is not empty.
	for _, entry := range logs.Recent(200, after) {
		if !enqueue(entry) {
			return
		}
		after = entry.Seq
	}
	flusher.Flush()

	entries, cancel := logs.Subscribe()
	defer cancel()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case entry, open := <-entries:
			if !open {
				return
			}
			if entry.Seq <= after {
				continue
			}
			if !enqueue(entry) {
				return
			}
			after = entry.Seq
			flusher.Flush()
		}
	}
}
