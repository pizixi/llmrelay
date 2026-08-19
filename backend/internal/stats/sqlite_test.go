package stats

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageAccumulatorAcceptsJSONNumbersAndNumericStrings(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	usage := NewRequestUsageAccumulator("chat", "primary", "gpt-test")
	usage.ObserveMap(map[string]any{
		"prompt_tokens":     json.Number("12"),
		"completion_tokens": "8",
		"total_tokens":      "20",
	})
	usage.Commit()

	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v, want prompt=12 completion=8 total=20", item)
	}
}

func TestSQLiteUsageRecordsAndDimensions(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	first := NewRequestUsageAccumulator(UsageIdentity("chat", "primary", "gpt-test"))
	first.ObserveMap(map[string]any{"prompt_tokens": float64(11), "completion_tokens": float64(7), "total_tokens": float64(18)})
	first.Commit()
	second := NewRequestUsageAccumulator("chat", "secondary", "gpt-test-2")
	second.ObserveMap(map[string]any{"input_tokens": float64(5), "output_tokens": float64(3)})
	second.Commit()

	snapshot := Snapshot()
	if snapshot.TotalRequests != 2 || snapshot.Models["chat"].TotalTokens != 26 {
		t.Fatalf("unexpected model summary: %#v", snapshot)
	}
	if snapshot.Upstreams["primary"].TotalTokens != 18 || snapshot.DailyUpstreams["secondary"].TotalTokens != 8 {
		t.Fatalf("unexpected upstream summary: %#v %#v", snapshot.Upstreams, snapshot.DailyUpstreams)
	}
	if len(snapshot.ModelUpstreams) != 2 || len(snapshot.Days) != 1 {
		t.Fatalf("unexpected dimensions: model_upstreams=%#v days=%#v", snapshot.ModelUpstreams, snapshot.Days)
	}

	page, err := ListUsageRecords(UsageQuery{Limit: 10, Upstream: "primary"})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].RequestModel != "chat" || page.Items[0].UpstreamModel != "gpt-test" {
		t.Fatalf("unexpected usage page: %#v", page)
	}

	Reset()
	if after := Snapshot(); after.TotalRequests != 0 || len(after.Models) != 0 {
		t.Fatalf("reset left usage behind: %#v", after)
	}
}

func TestDeleteUsageRecordUpdatesSummaries(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	RecordUsage("chat", "primary", "gpt-primary", 11, 7, 18)
	RecordUsage("responses", "secondary", "gpt-secondary", 5, 3, 8)

	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("usage records = %#v, want two records", page)
	}
	var deletedID int64
	for _, item := range page.Items {
		if item.UpstreamName == "primary" {
			deletedID = item.ID
			break
		}
	}
	if deletedID == 0 {
		t.Fatalf("primary usage record not found: %#v", page.Items)
	}

	deleted, err := DeleteUsageRecord(deletedID)
	if err != nil || !deleted {
		t.Fatalf("delete usage record: deleted=%v err=%v", deleted, err)
	}
	deleted, err = DeleteUsageRecord(deletedID)
	if err != nil || deleted {
		t.Fatalf("delete missing usage record: deleted=%v err=%v", deleted, err)
	}

	after, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage after delete: %v", err)
	}
	if after.Total != 1 || len(after.Items) != 1 || after.Items[0].UpstreamName != "secondary" {
		t.Fatalf("unexpected usage page after delete: %#v", after)
	}
	if after.Summary.RequestCount != 1 || after.Summary.TotalTokens != 8 {
		t.Fatalf("unexpected usage summary after delete: %#v", after.Summary)
	}
	if snapshot := Snapshot(); snapshot.TotalRequests != 1 || snapshot.Models["responses"].TotalTokens != 8 {
		t.Fatalf("unexpected snapshot after delete: %#v", snapshot)
	}
}

func TestTrackedRequestPersistsFirstByteAndDuration(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	handler := TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		usage := NewRequestUsageAccumulatorForContext(r.Context(), "chat", "primary", "gpt-test")
		usage.ObserveMap(map[string]any{"input_tokens": 2, "output_tokens": 3})
		usage.Commit()
		time.Sleep(3 * time.Millisecond)
		_, _ = w.Write([]byte("first"))
		time.Sleep(3 * time.Millisecond)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	response := httptest.NewRecorder()
	handler(response, request)

	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage items: got %d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.FirstByteMS < 2 {
		t.Fatalf("first byte duration: got %dms, want at least 2ms", item.FirstByteMS)
	}
	if item.DurationMS < item.FirstByteMS || item.DurationMS < 5 {
		t.Fatalf("total duration: got %dms, first byte %dms", item.DurationMS, item.FirstByteMS)
	}
}

func TestTrackedUsageIdentityCarriesRequestTiming(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { SetPath(previousPath) })
	LoadTokenStats()

	handler := TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		identity := UsageIdentityForContext(r.Context(), "responses", "secondary", "gpt-stream")
		usage := NewRequestUsageAccumulator(identity)
		usage.Commit()
		time.Sleep(2 * time.Millisecond)
		_, _ = w.Write([]byte("data: first\n\n"))
	})
	handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/responses", nil))

	page, err := ListUsageRecords(UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].FirstByteMS < 1 {
		t.Fatalf("stream timing was not persisted: %#v", page.Items)
	}
	if page.Items[0].UpstreamName != "secondary" || page.Items[0].UpstreamModel != "gpt-stream" {
		t.Fatalf("stream route was not persisted: %#v", page.Items[0])
	}
}
