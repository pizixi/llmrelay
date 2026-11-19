package admin

import (
	"net/http"

	"llmrelay/backend/internal/routing/health"
	"llmrelay/backend/internal/upstream"
)

type upstreamHealth struct {
	KeyCount int                           `json:"key_count"`
	Models   map[string][]health.KeyStatus `json:"models"`
}

func AdminRoutingHealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := currentConfig()
	enabled := !cfg.ModelRouting.DisableCooldown
	upstreams := make(map[string]upstreamHealth, len(cfg.Upstreams))
	for name, target := range cfg.Upstreams {
		keys := upstream.GetUpstreamAPIKeys(target)
		status := upstreamHealth{KeyCount: len(keys), Models: map[string][]health.KeyStatus{}}
		if enabled {
			status.Models = health.UpstreamStatuses(name, target, keys)
		}
		upstreams[name] = status
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"enabled": enabled, "upstreams": upstreams,
	})
}
