package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/routing/health"
)

func TestAdminModelRoutingConfigPreservationAndValidation(t *testing.T) {
	previousPath, previous := config.Path(), config.Snapshot()
	t.Cleanup(func() { config.SetPath(previousPath); config.ApplyConfig(previous) })
	config.SetPath(filepath.Join(t.TempDir(), "config.db"))
	want := config.ModelRoutingConfig{Strategy: config.RoutingWeightedRoundRobin, MaxAttempts: 3, FailureThreshold: 2, CooldownSeconds: 120}
	config.ApplyConfig(config.AppConfig{ModelRouting: want})
	for _, body := range []string{
		`{"model_alias":{},"upstreams":{}}`,
		`{"model_alias":{},"upstreams":{},"model_routing":{"strategy":"priority","disable_failover":true,"disable_cooldown":true,"max_attempts":2,"failure_threshold":4,"cooldown_seconds":90}}`,
	} {
		if strings.Contains(body, "model_routing") {
			want = config.ModelRoutingConfig{Strategy: config.RoutingPriority, DisableFailover: true, DisableCooldown: true, MaxAttempts: 2, FailureThreshold: 4, CooldownSeconds: 90}
		}
		response := httptest.NewRecorder()
		AdminConfigHandler(response, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body)))
		if response.Code != http.StatusOK {
			t.Fatalf("save status=%d body=%s", response.Code, response.Body.String())
		}
		persisted, err := config.LoadConfig(config.Path())
		if err != nil || persisted.ModelRouting != want || config.Snapshot().ModelRouting != want {
			t.Fatalf("saved policy=%#v, want=%#v, err=%v", persisted.ModelRouting, want, err)
		}
		response = httptest.NewRecorder()
		AdminConfigHandler(response, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		var loaded struct {
			ModelRouting config.ModelRoutingConfig `json:"model_routing"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &loaded); err != nil || loaded.ModelRouting != want {
			t.Fatalf("returned policy=%#v err=%v", loaded.ModelRouting, err)
		}
	}
	response := httptest.NewRecorder()
	AdminConfigHandler(response, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"model_routing":{"strategy":"invalid"}}`)))
	if response.Code != http.StatusBadRequest || config.Snapshot().ModelRouting != want {
		t.Fatalf("invalid policy status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminRoutingHealthReportsSlotsAndCountdown(t *testing.T) {
	previous := config.Snapshot()
	t.Cleanup(func() { config.ApplyConfig(previous) })
	config.ApplyConfig(config.AppConfig{
		ModelRouting: config.ModelRoutingConfig{FailureThreshold: 1, CooldownSeconds: 95},
		Upstreams: map[string]*config.UpstreamConfig{
			"routing-health-test": {BaseURL: "https://cooldown.example/v1", APIKey: "private-failed\nprivate-working"},
		},
	})
	cfg := config.Snapshot()
	target := health.NewTarget("routing-health-test", "actual-model", cfg.Upstreams["routing-health-test"])
	attempt, _ := target.TryKey("private-failed", cfg.ModelRouting)
	if attempt == nil {
		t.Fatal("test credential unexpectedly unavailable")
	}
	failed := health.ClassifyFailure(401, nil)
	attempt.Complete(&failed)
	response := httptest.NewRecorder()
	AdminRoutingHealthHandler(response, httptest.NewRequest(http.MethodGet, "/api/routing/health", nil))
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private-") {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var result struct {
		Enabled   bool                      `json:"enabled"`
		Upstreams map[string]upstreamHealth `json:"upstreams"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	status := result.Upstreams["routing-health-test"]
	keys := status.Models["actual-model"]
	if !result.Enabled || status.KeyCount != 2 || len(keys) != 1 {
		t.Fatalf("status=%#v", result)
	}
	if key := keys[0]; key.Slot != 1 || key.State != "cooling" || key.Failures != 1 || key.RemainingMS <= 0 || key.RemainingMS > 95000 {
		t.Fatalf("key=%#v", key)
	}
	cfg.ModelRouting.DisableCooldown = true
	config.ApplyConfig(cfg)
	response = httptest.NewRecorder()
	AdminRoutingHealthHandler(response, httptest.NewRequest(http.MethodGet, "/api/routing/health", nil))
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Enabled || len(result.Upstreams["routing-health-test"].Models) != 0 {
		t.Fatalf("disabled cooldown status=%s err=%v", response.Body, err)
	}
}
