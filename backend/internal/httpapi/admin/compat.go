package admin

import (
	"context"
	"embed"
	"io"
	"net/http"
	"sync"

	"llmrelay/backend/internal/bridge/convert"
	bridgestream "llmrelay/backend/internal/bridge/stream"
	catalogpkg "llmrelay/backend/internal/catalog"
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/stats"
	"llmrelay/backend/internal/upstream"
	"llmrelay/backend/internal/websearch"
)

type AppConfig = domain.AppConfig
type ModelInfo = domain.ModelInfo
type ModelMappingCleanup = config.ModelMappingCleanup
type UpstreamConfig = domain.UpstreamConfig
type UpstreamType = domain.UpstreamType
type TokenStatsData = stats.TokenStatsData
type ModelStats = stats.ModelStats
type DailyStats = stats.DailyStats

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses
)

var (
	frontendFS     embed.FS
	configUpdateMu sync.Mutex
	debugMode      bool
)

func Configure(assets embed.FS, debug bool) {
	frontendFS = assets
	debugMode = debug
}

func fetchModelsForUpstream(name string, custom bool) ([]ModelInfo, error) {
	return catalogpkg.FetchModelsForUpstream(name, custom)
}

func fetchModels() ([]ModelInfo, error) { return catalogpkg.FetchModels() }

func fetchUpstreamModelCatalog() ([]ModelInfo, error) { return catalogpkg.FetchUpstreamModelCatalog() }

func getConfiguredUpstreamCount() int { return config.GetConfiguredUpstreamCount() }

func getConfiguredUpstreams() (map[string]*UpstreamConfig, string) {
	snapshot := config.Snapshot()
	return snapshot.Upstreams, snapshot.DefaultUpstream
}

func cloneUpstreamConfig(target *UpstreamConfig) *UpstreamConfig {
	return config.CloneUpstreamConfig(target)
}

func callPreparedUpstreamStream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig, raw ...bool) (io.ReadCloser, int, http.Header, error) {
	return upstream.CallPreparedUpstreamStream(ctx, body, upstreamName, model, target, raw...)
}

func anthropicStreamToChatHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, record bool) {
	bridgestream.AnthropicStreamToChatHandler(w, body, model, usageModel, record)
}

func responsesStreamToChatHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, record bool) {
	upstream.ResponsesStreamToChatHandler(w, body, model, usageModel, record)
}

func setSSEHeaders(header http.Header) { sse.SetHeaders(header) }

func emitOpenAIStreamError(w http.ResponseWriter, flusher http.Flusher, payload map[string]any, fallback string) {
	upstream.EmitOpenAIStreamError(w, flusher, payload, fallback)
}

func truncatePreview(value string, limit int) string { return convert.TruncatePreview(value, limit) }

func normalizeConfig(value *AppConfig) { config.NormalizeConfig(value) }

func validateConfig(value *AppConfig) error { return config.ValidateConfig(value) }

func reconcileRemovedUpstreamModels(previous AppConfig, next *AppConfig) ModelMappingCleanup {
	return config.ReconcileRemovedUpstreamModels(previous, next)
}

func saveConfig(path string, value AppConfig) error { return config.SaveConfig(path, value) }

func applyConfig(value AppConfig) { config.ApplyConfig(value) }

func currentConfig() AppConfig { return config.Snapshot() }

func configPath() string { return config.Path() }

func reconfigureCatalog(upstreams map[string]*UpstreamConfig) {
	catalogpkg.Reconfigure(upstreams)
}

func applyUpstreamCatalogRefresh(name string, models []ModelInfo) (int, int) {
	return catalogpkg.ApplyUpstreamCatalogRefresh(name, models)
}

func statsSnapshot() TokenStatsData { return stats.Snapshot() }

func resetStats() { stats.Reset() }

func configuredModelsFromUpstreams() []ModelInfo { return catalogpkg.ConfiguredModelsFromUpstreams() }

func getAdminAvailableModels() []ModelInfo { return catalogpkg.GetAdminAvailableModels() }

func groupModelsByUpstream(models []ModelInfo) map[string][]ModelInfo {
	return catalogpkg.GroupModelsByUpstream(models)
}

func getToday() string { return stats.GetToday() }

func saveTokenStats() { stats.SaveTokenStats() }

func searxngInstancesHandler(w http.ResponseWriter, r *http.Request) {
	websearch.SearxngInstancesHandler(w, r)
}
