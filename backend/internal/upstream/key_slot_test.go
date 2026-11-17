package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/stats"
)

func TestSuccessfulUpstreamCallRecordsKeySlot(t *testing.T) {
	proxies, active := netproxy.Snapshot()
	netproxy.Configure(nil, "")
	t.Cleanup(func() { netproxy.Configure(proxies, active) })

	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	var seenKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	target := &UpstreamConfig{
		BaseURL: server.URL,
		APIKey:  "key-1\nkey-2\nkey-3",
		APIType: UpstreamOpenAI,
	}
	handler := stats.TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		if _, _, _, err := CallPreparedUpstream(r.Context(), []byte(`{"model":"test"}`), "primary", "test", target, true); err != nil {
			t.Errorf("call upstream: %v", err)
			return
		}
		usage := stats.NewRequestUsageAccumulatorForContext(r.Context(), "alias", "primary", "test")
		usage.ObserveMap(map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2})
		usage.Commit()
	})
	handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if seenKey == "" {
		t.Fatalf("upstream did not receive a credential")
	}
	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list usage: page=%#v err=%v", page, err)
	}
	item := page.Items[0]
	if item.UpstreamKeyTotal != 3 {
		t.Fatalf("key total = %d, want 3", item.UpstreamKeyTotal)
	}
	if item.UpstreamKeyIndex < 1 || item.UpstreamKeyIndex > 3 {
		t.Fatalf("key index = %d, want 1..3", item.UpstreamKeyIndex)
	}
}

func TestSuccessfulStreamCallRecordsKeySlot(t *testing.T) {
	proxies, active := netproxy.Snapshot()
	netproxy.Configure(nil, "")
	t.Cleanup(func() { netproxy.Configure(proxies, active) })

	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	target := &UpstreamConfig{BaseURL: server.URL, APIKey: "a\nb", APIType: UpstreamOpenAI}
	handler := stats.TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		reader, _, _, err := CallPreparedUpstreamStream(r.Context(), []byte(`{"model":"test","stream":true}`), "primary", "test", target)
		if err != nil {
			t.Errorf("stream upstream: %v", err)
			return
		}
		_, _ = io.Copy(io.Discard, reader)
		_ = reader.Close()
		usage := stats.NewRequestUsageAccumulatorForContext(r.Context(), "alias", "primary", "test")
		usage.ObserveMap(map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2})
		usage.Commit()
	})
	handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list usage: page=%#v err=%v", page, err)
	}
	if page.Items[0].UpstreamKeyTotal != 2 || page.Items[0].UpstreamKeyIndex == 0 {
		t.Fatalf("key slot = %d/%d, want set with total 2", page.Items[0].UpstreamKeyIndex, page.Items[0].UpstreamKeyTotal)
	}
}
