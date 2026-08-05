// Package domain 定义网关各模块共享的稳定值对象。
//
// 该包不执行文件、网络或 HTTP 操作，避免基础类型反向依赖具体实现。
package domain

import "encoding/json"

// UpstreamType 表示上游服务使用的 API 协议。
type UpstreamType string

const (
	UpstreamOpenAI    UpstreamType = "openai"
	UpstreamAnthropic UpstreamType = "anthropic"
	UpstreamResponses UpstreamType = "openai-responses"
)

// BridgeMode 控制跨协议转换遇到已知损失时的处理方式。
type BridgeMode string

const (
	BridgeModeCompatible BridgeMode = "compatible"
	BridgeModeStrict     BridgeMode = "strict"
)

type UpstreamConfig struct {
	// ID is the stable relational identity of this upstream. The display name
	// is editable, so model alias targets must not depend on it to detect a
	// rename.
	ID                       int64        `json:"id,omitempty"`
	BaseURL                  string       `json:"base_url"`
	APIKey                   string       `json:"api_key"`
	APIType                  UpstreamType `json:"api_type"`
	BridgeMode               BridgeMode   `json:"bridge_mode,omitempty"`
	CustomModels             []string     `json:"custom_models,omitempty"`
	ResponsesReasoningFormat string       `json:"responses_reasoning_format,omitempty"`
	MaxRetries               *int         `json:"max_retries,omitempty"`
}

// WebSearchConfig 控制网关托管的 Web Search 降级功能。它有意设计为全局配置：
// 请求会先尝试上游原生托管搜索；只有明确检测到上游不支持时才选择此执行器。
type WebSearchConfig struct {
	Enabled             bool   `json:"enabled"`
	Provider            string `json:"provider,omitempty"`
	FallbackProvider    string `json:"fallback_provider,omitempty"`
	BaseURL             string `json:"base_url,omitempty"`
	APIKey              string `json:"api_key,omitempty"`
	SearXNGMode         string `json:"searxng_mode,omitempty"`
	SearXNGDirectoryURL string `json:"searxng_directory_url,omitempty"`
	MaxResults          int    `json:"max_results,omitempty"`
	TimeoutSeconds      int    `json:"timeout_seconds,omitempty"`
	MaxToolRounds       int    `json:"max_tool_rounds,omitempty"`
	MaxResultBytes      int    `json:"max_result_bytes,omitempty"`
}

type Socks5Proxy struct {
	Addr     string `json:"addr"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Name     string `json:"name,omitempty"`
}

// APIKey is a client credential accepted by the public /v1 endpoints.
// The clear-text key is intentionally kept in the local configuration so an
// authenticated administrator can copy it from the API key management page.
type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Disabled  bool   `json:"disabled,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type AppConfig struct {
	ModelAlias map[string]ModelAlias `json:"model_alias"`

	ReasoningEffortMap map[string]string          `json:"reasoning_effort_map"`
	WebSearch          WebSearchConfig            `json:"web_search,omitempty"`
	APIKeys            []APIKey                   `json:"api_keys,omitempty"`
	Socks5Proxies      []Socks5Proxy              `json:"socks5_proxies,omitempty"`
	ActiveSocks5       string                     `json:"active_socks5,omitempty"`
	Upstream           *UpstreamConfig            `json:"upstream,omitempty"`
	Upstreams          map[string]*UpstreamConfig `json:"upstreams,omitempty"`
	UpstreamOrder      []string                   `json:"upstream_order,omitempty"`
	DefaultUpstream    string                     `json:"default_upstream,omitempty"`
}

type ModelAlias struct {
	TargetModel        string             `json:"target_model,omitempty"`
	Upstream           string             `json:"upstream,omitempty"`
	Targets            []ModelAliasTarget `json:"targets,omitempty"`
	WithReasoning      bool               `json:"with_reasoning,omitempty"`
	ReasoningEffortMap map[string]string  `json:"reasoning_effort_map,omitempty"`
}

type ModelAliasTarget struct {
	TargetModel string `json:"target_model"`
	Upstream    string `json:"upstream"`
	Weight      int    `json:"weight"`
}

// UnmarshalJSON keeps legacy targets without a weight at the historical
// default of 1 while preserving an explicitly configured zero weight.
func (target *ModelAliasTarget) UnmarshalJSON(data []byte) error {
	var value struct {
		TargetModel string `json:"target_model"`
		Upstream    string `json:"upstream"`
		Weight      *int   `json:"weight"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	target.TargetModel = value.TargetModel
	target.Upstream = value.Upstream
	target.Weight = 1
	if value.Weight != nil {
		target.Weight = *value.Weight
	}
	return nil
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
