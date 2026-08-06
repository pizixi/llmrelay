package bridge

import (
	"net/http"

	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/protocol/responses"
)

// Decide 为客户端与上游协议组合选择转换路径。
func Decide(client WireProtocol, upstream *domain.UpstreamConfig, mode domain.BridgeMode) ProtocolDecision {
	return decideProtocolBridge(client, upstream, mode)
}

// BuildPlan exposes the pure planner for admin tooling and protocol fixture
// tests without requiring a gateway handler or HTTP request.
func BuildPlan(request BridgePlanRequest) BridgePlan { return BuildBridgePlan(request) }

func BuildPlanForUpstream(client WireProtocol, upstream *domain.UpstreamConfig, mode domain.BridgeMode, requirements ...Capability) BridgePlan {
	return BuildBridgePlanForUpstream(client, upstream, mode, requirements...)
}

func NativeCapabilityProfile(protocol WireProtocol) map[Capability]bool {
	return NativeCapabilities(protocol)
}

func EffectiveCapabilityProfile(protocol WireProtocol, upstream *domain.UpstreamConfig) map[Capability]bool {
	return EffectiveCapabilities(protocol, upstream)
}

func ProviderCapabilities(upstream *domain.UpstreamConfig) ProviderCapabilityDeclaration {
	return DeclareProviderCapabilities(upstream)
}

func CapabilityOutcomesMatrix() map[WireProtocol]map[WireProtocol]map[Capability]CapabilityOutcome {
	return CapabilityMatrix()
}

func ProtocolFromUpstream(apiType domain.UpstreamType) WireProtocol {
	return wireProtocolFromUpstream(apiType)
}

func ChooseStreamDispatch(client WireProtocol, decision ProtocolDecision, upstream *domain.UpstreamConfig) string {
	return string(chooseStreamDispatch(client, decision, upstream))
}

func ClientProtocolFromPath(path string) WireProtocol { return clientProtocolFromPath(path) }

func WriteProtocolHeaders(header http.Header, decision ProtocolDecision) {
	writeProtocolBridgeHeaders(header, decision)
}

func ApplyDecisionHeaders(header http.Header, decision ProtocolDecision, warnings []BridgeWarning) {
	applyDecisionHeaders(header, decision, warnings)
}

func UsePivot(decision *ProtocolDecision) {
	decision.UsePivot()
}

func MarkPatched(decision *ProtocolDecision) {
	decision.MarkPatched()
}

func EvaluateCapabilities(decision *ProtocolDecision, upstream *domain.UpstreamConfig, requirements ...Capability) {
	decision.EvaluateCapabilities(upstream, requirements...)
}

// CapabilityWarnings exposes planner losses in the same shape as converter
// warnings, allowing strict-mode handlers and diagnostics to share one policy.
func CapabilityWarnings(decision ProtocolDecision) []BridgeWarning {
	return capabilityWarnings(decision)
}

func AppendWarning(warnings []BridgeWarning, warning BridgeWarning) []BridgeWarning {
	return appendBridgeWarning(warnings, warning)
}

func WarningSeverity(code string) string { return bridgeWarningSeverity(code) }

func AppendWarnings(warnings []BridgeWarning, additions ...[]BridgeWarning) []BridgeWarning {
	return appendBridgeWarnings(warnings, additions...)
}

func WriteWarningHeaders(header http.Header, warnings []BridgeWarning) {
	writeBridgeWarningHeaders(header, warnings)
}

func ApplyWarnings(response map[string]any, warnings []BridgeWarning) {
	applyBridgeWarnings(response, warnings)
}

func WarningsError(warnings []BridgeWarning) error { return bridgeWarningsError(warnings) }

func LogWarnings(scope, upstreamName, model string, warnings []BridgeWarning) {
	logBridgeWarnings(scope, upstreamName, model, warnings)
}

func EffectiveMode(r *http.Request, upstream *domain.UpstreamConfig) domain.BridgeMode {
	return effectiveBridgeMode(r, upstream)
}

func RejectStrictWarnings(w http.ResponseWriter, r *http.Request, warnings []BridgeWarning) bool {
	return rejectStrictBridgeWarnings(w, r, warnings)
}

func ChatWarnings(req *chat.Request, upstream *domain.UpstreamConfig) []BridgeWarning {
	return chatBridgeWarnings(req, upstream)
}

func OpenAIServiceTierFromAnthropic(value any) (string, bool) {
	return openAIServiceTierFromAnthropic(value)
}

func AnthropicServiceTierFromOpenAI(value any) (string, bool, bool) {
	return anthropicServiceTierFromOpenAI(value)
}

func PrependGuidance(messages []chat.Message, guidance []string) []chat.Message {
	return prependBridgeGuidance(messages, guidance)
}

func AnthropicServerToolGuidance(tools []anthropic.Tool) []string {
	return anthropicServerToolGuidance(tools)
}

func ResponsesHostedToolGuidance(tools []responses.Tool) []string {
	return responsesHostedToolGuidance(tools)
}
