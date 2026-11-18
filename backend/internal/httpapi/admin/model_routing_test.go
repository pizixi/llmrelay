package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"llmrelay/backend/internal/config"
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
