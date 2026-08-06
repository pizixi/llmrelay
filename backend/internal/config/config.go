package config

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"llmrelay/backend/internal/storage"
)

// ======================== 配置 ========================

var (
	configPath = "llmrelay.db"
	modelAlias = map[string]ModelAlias{}

	reasoningEffortMap  = map[string]string{}
	webSearchCfg        WebSearchConfig
	apiKeys             []APIKey
	upstreamOrder       []string
	legacyDefaultRoute  bool
	configMu            sync.RWMutex
	aliasTargetCounters sync.Map
	modelRouteCounters  sync.Map
)

// ======================== 配置管理 ========================

func LoadConfig(path string) (AppConfig, error) {
	if !storage.IsSQLitePath(path) {
		return AppConfig{}, fmt.Errorf("configuration must be stored in SQLite; JSON config files are no longer supported")
	}
	return loadSQLiteConfig(path)
}

func ValidateConfig(cfg *AppConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	seenAPIKeyIDs := make(map[string]struct{}, len(cfg.APIKeys))
	seenAPIKeys := make(map[string]struct{}, len(cfg.APIKeys))
	seenAPIKeyNames := make(map[string]struct{}, len(cfg.APIKeys))
	seenUpstreamIDs := make(map[int64]string, len(cfg.Upstreams))
	for _, apiKey := range cfg.APIKeys {
		key := strings.TrimSpace(apiKey.Key)
		if key == "" {
			return fmt.Errorf("api key must not be empty")
		}
		if id := strings.TrimSpace(apiKey.ID); id != "" {
			if _, exists := seenAPIKeyIDs[id]; exists {
				return fmt.Errorf("duplicate api key id %q", id)
			}
			seenAPIKeyIDs[id] = struct{}{}
		}
		if _, exists := seenAPIKeys[key]; exists {
			return fmt.Errorf("duplicate api key")
		}
		seenAPIKeys[key] = struct{}{}
		if name := strings.TrimSpace(apiKey.Name); name != "" {
			nameKey := strings.ToLower(name)
			if _, exists := seenAPIKeyNames[nameKey]; exists {
				return fmt.Errorf("duplicate api key name %q", name)
			}
			seenAPIKeyNames[nameKey] = struct{}{}
		}
	}
	validType := func(apiType UpstreamType) bool {
		return apiType == "" || apiType == UpstreamOpenAI || apiType == UpstreamAnthropic || apiType == UpstreamResponses
	}
	validBridgeMode := func(mode BridgeMode) bool {
		value := strings.TrimSpace(string(mode))
		return value == "" || strings.EqualFold(value, string(BridgeModeCompatible)) || strings.EqualFold(value, string(BridgeModeStrict))
	}
	for name, upstream := range cfg.Upstreams {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("upstream name must not be empty")
		}
		if upstream == nil || strings.TrimSpace(upstream.BaseURL) == "" {
			return fmt.Errorf("upstream %q must have base_url", name)
		}
		if upstream.ID < 0 {
			return fmt.Errorf("upstream %q has invalid id %d", name, upstream.ID)
		}
		if upstream.ID > 0 {
			if previousName, exists := seenUpstreamIDs[upstream.ID]; exists {
				return fmt.Errorf("duplicate upstream id %d for %q and %q", upstream.ID, previousName, name)
			}
			seenUpstreamIDs[upstream.ID] = name
		}
		if !validType(upstream.APIType) {
			return fmt.Errorf("upstream %q has unsupported api_type %q", name, upstream.APIType)
		}
		if !validBridgeMode(upstream.BridgeMode) {
			return fmt.Errorf("upstream %q has unsupported bridge_mode %q", name, upstream.BridgeMode)
		}
		proxyAddress := strings.TrimSpace(upstream.Proxy)
		if strings.EqualFold(proxyAddress, "direct") {
			proxyAddress = ""
		}
		if proxyAddress != "" {
			proxyConfigured := false
			for _, proxy := range cfg.Socks5Proxies {
				if strings.TrimSpace(proxy.Addr) == proxyAddress || strings.TrimSpace(proxy.Name) == proxyAddress {
					proxyConfigured = true
					break
				}
			}
			if !proxyConfigured {
				return fmt.Errorf("upstream %q references unknown socks5 proxy %q", name, proxyAddress)
			}
		}
	}
	if cfg.Upstream != nil && !validType(cfg.Upstream.APIType) {
		return fmt.Errorf("legacy upstream has unsupported api_type %q", cfg.Upstream.APIType)
	}
	if cfg.Upstream != nil && !validBridgeMode(cfg.Upstream.BridgeMode) {
		return fmt.Errorf("legacy upstream has unsupported bridge_mode %q", cfg.Upstream.BridgeMode)
	}
	defaultName := strings.TrimSpace(cfg.DefaultUpstream)
	if defaultName != "" && len(cfg.Upstreams) > 0 && cfg.Upstreams[defaultName] == nil {
		return fmt.Errorf("default_upstream %q does not exist", defaultName)
	}
	upstreamExists := func(name string) bool {
		if len(cfg.Upstreams) == 0 && cfg.Upstream != nil && name == "default" {
			return true
		}
		return cfg.Upstreams[name] != nil
	}
	for model, alias := range cfg.ModelAlias {
		if len(alias.Targets) == 0 {
			upstreamName := strings.TrimSpace(alias.Upstream)
			if upstreamName != "" && !upstreamExists(upstreamName) {
				return fmt.Errorf("model alias %q references unknown upstream %q", model, upstreamName)
			}
			continue
		}
		seenTargets := make(map[string]struct{}, len(alias.Targets))
		hasRoutableTarget := false
		for index, target := range alias.Targets {
			upstreamName := strings.TrimSpace(target.Upstream)
			targetModel := strings.TrimSpace(target.TargetModel)
			if upstreamName == "" || targetModel == "" {
				return fmt.Errorf("model alias %q target %d must have upstream and target_model", model, index+1)
			}
			if !upstreamExists(upstreamName) {
				return fmt.Errorf("model alias %q references unknown upstream %q", model, upstreamName)
			}
			if target.Weight < 0 || target.Weight > 1000000 {
				return fmt.Errorf("model alias %q target %d weight must be between 0 and 1000000", model, index+1)
			}
			if target.Weight > 0 {
				hasRoutableTarget = true
			}
			key := upstreamName + "\x00" + targetModel
			if _, exists := seenTargets[key]; exists {
				return fmt.Errorf("model alias %q has duplicate target %q on upstream %q", model, targetModel, upstreamName)
			}
			seenTargets[key] = struct{}{}
		}
		if !hasRoutableTarget {
			return fmt.Errorf("model alias %q must have at least one target with a weight greater than 0", model)
		}
	}
	if err := ValidateWebSearchConfig(cfg.WebSearch); err != nil {
		return err
	}
	return nil
}

func NormalizeConfig(cfg *AppConfig) {
	explicitLegacyDefault := strings.TrimSpace(cfg.DefaultUpstream) != ""
	cfg.LegacyDefaultUpstream = explicitLegacyDefault
	cfg.APIKeys = NormalizeAPIKeys(cfg.APIKeys)
	if cfg.ModelAlias == nil {
		cfg.ModelAlias = map[string]ModelAlias{}
	}
	for key, alias := range cfg.ModelAlias {
		trimmedKey := strings.TrimSpace(key)
		alias.TargetModel = strings.TrimSpace(alias.TargetModel)
		alias.Upstream = strings.TrimSpace(alias.Upstream)
		if len(alias.Targets) > 0 {
			targets := make([]ModelAliasTarget, 0, len(alias.Targets))
			seen := make(map[string]struct{}, len(alias.Targets))
			for _, target := range alias.Targets {
				target.TargetModel = strings.TrimSpace(target.TargetModel)
				target.Upstream = strings.TrimSpace(target.Upstream)
				if target.TargetModel == "" || target.Upstream == "" {
					continue
				}
				identity := target.Upstream + "\x00" + target.TargetModel
				if _, exists := seen[identity]; exists {
					continue
				}
				seen[identity] = struct{}{}
				targets = append(targets, target)
			}
			alias.Targets = targets
			if len(targets) > 0 {
				alias.TargetModel = ""
				alias.Upstream = ""
			}
		}
		alias.ReasoningEffortMap = NormalizeReasoningEffortMap(alias.ReasoningEffortMap)
		if trimmedKey == "" {
			delete(cfg.ModelAlias, key)
			continue
		}
		if trimmedKey != key {
			delete(cfg.ModelAlias, key)
		}
		cfg.ModelAlias[trimmedKey] = alias
	}

	if cfg.ReasoningEffortMap == nil {
		cfg.ReasoningEffortMap = map[string]string{}
	} else {
		cfg.ReasoningEffortMap = NormalizeReasoningEffortMap(cfg.ReasoningEffortMap)
	}
	cfg.WebSearch = NormalizeWebSearchConfig(cfg.WebSearch)
	NormalizeSocks5Config(cfg)
	if cfg.Upstreams == nil {
		cfg.Upstreams = map[string]*UpstreamConfig{}
	}
	legacy := CloneUpstreamConfig(cfg.Upstream)
	legacyValid := NormalizeSingleUpstream(legacy)
	normalizedUpstreams := make(map[string]*UpstreamConfig, len(cfg.Upstreams))
	for name, upstream := range cfg.Upstreams {
		trimmedName := strings.TrimSpace(name)
		copied := CloneUpstreamConfig(upstream)
		if trimmedName == "" || !NormalizeSingleUpstream(copied) {
			continue
		}
		normalizedUpstreams[trimmedName] = copied
	}
	cfg.Upstreams = normalizedUpstreams
	// DefaultUpstream is no longer generated from order. It is normalized only
	// for old callers that still carry the deprecated field; model routing below
	// never uses it to select a configured model.
	cfg.DefaultUpstream = strings.TrimSpace(cfg.DefaultUpstream)
	if len(cfg.Upstreams) == 0 && legacyValid {
		cfg.Upstreams["default"] = legacy
		if cfg.DefaultUpstream == "" {
			cfg.DefaultUpstream = "default"
			cfg.LegacyDefaultUpstream = true
		}
	}
	cfg.UpstreamOrder = NormalizeUpstreamOrder(cfg.UpstreamOrder, cfg.Upstreams)
	if len(cfg.Upstreams) == 0 {
		cfg.DefaultUpstream = ""
		cfg.LegacyDefaultUpstream = false
		cfg.Upstream = nil
		return
	}
	if cfg.DefaultUpstream == "" {
		// Keep the aggregate shape backwards compatible for old callers that
		// inspect NormalizeConfig output. The runtime flag below remains false,
		// so this value is not used as a routing decision or persisted as a
		// default selector.
		if len(cfg.UpstreamOrder) > 0 {
			cfg.DefaultUpstream = cfg.UpstreamOrder[0]
		}
	}
	cfg.Upstream = nil
}

func NormalizeReasoningEffortMap(effortMap map[string]string) map[string]string {
	if effortMap == nil {
		return nil
	}
	result := make(map[string]string, len(effortMap))
	for requestValue, mappedValue := range effortMap {
		requestValue = strings.TrimSpace(requestValue)
		if requestValue == "" {
			continue
		}
		result[requestValue] = strings.TrimSpace(mappedValue)
	}
	return result
}

func NormalizeUpstreamOrder(order []string, upstreams map[string]*UpstreamConfig) []string {
	if len(upstreams) == 0 {
		return nil
	}
	result := make([]string, 0, len(upstreams))
	seen := make(map[string]struct{}, len(upstreams))
	for _, name := range order {
		name = strings.TrimSpace(name)
		if upstreams[name] == nil {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	for _, name := range SortedUpstreamNames(upstreams) {
		if _, exists := seen[name]; exists {
			continue
		}
		result = append(result, name)
	}
	return result
}

func NormalizeSocks5Config(cfg *AppConfig) {
	if cfg.Socks5Proxies != nil {
		cleaned := make([]Socks5Proxy, 0, len(cfg.Socks5Proxies))
		seen := map[string]struct{}{}
		for _, proxy := range cfg.Socks5Proxies {
			proxy.Addr = strings.TrimSpace(proxy.Addr)
			proxy.Username = strings.TrimSpace(proxy.Username)
			proxy.Password = strings.TrimSpace(proxy.Password)
			proxy.Name = strings.TrimSpace(proxy.Name)
			if proxy.Addr == "" {
				continue
			}
			if _, ok := seen[proxy.Addr]; ok {
				continue
			}
			seen[proxy.Addr] = struct{}{}
			cleaned = append(cleaned, proxy)
		}
		cfg.Socks5Proxies = cleaned
	}
	cfg.ActiveSocks5 = strings.TrimSpace(cfg.ActiveSocks5)
	if cfg.ActiveSocks5 == "" || cfg.ActiveSocks5 == socks5RR || cfg.ActiveSocks5 == socks5RateLimitSwitch || cfg.ActiveSocks5 == socks5RateLimitSwitchNoDirect {
		return
	}
	for _, proxy := range cfg.Socks5Proxies {
		if proxy.Addr == cfg.ActiveSocks5 {
			return
		}
	}
	cfg.ActiveSocks5 = ""
}

func SaveConfig(path string, cfg AppConfig) error {
	if !storage.IsSQLitePath(path) {
		return fmt.Errorf("configuration must be stored in SQLite; JSON config files are no longer supported")
	}
	return saveSQLiteConfig(path, cfg)
}

func ApplyConfig(cfg AppConfig) {
	configMu.Lock()
	defer configMu.Unlock()
	if cfg.ModelAlias != nil {
		modelAlias = make(map[string]ModelAlias, len(cfg.ModelAlias))
		for name, alias := range cfg.ModelAlias {
			modelAlias[name] = CloneModelAlias(alias)
		}
		aliasTargetCounters.Range(func(key, _ any) bool {
			aliasTargetCounters.Delete(key)
			return true
		})
	}
	apiKeys = CloneAPIKeys(cfg.APIKeys)

	if cfg.ReasoningEffortMap != nil {
		reasoningEffortMap = cfg.ReasoningEffortMap
	}
	webSearchCfg = cfg.WebSearch
	ResetHostedWebSearchCapabilityCache()
	upstreamCfgs = make(map[string]*UpstreamConfig, len(cfg.Upstreams))
	for name, upstream := range cfg.Upstreams {
		upstreamCfgs[name] = CloneUpstreamConfig(upstream)
	}
	upstreamOrder = append([]string(nil), cfg.UpstreamOrder...)
	legacyDefaultRoute = cfg.LegacyDefaultUpstream
	if legacyDefaultRoute {
		defaultUpstreamName = strings.TrimSpace(cfg.DefaultUpstream)
	} else {
		defaultUpstreamName = ""
	}
	upstreamCfg = CloneUpstreamConfig(upstreamCfgs[defaultUpstreamName])
	modelRouteCounters.Range(func(key, _ any) bool {
		modelRouteCounters.Delete(key)
		return true
	})
	ApplyRuntimeDependencies(cfg)
}

// ResolveRequestModel 解析请求模型，并返回配置的路由表是否允许该请求。
// 模型别名属于显式路由，因此优先于按模型名自动选择的上游。
// 没有别名时，所有 custom_models 中包含该模型的上游共同参与轮询。
func ResolveRequestModel(model string) (string, ModelAlias, string, *UpstreamConfig, bool, bool) {
	m := strings.TrimSpace(model)
	alias := ModelAlias{}
	configMu.RLock()
	found, aliasMatched := modelAlias[m]
	if aliasMatched {
		alias = CloneModelAlias(found)
		if len(alias.Targets) > 0 {
			counterValue, _ := aliasTargetCounters.LoadOrStore(m, &atomic.Uint64{})
			counter := counterValue.(*atomic.Uint64)
			target := SelectWeightedAliasTarget(alias.Targets, counter.Add(1)-1)
			alias.TargetModel = target.TargetModel
			alias.Upstream = target.Upstream
		}
	}
	if alias.TargetModel != "" {
		m = alias.TargetModel
	}
	if aliasMatched {
		upstreamName, upstream := resolveUpstreamLocked(alias.Upstream)
		configMu.RUnlock()
		if m == "" {
			m = strings.TrimSpace(model)
		}
		return m, alias, upstreamName, upstream, true, true
	}

	candidates := make([]string, 0, len(upstreamCfgs))
	for _, name := range NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs) {
		if upstream := upstreamCfgs[name]; upstream != nil && upstreamHasModel(upstream, m) {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) > 0 {
		counterValue, _ := modelRouteCounters.LoadOrStore(m, &atomic.Uint64{})
		counter := counterValue.(*atomic.Uint64)
		selectedName := candidates[(counter.Add(1)-1)%uint64(len(candidates))]
		selected := CloneUpstreamConfig(upstreamCfgs[selectedName])
		configMu.RUnlock()
		return m, alias, selectedName, selected, false, true
	}

	// Compatibility for old configurations that explicitly declared a default
	// unrestricted upstream. New configurations never populate this field, so
	// they cannot accidentally regain default-upstream routing.
	if legacyDefaultRoute {
		legacyDefault := upstreamCfgs[defaultUpstreamName]
		if legacyDefault == nil || len(legacyDefault.CustomModels) != 0 {
			legacyDefault = nil
		}
		if legacyDefault != nil && strings.TrimSpace(defaultUpstreamName) != "" {
			selected := CloneUpstreamConfig(legacyDefault)
			configMu.RUnlock()
			return m, alias, defaultUpstreamName, selected, false, true
		}
	}

	// Keep a useful upstream name in the not-found error path without treating
	// that upstream as a match. The handler rejects the request when matched is
	// false.
	fallbackName := ""
	var fallback *UpstreamConfig
	if legacyDefaultRoute && strings.TrimSpace(defaultUpstreamName) != "" {
		fallbackName = defaultUpstreamName
		fallback = CloneUpstreamConfig(upstreamCfgs[fallbackName])
	}
	if fallback == nil {
		names := NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs)
		if len(names) > 0 {
			fallbackName = names[0]
			fallback = CloneUpstreamConfig(upstreamCfgs[fallbackName])
		}
	}
	configMu.RUnlock()
	return m, alias, fallbackName, fallback, false, false

}

func upstreamHasModel(upstream *UpstreamConfig, model string) bool {
	if upstream == nil {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, configuredModel := range upstream.CustomModels {
		if strings.TrimSpace(configuredModel) == model {
			return true
		}
	}
	return false
}

// ShouldForwardReasoningParameters 区分“没有模型别名”和“别名显式关闭推理”。
// 直接模型必须保留客户端显式发送的推理参数；只有命中的模型别名可以通过
// WithReasoning=false 禁止向上游发送这些参数。
func ShouldForwardReasoningParameters(alias ModelAlias, aliasMatched bool) bool {
	return !aliasMatched || alias.WithReasoning
}

func ResolveModel(model string) (string, ModelAlias, string, *UpstreamConfig) {
	m, alias, upstreamName, upstream, _, _ := ResolveRequestModel(model)
	return m, alias, upstreamName, upstream
}

func ResolveModelAlias(model string) (string, ModelAlias) {
	resolvedModel, alias, _, _ := ResolveModel(model)
	return resolvedModel, alias
}

func ResolveModelName(model string) string {
	name, _, _, _ := ResolveModel(model)
	return name
}

func GetDefaultUpstreamName() string {
	configMu.RLock()
	defer configMu.RUnlock()
	return defaultUpstreamName
}

func GetConfiguredUpstreamCount() int {
	configMu.RLock()
	defer configMu.RUnlock()
	return len(upstreamCfgs)
}

func GetDefaultUpstreamConfig() *UpstreamConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return CloneUpstreamConfig(upstreamCfgs[defaultUpstreamName])
}

func GetUpstreamNames() []string {
	configMu.RLock()
	defer configMu.RUnlock()
	return NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs)
}

func GetReasoningEffortMap() map[string]string {
	configMu.RLock()
	defer configMu.RUnlock()
	cp := make(map[string]string, len(reasoningEffortMap))
	for k, v := range reasoningEffortMap {
		cp[k] = v
	}
	return cp
}

func GetReasoningEffortMapForAlias(alias ModelAlias) map[string]string {
	if len(alias.ReasoningEffortMap) > 0 {
		return CloneStringMap(alias.ReasoningEffortMap)
	}
	return GetReasoningEffortMap()
}

func GetWebSearchConfig() WebSearchConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	return webSearchCfg
}

func CloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func CloneModelAlias(source ModelAlias) ModelAlias {
	result := source
	result.ReasoningEffortMap = CloneStringMap(source.ReasoningEffortMap)
	if source.Targets != nil {
		result.Targets = append([]ModelAliasTarget(nil), source.Targets...)
	}
	return result
}

// SelectWeightedAliasTarget returns a target for a zero-based sequence number.
// ResolveRequestModel keeps an independent sequence per alias, producing a stable
// weighted cycle without the short-term skew of random selection.
func SelectWeightedAliasTarget(targets []ModelAliasTarget, sequence uint64) ModelAliasTarget {
	if len(targets) == 0 {
		return ModelAliasTarget{}
	}
	var total uint64
	for _, target := range targets {
		if target.Weight <= 0 {
			continue
		}
		total += uint64(target.Weight)
	}
	if total == 0 {
		return ModelAliasTarget{}
	}
	position := sequence % total
	for _, target := range targets {
		if target.Weight <= 0 {
			continue
		}
		if position < uint64(target.Weight) {
			return target
		}
		position -= uint64(target.Weight)
	}
	return ModelAliasTarget{}
}
