package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"llmrelay/backend/internal/stats"
)

func TestServeNativeProtocolRecordsJSONResponseForStreamRequest(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const responseBody = `{"id":"chatcmpl_test","object":"chat.completion","usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	ServeNativeProtocol(response, NativeProxyRequest{
		Client:         WireChat,
		Body:           []byte(`{"model":"gpt-test","stream":true}`),
		UpstreamName:   "primary",
		Model:          "gpt-test",
		UsageModel:     "chat-alias",
		Stream:         true,
		Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
		RequestContext: context.Background(),
	})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != responseBody {
		t.Fatalf("response body = %q, want %q", response.Body.String(), responseBody)
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.RequestModel != "chat-alias" || item.UpstreamName != "primary" || item.UpstreamModel != "gpt-test" {
		t.Fatalf("usage identity = %#v", item)
	}
	if item.RequestCount != 1 || item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v", item)
	}
}
