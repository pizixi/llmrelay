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
	return &copy
}

func NormalizeSingleUpstream(config *UpstreamConfig) bool {
	if config == nil {
		return false
	}
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKey = strings.TrimSpace(config.APIKey)
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
	if len(config.CustomModels) > 0 {
		cleaned := make([]string, 0, len(config.CustomModels))
		for _, model := range config.CustomModels {
			model = strings.TrimSpace(model)
			if model != "" {
				cleaned = append(cleaned, model)
			}
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
	requestedName := strings.TrimSpace(name)
	if requestedName != "" {
		if config := CloneUpstreamConfig(upstreamCfgs[requestedName]); config != nil {
			return requestedName, config
		}
		// 显式的模型到上游绑定绝不能静默路由到其他供应商。
		// 返回 nil 可产生明确的配置错误。
		return requestedName, nil
	}
	resolvedName := defaultUpstreamName
	if config := CloneUpstreamConfig(upstreamCfgs[resolvedName]); config != nil {
		return resolvedName, config
	}
	for _, fallbackName := range SortedUpstreamNames(upstreamCfgs) {
		if config := CloneUpstreamConfig(upstreamCfgs[fallbackName]); config != nil {
			return fallbackName, config
		}
	}
	return resolvedName, nil
}

func ApplyRuntimeDependencies(config AppConfig) {
	netproxy.Configure(config.Socks5Proxies, config.ActiveSocks5)
	websearch.SetConfig(config.WebSearch)
	convert.SetReasoningEffortMap(config.ReasoningEffortMap)
}
