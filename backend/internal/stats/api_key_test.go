package stats

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/domain"
)

func TestTrackedUsageCarriesAPIKeyAndFilteredSummary(t *testing.T) {
	previousPath := Path()
	SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() {
		SetPath(previousPath)
		auth.Configure("", "")
	})
	LoadTokenStats()
	auth.Configure("", "")
	auth.SetAPIKeys([]domain.APIKey{{ID: "key-prod", Name: "生产环境", Key: "secret-prod"}})

	handler := auth.RequireAPIAuth(TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		usage := NewRequestUsageAccumulatorForContext(r.Context(), "chat", "primary", "gpt-test")
		usage.ObserveMap(map[string]any{"input_tokens": 11, "output_tokens": 7})
		usage.Commit()
		_, _ = w.Write([]byte("ok"))
	}))
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.Header.Set("Authorization", "Bearer secret-prod")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	page, err := ListUsageRecords(UsageQuery{Limit: 10, APIKeyName: "生产环境"})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	item := page.Items[0]
	if item.APIKeyID != "key-prod" || item.APIKeyName != "生产环境" {
		t.Fatalf("api key identity = %#v", item)
	}
	if page.Summary.RequestCount != 1 || page.Summary.PromptTokens != 11 || page.Summary.CompletionTokens != 7 || page.Summary.TotalTokens != 18 {
		t.Fatalf("unexpected summary: %#v", page.Summary)
	}
}
