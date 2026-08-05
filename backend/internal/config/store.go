package config

import "llmrelay/backend/internal/netproxy"

// SetPath 设置配置数据库路径。
func SetPath(path string) { configPath = path }

func Path() string { return configPath }

// Snapshot 返回当前运行配置的深拷贝。
func Snapshot() AppConfig {
	configMu.RLock()
	result := AppConfig{
		ModelAlias:         make(map[string]ModelAlias, len(modelAlias)),
		ReasoningEffortMap: CloneStringMap(reasoningEffortMap),
		WebSearch:          webSearchCfg,
		APIKeys:            CloneAPIKeys(apiKeys),
		Upstreams:          make(map[string]*UpstreamConfig, len(upstreamCfgs)),
		UpstreamOrder:      append([]string(nil), upstreamOrder...),
		DefaultUpstream:    defaultUpstreamName,
	}
	for name, alias := range modelAlias {
		result.ModelAlias[name] = CloneModelAlias(alias)
	}
	for name, upstream := range upstreamCfgs {
		result.Upstreams[name] = CloneUpstreamConfig(upstream)
	}
	configMu.RUnlock()
	result.Socks5Proxies, result.ActiveSocks5 = netproxy.Snapshot()
	return result
}
