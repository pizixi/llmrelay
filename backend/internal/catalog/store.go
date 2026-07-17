package catalog

import "llmrelay/backend/internal/domain"

type State struct {
	Models                []ModelInfo
	UpstreamCatalog       []ModelInfo
	ModelsLoaded          bool
	UpstreamCatalogLoaded bool
}

func SnapshotState() State {
	modelMu.RLock()
	defer modelMu.RUnlock()
	return State{
		Models:                append([]ModelInfo(nil), modelsCache...),
		UpstreamCatalog:       append([]ModelInfo(nil), upstreamModelCatalogCache...),
		ModelsLoaded:          modelsLoaded,
		UpstreamCatalogLoaded: upstreamModelCatalogLoaded,
	}
}

func RestoreState(state State) {
	modelMu.Lock()
	modelsCache = append([]ModelInfo(nil), state.Models...)
	upstreamModelCatalogCache = append([]ModelInfo(nil), state.UpstreamCatalog...)
	modelsLoaded = state.ModelsLoaded
	upstreamModelCatalogLoaded = state.UpstreamCatalogLoaded
	modelMu.Unlock()
}

// Initialize 使用配置中的模型初始化有效模型缓存，并清空上游同步目录。
func Initialize() []ModelInfo {
	models := ConfiguredModelsFromUpstreams()
	modelMu.Lock()
	modelsCache = append([]ModelInfo(nil), models...)
	modelsLoaded = true
	upstreamModelCatalogCache = nil
	upstreamModelCatalogLoaded = false
	modelMu.Unlock()
	return models
}

func ApplyUpstreamRefresh(name string, effective, catalog []ModelInfo) (int, int, int, int) {
	modelMu.Lock()
	modelsCache = ReplaceModelsForUpstream(modelsCache, name, effective)
	modelsLoaded = true
	upstreamModelCatalogCache = ReplaceModelsForUpstream(upstreamModelCatalogCache, name, catalog)
	upstreamModelCatalogLoaded = true
	effectiveCount := CountModelsForUpstream(modelsCache, name)
	catalogCount := CountModelsForUpstream(upstreamModelCatalogCache, name)
	totalEffective := len(modelsCache)
	totalCatalog := len(upstreamModelCatalogCache)
	modelMu.Unlock()
	return effectiveCount, catalogCount, totalEffective, totalCatalog
}

func ApplyEffective(models []ModelInfo) {
	modelMu.Lock()
	modelsCache = append([]ModelInfo(nil), models...)
	modelsLoaded = true
	modelMu.Unlock()
}

func ApplyCatalog(models []ModelInfo) {
	modelMu.Lock()
	upstreamModelCatalogCache = append([]ModelInfo(nil), models...)
	upstreamModelCatalogLoaded = true
	modelMu.Unlock()
}

func Counts() (int, int) {
	modelMu.RLock()
	defer modelMu.RUnlock()
	return len(modelsCache), len(upstreamModelCatalogCache)
}

// Reconfigure 在配置更新后重建有效模型，同时保留仍存在上游的目录缓存。
func Reconfigure(upstreams map[string]*domain.UpstreamConfig) {
	configured := ConfiguredModelsFromUpstreams()
	modelMu.Lock()
	modelsCache = configured
	upstreamModelCatalogCache = FilterModelsForUpstreams(upstreamModelCatalogCache, upstreams)
	modelsLoaded = true
	modelMu.Unlock()
}
