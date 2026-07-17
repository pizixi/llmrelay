package stream

import (
	"encoding/json"
	"io"
	"net/http"

	"llmrelay/backend/internal/bridge"
	"llmrelay/backend/internal/bridge/convert"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/protocol/responses"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/stats"
	"llmrelay/backend/internal/upstream"
)

type WireProtocol = bridge.WireProtocol
type ProtocolDecision = bridge.ProtocolDecision
type UpstreamConfig = domain.UpstreamConfig
type BridgeMode = domain.BridgeMode
type BridgeWarning = bridge.BridgeWarning
type ResponseToolNameMapping = responses.ToolNameMapping

const (
	WireChat      = bridge.WireChat
	WireAnthropic = bridge.WireAnthropic
	WireResponses = bridge.WireResponses

	BridgeModeStrict = domain.BridgeModeStrict
)

type streamDispatchKind string

const (
	streamKindChatPassthrough      streamDispatchKind = "chat_passthrough"
	streamKindAnthropicPassthrough streamDispatchKind = "anthropic_passthrough"
	streamKindResponsesPassthrough streamDispatchKind = "responses_passthrough"
	streamKindAnthropicToChat      streamDispatchKind = "anthropic_to_chat"
	streamKindResponsesToChat      streamDispatchKind = "responses_to_chat"
	streamKindChatToAnthropic      streamDispatchKind = "chat_to_anthropic"
	streamKindResponsesToAnthropic streamDispatchKind = "responses_to_anthropic"
	streamKindChatToResponses      streamDispatchKind = "chat_to_responses"
	streamKindAnthropicToResponses streamDispatchKind = "anthropic_to_responses"
)

var (
	debugMode      bool
	debugLogBodies bool
)

func SetDebug(enabled, logBodies bool) {
	debugMode = enabled
	debugLogBodies = logBodies
}

func SetSSEHeaders(header http.Header) { sse.SetHeaders(header) }

type requestUsageAccumulator struct {
	inner *stats.RequestUsageAccumulator
}

func NewRequestUsageAccumulator(model string) *requestUsageAccumulator {
	return &requestUsageAccumulator{inner: stats.NewRequestUsageAccumulator(model)}
}

func (a *requestUsageAccumulator) observeMap(usage map[string]any) {
	if a != nil {
		a.inner.ObserveMap(usage)
	}
}

func (a *requestUsageAccumulator) commit() {
	if a != nil {
		a.inner.Commit()
	}
}

func ChooseStreamDispatch(client WireProtocol, decision ProtocolDecision, target *UpstreamConfig) streamDispatchKind {
	return streamDispatchKind(bridge.ChooseStreamDispatch(client, decision, target))
}

func ProxyChatPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string, recordUsage ...bool) error {
	return upstream.ProxyChatPassthroughStream(w, body, model, recordUsage...)
}

func ProxyAnthropicPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	return upstream.ProxyAnthropicPassthroughStream(w, body, model)
}

func ProxyResponsesPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	return upstream.ProxyResponsesPassthroughStream(w, body, model)
}

func ResponsesStreamToChatHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, recordUsage bool) {
	upstream.ResponsesStreamToChatHandler(w, body, model, usageModel, recordUsage)
}

func WriteClientAPIError(w http.ResponseWriter, client WireProtocol, status int, errorType, message string) {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	if errorType == "" {
		errorType = "api_error"
	}
	if message == "" {
		message = "request failed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if client == WireAnthropic {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": errorType, "message": message},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"type": errorType, "message": message},
	})
}

func RandomString(n int) string { return convert.RandomString(n) }

func EmitOpenAIStreamError(w http.ResponseWriter, flusher http.Flusher, payload map[string]any, fallbackMessage string) {
	upstream.EmitOpenAIStreamError(w, flusher, payload, fallbackMessage)
}

func SseDataPayload(line string) (string, bool) { return upstream.SseDataPayload(line) }

func GetFloat(values map[string]any, keys ...string) (float64, bool) {
	return convert.GetFloat(values, keys...)
}

func StreamErrorObject(payload map[string]any, fallbackMessage string) map[string]any {
	return upstream.StreamErrorObject(payload, fallbackMessage)
}

func AnthropicWebSearchRequestsFromUsage(usage map[string]any) int {
	return convert.AnthropicWebSearchRequestsFromUsage(usage)
}

func WithAnthropicWebSearchUsage(usage map[string]any, requests int) map[string]any {
	return convert.WithAnthropicWebSearchUsage(usage, requests)
}

func NormalizeReasoningContent(message map[string]any) {
	convert.NormalizeReasoningContent(message)
}

func ChatWebSearchEvidenceCount(providerOutput, annotations []any) int {
	return convert.ChatWebSearchEvidenceCount(providerOutput, annotations)
}

func NormalizeToolCallArguments(raw string) (string, error) {
	return convert.NormalizeToolCallArguments(raw)
}

func LogStreamToolCallArgumentsValidationFailure(source, itemID, callID, toolName, rawArgs string, outputIndex int, err error) {
	convert.LogStreamToolCallArgumentsValidationFailure(source, itemID, callID, toolName, rawArgs, outputIndex, err)
}

func AppendBridgeWarnings(warnings []BridgeWarning, additions ...[]BridgeWarning) []BridgeWarning {
	return bridge.AppendWarnings(warnings, additions...)
}

func AppendBridgeWarning(warnings []BridgeWarning, warning BridgeWarning) []BridgeWarning {
	return bridge.AppendWarning(warnings, warning)
}

func WriteBridgeWarningHeaders(header http.Header, warnings []BridgeWarning) {
	bridge.WriteWarningHeaders(header, warnings)
}

func LogBridgeWarnings(scope, upstreamName, model string, warnings []BridgeWarning) {
	bridge.LogWarnings(scope, upstreamName, model, warnings)
}

func BridgeWarningsError(warnings []BridgeWarning) error { return bridge.WarningsError(warnings) }

func ApplyResponsesRequestEcho(response map[string]any, echo map[string]any) {
	convert.ApplyResponsesRequestEcho(response, echo)
}

func EmitSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data map[string]any) {
	convert.EmitSSEEvent(w, flusher, event, data)
}

func ResponsesToolsForOutput(tools any) (any, bool) { return convert.ResponsesToolsForOutput(tools) }

func ResponseFunctionCallItem(itemID, status, arguments, callID, name string, mappings map[string]ResponseToolNameMapping) map[string]any {
	return convert.ResponseFunctionCallItem(itemID, status, arguments, callID, name, mappings)
}

func ResponseToolCallItemID(callID, name string, mappings map[string]ResponseToolNameMapping) string {
	return convert.ResponseToolCallItemID(callID, name, mappings)
}

func ResponsesWebSearchToAnthropicBlocks(item map[string]any) []any {
	return convert.ResponsesWebSearchToAnthropicBlocks(item)
}

func BridgeString(value any) string { return convert.BridgeString(value) }

func CloneAnyMap(source map[string]any) map[string]any { return convert.CloneAnyMap(source) }

func LookupResponseToolNameMapping(name string, mappings map[string]ResponseToolNameMapping) (ResponseToolNameMapping, bool) {
	return convert.LookupResponseToolNameMapping(name, mappings)
}

func CustomToolInputFromArguments(arguments string) string {
	return convert.CustomToolInputFromArguments(arguments)
}

func ApplyBridgeWarnings(response map[string]any, warnings []BridgeWarning) {
	bridge.ApplyWarnings(response, warnings)
}
