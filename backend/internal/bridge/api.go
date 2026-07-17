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
