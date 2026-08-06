package config

import (
	"log"
	"sort"
	"strings"

	"llmrelay/backend/internal/bridge/convert"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/websearch"
)

type AppConfig = domain.AppConfig
type ModelAlias = domain.ModelAlias
type ModelAliasTarget = domain.ModelAliasTarget
type UpstreamConfig = domain.UpstreamConfig
type UpstreamType = domain.UpstreamType
type BridgeMode = domain.BridgeMode
type WebSearchConfig = domain.WebSearchConfig
type Socks5Proxy = domain.Socks5Proxy

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses

	BridgeModeCompatible = domain.BridgeModeCompatible
	BridgeModeStrict     = domain.BridgeModeStrict

	socks5RR                      = netproxy.ModeRoundRobin
	socks5RateLimitSwitch         = netproxy.ModeRateLimitSwitch
	socks5RateLimitSwitchNoDirect = netproxy.ModeRateLimitSwitchNoDirect
)

var (
	upstreamCfg         *UpstreamConfig
	upstreamCfgs        = map[string]*UpstreamConfig{}
	defaultUpstreamName string
)

func ValidateWebSearchConfig(config WebSearchConfig) error {
	return websearch.ValidateWebSearchConfig(config)
}

func NormalizeWebSearchConfig(config WebSearchConfig) WebSearchConfig {
	return websearch.NormalizeWebSearchConfig(config)
}

func ResetHostedWebSearchCapabilityCache() {
	websearch.ResetHostedWebSearchCapabilityCache()
}

func CloneUpstreamConfig(config *UpstreamConfig) *UpstreamConfig {
	if config == nil {
		return nil
	}
	copy := *config
	if config.MaxRetries != nil {
		value := *config.MaxRetries
		copy.MaxRetries = &value
	}
	if config.CustomModels != nil {
		copy.CustomModels = append([]string(nil), config.CustomModels...)
	}
	if config.Capabilities != nil {
		copy.Capabilities = make(map[string]bool, len(config.Capabilities))
		for name, enabled := range config.Capabilities {
			copy.Capabilities[name] = enabled
		}
	}
	return &copy
}

func NormalizeSingleUpstream(config *UpstreamConfig) bool {
	if config == nil {
		return false
	}
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Proxy = strings.TrimSpace(config.Proxy)
	if strings.EqualFold(config.Proxy, "direct") {
		config.Proxy = ""
	}
	if config.APIType == "" {
		config.APIType = UpstreamOpenAI
	}
	switch config.APIType {
	case UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses:
	default:
		log.Printf("警告: 忽略未知上游 api_type %q", config.APIType)
		return false
	}
	config.BridgeMode = BridgeMode(strings.ToLower(strings.TrimSpace(string(config.BridgeMode))))
	if config.BridgeMode == "" {
		config.BridgeMode = BridgeModeCompatible
	}
	switch config.BridgeMode {
	case BridgeModeCompatible, BridgeModeStrict:
	default:
		log.Printf("警告: 上游 bridge_mode %q 无效，已回退为 compatible", config.BridgeMode)
		config.BridgeMode = BridgeModeCompatible
	}
	config.ResponsesReasoningFormat = strings.TrimSpace(config.ResponsesReasoningFormat)
	if len(config.Capabilities) > 0 {
		capabilities := make(map[string]bool, len(config.Capabilities))
		for name, enabled := range config.Capabilities {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" {
				capabilities[name] = enabled
			}
		}
		config.Capabilities = capabilities
	}
	if len(config.CustomModels) > 0 {
		cleaned := make([]string, 0, len(config.CustomModels))
		seen := make(map[string]struct{}, len(config.CustomModels))
		for _, model := range config.CustomModels {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, exists := seen[model]; exists {
				continue
			}
			seen[model] = struct{}{}
			cleaned = append(cleaned, model)
		}
		config.CustomModels = cleaned
	}
	return config.BaseURL != ""
}

func SortedUpstreamNames(values map[string]*UpstreamConfig) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ResolveUpstream(name string) (string, *UpstreamConfig) {
	configMu.RLock()
	defer configMu.RUnlock()
	return resolveUpstreamLocked(name)
}

// resolveUpstreamLocked resolves an explicitly named upstream and otherwise
// returns the first configured upstream in stable order. It intentionally does
// not consult a default-upstream selector.
func resolveUpstreamLocked(name string) (string, *UpstreamConfig) {
	requestedName := strings.TrimSpace(name)
	if requestedName != "" {
		if config := CloneUpstreamConfig(upstreamCfgs[requestedName]); config != nil {
			return requestedName, config
		}
		// 显式的模型到上游绑定绝不能静默路由到其他供应商。
		// 返回 nil 可产生明确的配置错误。
		return requestedName, nil
	}
	for _, fallbackName := range NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs) {
		if config := CloneUpstreamConfig(upstreamCfgs[fallbackName]); config != nil {
			return fallbackName, config
		}
	}
	return "", nil
}

func ApplyRuntimeDependencies(config AppConfig) {
	// SOCKS5 entries are a pool of named connection definitions. The selected
	// proxy is now read from each upstream's Proxy field at request time; there
	// is no process-wide active proxy.
	netproxy.Configure(config.Socks5Proxies)
	websearch.SetConfig(config.WebSearch)
	convert.SetReasoningEffortMap(config.ReasoningEffortMap)
}
