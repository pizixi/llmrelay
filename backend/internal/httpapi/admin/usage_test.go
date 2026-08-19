package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"llmrelay/backend/internal/stats"
)

func TestAdminUsageHandlerDeletesRecord(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()
	stats.RecordUsage("chat", "primary", "gpt-test", 4, 2, 6)

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("prepare usage record: page=%#v err=%v", page, err)
	}
	id := page.Items[0].ID

	request := httptest.NewRequest(http.MethodDelete, "/api/usage?id="+strconv.FormatInt(id, 10), nil)
	recorder := httptest.NewRecorder()
	AdminUsageHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	after, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil || after.Total != 0 {
		t.Fatalf("usage remained after delete: page=%#v err=%v", after, err)
	}

	missingRecorder := httptest.NewRecorder()
	AdminUsageHandler(missingRecorder, httptest.NewRequest(http.MethodDelete, request.URL.String(), nil))
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing delete status = %d, body = %s", missingRecorder.Code, missingRecorder.Body.String())
	}
}

func TestAdminUsageHandlerRejectsInvalidDeleteID(t *testing.T) {
	recorder := httptest.NewRecorder()
	AdminUsageHandler(recorder, httptest.NewRequest(http.MethodDelete, "/api/usage?id=invalid", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid id status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
