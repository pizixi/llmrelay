// Package catalog 管理可路由模型列表和上游模型目录缓存。
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/upstream"
)

type ModelInfo = domain.ModelInfo
type UpstreamConfig = domain.UpstreamConfig

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
)

var (
	modelsCache                []ModelInfo
	upstreamModelCatalogCache  []ModelInfo
	modelMu                    sync.RWMutex
	modelsLoaded               bool
	upstreamModelCatalogLoaded bool
)

func EffectiveUpstreamName(name string) string { return upstream.EffectiveUpstreamName(name) }
func GetUpstreamModelsEndpoint(target *UpstreamConfig) string {
	return upstream.GetUpstreamModelsEndpoint(target)
}
func GetUpstreamAPIKeys(target *UpstreamConfig) []string { return upstream.GetUpstreamAPIKeys(target) }
func NextUpstreamAPIKeyIndex(name string, total int) int {
	return upstream.NextUpstreamAPIKeyIndex(name, total)
}
func ShouldRetryUpstreamStatus(status int) bool { return upstream.ShouldRetryUpstreamStatus(status) }
func FormatUpstreamAPIKeySlot(index, total int) string {
	return upstream.FormatUpstreamAPIKeySlot(index, total)
}
func GetHTTPClient(stream bool) *http.Client { return netproxy.Client(stream) }
func CloneUpstreamConfig(target *UpstreamConfig) *UpstreamConfig {
	return config.CloneUpstreamConfig(target)
}
func SortedUpstreamNames(values map[string]*UpstreamConfig) []string {
	return config.SortedUpstreamNames(values)
}
func GetConfiguredUpstreams() (map[string]*UpstreamConfig, string) {
	snapshot := config.Snapshot()
	return snapshot.Upstreams, snapshot.DefaultUpstream
}
func FetchModelsFromUpstream(name string, cfg *UpstreamConfig, useCustomModels bool) ([]ModelInfo, error) {
	if cfg == nil || cfg.BaseURL == "" {
		return []ModelInfo{}, nil
	}
	ownedBy := EffectiveUpstreamName(name)
	if useCustomModels && len(cfg.CustomModels) > 0 {
		var models []ModelInfo
		now := time.Now().Unix()
		for _, m := range cfg.CustomModels {
			models = append(models, ModelInfo{ID: m, Object: "model", Created: now, OwnedBy: ownedBy})
		}
		return models, nil
	}
	models, err := FetchModelsFromUpstreamOnce(name, cfg)
	if err == nil {
		return models, nil
	}
	fallback, ok := OpenAIModelSyncFallback(cfg)
	if !ok {
		return nil, err
	}
	log.Printf("上游 %s 使用 %s 同步模型失败，尝试通过 OpenAI 接口 %s 同步: %v", EffectiveUpstreamName(name), cfg.APIType, GetUpstreamModelsEndpoint(fallback), err)
	models, fallbackErr := FetchModelsFromUpstreamOnce(name, fallback)
	if fallbackErr != nil {
		return nil, fmt.Errorf("native model sync failed: %v; OpenAI /v1 fallback failed: %w", err, fallbackErr)
	}
	return models, nil
}

func OpenAIModelSyncFallback(cfg *UpstreamConfig) (*UpstreamConfig, bool) {
	if cfg == nil || cfg.APIType == "" || cfg.APIType == UpstreamOpenAI {
		return nil, false
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" || strings.HasSuffix(baseURL, "/v1") {
		return nil, false
	}
	fallback := CloneUpstreamConfig(cfg)
	fallback.BaseURL = baseURL + "/v1"
	fallback.APIType = UpstreamOpenAI
	return fallback, true
}

func FetchModelsFromUpstreamOnce(name string, cfg *UpstreamConfig) ([]ModelInfo, error) {
	ownedBy := EffectiveUpstreamName(name)
	endpoint := GetUpstreamModelsEndpoint(cfg)
	apiKeys := GetUpstreamAPIKeys(cfg)
	if len(apiKeys) == 0 {
		apiKeys = []string{""}
	}
	start := NextUpstreamAPIKeyIndex(name, len(apiKeys))
	var lastErr error
	for i := 0; i < len(apiKeys); i++ {
		apiKeyIndex := (start + i) % len(apiKeys)
		apiKey := apiKeys[apiKeyIndex]
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return nil, err
		}
		if cfg.APIType == UpstreamAnthropic {
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("anthropic-beta", "prompt-caching-2025-01-31")
			if apiKey != "" {
				req.Header.Set("x-api-key", apiKey)
				req.Header.Set("Authorization", "Bearer "+apiKey)
			}
		} else if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := GetHTTPClient(false).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var result struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, err
			}
			var models []ModelInfo
			now := time.Now().Unix()
			for _, m := range result.Data {
				models = append(models, ModelInfo{ID: m.ID, Object: "model", Created: now, OwnedBy: ownedBy})
			}
			return models, nil
		}
		if ShouldRetryUpstreamStatus(resp.StatusCode) && len(apiKeys) > 1 {
			lastErr = fmt.Errorf("models endpoint retryable status %d on key %s", resp.StatusCode, FormatUpstreamAPIKeySlot(apiKeyIndex, len(apiKeys)))
			continue
		}
		lastErr = fmt.Errorf("models endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		break
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("models endpoint request failed")
	}
	return nil, lastErr
}

func FetchModelsWithMode(useCustomModels bool) ([]ModelInfo, error) {
	upstreams, _ := GetConfiguredUpstreams()
	if len(upstreams) == 0 {
		return []ModelInfo{}, nil
	}
	var merged []ModelInfo
	var errs []string
	for _, name := range SortedUpstreamNames(upstreams) {
		models, err := FetchModelsFromUpstream(name, upstreams[name], useCustomModels)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		merged = append(merged, models...)
	}
	if len(merged) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return merged, nil
}

func FetchModels() ([]ModelInfo, error) {
	return FetchModelsWithMode(true)
}

func FetchUpstreamModelCatalog() ([]ModelInfo, error) {
	return FetchModelsWithMode(false)
}

func FetchModelsForUpstream(name string, useCustomModels bool) ([]ModelInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("upstream name is required")
	}
	upstreams, _ := GetConfiguredUpstreams()
	cfg := upstreams[name]
	if cfg == nil {
		return nil, fmt.Errorf("upstream %q not found", name)
	}
	return FetchModelsFromUpstream(name, cfg, useCustomModels)
}

func ConfiguredModelsFromUpstreams() []ModelInfo {
	upstreams, _ := GetConfiguredUpstreams()
	if len(upstreams) == 0 {
		return []ModelInfo{}
	}
	now := time.Now().Unix()
	var models []ModelInfo
	for _, name := range SortedUpstreamNames(upstreams) {
		cfg := upstreams[name]
		if cfg == nil {
			continue
		}
		ownedBy := EffectiveUpstreamName(name)
		for _, model := range cfg.CustomModels {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			models = append(models, ModelInfo{ID: model, Object: "model", Created: now, OwnedBy: ownedBy})
		}
	}
	return models
}

func GetModelIDs() []string {
	modelMu.RLock()
	defer modelMu.RUnlock()
	ids := make([]string, len(modelsCache))
	for i, m := range modelsCache {
		ids[i] = m.ID
	}
	return ids
}

func GetAliasModelInfos() []ModelInfo {
	snapshot := config.Snapshot()
	if len(snapshot.ModelAlias) == 0 {
		return []ModelInfo{}
	}
	names := make([]string, 0, len(snapshot.ModelAlias))
	for name := range snapshot.ModelAlias {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	now := time.Now().Unix()
	models := make([]ModelInfo, 0, len(names))
	for _, name := range names {
		models = append(models, ModelInfo{
			ID:      name,
			Object:  "model",
			Created: now,
			OwnedBy: "alias",
		})
	}
	return models
}

// GetRoutableModelInfos 与请求路由保持一致：将别名作为公开模型名，
// 同时包含可直接访问默认上游的模型。两者同名时别名优先。
func GetRoutableModelInfos() []ModelInfo {
	snapshot := config.Snapshot()
	now := time.Now().Unix()
	byID := make(map[string]ModelInfo)
	if defaultUpstream := snapshot.Upstreams[snapshot.DefaultUpstream]; defaultUpstream != nil {
		for _, rawModel := range defaultUpstream.CustomModels {
			model := strings.TrimSpace(rawModel)
			if model == "" {
				continue
			}
			byID[model] = ModelInfo{
				ID:      model,
				Object:  "model",
				Created: now,
				OwnedBy: EffectiveUpstreamName(snapshot.DefaultUpstream),
			}
		}
	}
	for rawModel := range snapshot.ModelAlias {
		model := strings.TrimSpace(rawModel)
		if model == "" {
			continue
		}
		byID[model] = ModelInfo{
			ID:      model,
			Object:  "model",
			Created: now,
			OwnedBy: "alias",
		}
	}

	models := make([]ModelInfo, 0, len(byID))
	for _, model := range byID {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models
}

func GroupModelsByUpstream(models []ModelInfo) map[string][]ModelInfo {
	grouped := map[string][]ModelInfo{}
	for _, model := range models {
		name := EffectiveUpstreamName(model.OwnedBy)
		grouped[name] = append(grouped[name], model)
	}
	for name := range grouped {
		sort.Slice(grouped[name], func(i, j int) bool {
			return grouped[name][i].ID < grouped[name][j].ID
		})
	}
	return grouped
}

func ReplaceModelsForUpstream(existing []ModelInfo, upstreamName string, models []ModelInfo) []ModelInfo {
	owner := EffectiveUpstreamName(upstreamName)
	merged := make([]ModelInfo, 0, len(existing)+len(models))
	for _, model := range existing {
		if EffectiveUpstreamName(model.OwnedBy) != owner {
			merged = append(merged, model)
		}
	}
	merged = append(merged, models...)
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].OwnedBy == merged[j].OwnedBy {
			return merged[i].ID < merged[j].ID
		}
		return merged[i].OwnedBy < merged[j].OwnedBy
	})
	return merged
}

func CountModelsForUpstream(models []ModelInfo, upstreamName string) int {
	owner := EffectiveUpstreamName(upstreamName)
	count := 0
	for _, model := range models {
		if EffectiveUpstreamName(model.OwnedBy) == owner {
			count++
		}
	}
	return count
}

func FilterModelsForUpstreams(models []ModelInfo, upstreams map[string]*UpstreamConfig) []ModelInfo {
	allowed := map[string]struct{}{}
	for name := range upstreams {
		allowed[EffectiveUpstreamName(name)] = struct{}{}
	}
	filtered := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		if _, ok := allowed[EffectiveUpstreamName(model.OwnedBy)]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func GetAdminAvailableModels() []ModelInfo {
	modelMu.RLock()
	defer modelMu.RUnlock()
	if upstreamModelCatalogLoaded {
		return append([]ModelInfo(nil), upstreamModelCatalogCache...)
	}
	return append([]ModelInfo(nil), modelsCache...)
}
