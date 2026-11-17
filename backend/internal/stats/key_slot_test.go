package stats

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestUsageRecordsUpstreamKeySlotFromContext(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	handler := TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		RecordUpstreamKeySlot(r.Context(), 1, 3)
		usage := NewRequestUsageAccumulatorForContext(r.Context(), "chat", "primary", "gpt-test")
		usage.ObserveMap(map[string]any{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6})
		usage.Commit()
	})
	handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page.Items)
	}
	item := page.Items[0]
	if item.UpstreamKeyIndex != 2 || item.UpstreamKeyTotal != 3 {
		t.Fatalf("key slot = %d/%d, want 2/3", item.UpstreamKeyIndex, item.UpstreamKeyTotal)
	}
}

func TestUsageKeySlotIsZeroWithoutUpstreamKeyReporting(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	RecordUsage("chat", "primary", "gpt-test", 1, 1, 2)
	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list usage: page=%#v err=%v", page, err)
	}
	if page.Items[0].UpstreamKeyIndex != 0 || page.Items[0].UpstreamKeyTotal != 0 {
		t.Fatalf("key slot = %d/%d, want 0/0", page.Items[0].UpstreamKeyIndex, page.Items[0].UpstreamKeyTotal)
	}
}
