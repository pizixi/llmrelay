package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
)

func TestBridgeCapabilitiesHandlerReportsMatrixAndProviderOverrides(t *testing.T) {
	previousPath := config.Path()
	previousConfig := currentConfig()
	t.Cleanup(func() {
		config.SetPath(previousPath)
		applyConfig(previousConfig)
	})

	config.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	applyConfig(AppConfig{
		ModelAlias:         map[string]domain.ModelAlias{},
		ReasoningEffortMap: map[string]string{},
		Upstreams: map[string]*UpstreamConfig{
			"declared": {
				BaseURL: "https://provider.example/v1",
				APIType: UpstreamOpenAI,
				Capabilities: map[string]bool{
					"streaming":         false,
					"hosted_web_search": true,
				},
			},
		},
		UpstreamOrder:   []string{"declared"},
		DefaultUpstream: "declared",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/bridge/capabilities", nil)
	BridgeCapabilitiesHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Matrix    map[string]map[string]map[string]string `json:"matrix"`
		Upstreams map[string]struct {
			Protocol     string          `json:"protocol"`
			Capabilities map[string]bool `json:"capabilities"`
		} `json:"upstreams"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Matrix["openai-chat"]["anthropic-messages"] == nil || response.Matrix["openai-chat"]["openai-responses"] == nil {
		t.Fatalf("matrix is missing protocol pairs: %#v", response.Matrix)
	}
	declared, ok := response.Upstreams["declared"]
	if !ok || declared.Protocol != "openai-chat" {
		t.Fatalf("provider declaration=%#v", response.Upstreams)
	}
	if declared.Capabilities["streaming"] || !declared.Capabilities["hosted_web_search"] {
		t.Fatalf("provider overrides were not preserved: %#v", declared.Capabilities)
	}
}

func TestBridgeCapabilitiesHandlerRejectsNonGet(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/bridge/capabilities", nil)
	BridgeCapabilitiesHandler(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
