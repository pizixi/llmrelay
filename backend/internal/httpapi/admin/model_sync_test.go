package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	catalogpkg "llmrelay/backend/internal/catalog"
	"llmrelay/backend/internal/domain"
)

func TestAdminSyncModelsHandlerReturnsIndependentUpstreamResults(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"model-z"},{"id":"model-a"},{"id":"model-a"}]}`))
		case "/bad/v1/models":
			http.Error(w, "catalog unavailable", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstreamServer.Close()

	previousConfig := currentConfig()
	previousCatalog := catalogpkg.SnapshotState()
	defer func() {
		applyConfig(previousConfig)
		catalogpkg.RestoreState(previousCatalog)
	}()

	cfg := AppConfig{
		ModelAlias:         map[string]domain.ModelAlias{},
		ReasoningEffortMap: map[string]string{},
		Upstreams: map[string]*UpstreamConfig{
			"good": {BaseURL: upstreamServer.URL + "/good/v1", APIType: UpstreamOpenAI},
			"bad":  {BaseURL: upstreamServer.URL + "/bad/v1", APIType: UpstreamOpenAI},
		},
		UpstreamOrder:   []string{"good", "bad"},
		DefaultUpstream: "good",
	}
	applyConfig(cfg)
	reconfigureCatalog(cfg.Upstreams)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/sync-models", nil)
	AdminSyncModelsHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Status    string                         `json:"status"`
		Succeeded int                            `json:"succeeded"`
		Failed    int                            `json:"failed"`
		Upstreams []adminUpstreamModelSyncResult `json:"upstreams"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "partial" || response.Succeeded != 1 || response.Failed != 1 {
		t.Fatalf("unexpected summary: %#v", response)
	}
	if len(response.Upstreams) != 2 {
		t.Fatalf("upstream results = %#v", response.Upstreams)
	}
	if got := response.Upstreams[0]; got.Upstream != "good" || len(got.Models) != 2 || got.Models[0] != "model-a" || got.Models[1] != "model-z" || got.Error != "" {
		t.Fatalf("good result = %#v", got)
	}
	if got := response.Upstreams[1]; got.Upstream != "bad" || got.Error == "" {
		t.Fatalf("bad result = %#v", got)
	}
}
