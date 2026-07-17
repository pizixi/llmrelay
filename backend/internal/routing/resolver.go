// Package routing 负责模型别名、默认上游和推理参数转发策略。
package routing

import (
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
)

func ResolveRequestModel(model string) (string, domain.ModelAlias, string, *domain.UpstreamConfig, bool, bool) {
	return config.ResolveRequestModel(model)
}

func ResolveModel(model string) (string, domain.ModelAlias, string, *domain.UpstreamConfig) {
	return config.ResolveModel(model)
}

func ResolveModelAlias(model string) (string, domain.ModelAlias) {
	return config.ResolveModelAlias(model)
}

func ResolveModelName(model string) string { return config.ResolveModelName(model) }

func ShouldForwardReasoningParameters(alias domain.ModelAlias, aliasMatched bool) bool {
	return config.ShouldForwardReasoningParameters(alias, aliasMatched)
}

func ReasoningEffortMapForAlias(alias domain.ModelAlias) map[string]string {
	return config.GetReasoningEffortMapForAlias(alias)
}
