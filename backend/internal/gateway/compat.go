package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"llmrelay/backend/internal/bridge"
	"llmrelay/backend/internal/bridge/convert"
	bridgestream "llmrelay/backend/internal/bridge/stream"
	"llmrelay/backend/internal/catalog"
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/protocol/responses"
	"llmrelay/backend/internal/routing"
	"llmrelay/backend/internal/stats"
	"llmrelay/backend/internal/upstream"
)

type OpenAIRequest = chat.Request
type Message = chat.Message
type ToolCall = chat.ToolCall
type FunctionCall = chat.FunctionCall
type Tool = chat.Tool
type ToolFunction = chat.ToolFunction
type ClaudeRequest = anthropic.Request
type ClaudeTool = anthropic.Tool
type ResponsesAPIRequest = responses.Request
type ResponsesTool = responses.Tool
type ResponseToolNameMapping = responses.ToolNameMapping
type UpstreamConfig = domain.UpstreamConfig
type UpstreamType = domain.UpstreamType
type ModelAlias = domain.ModelAlias
type ModelInfo = domain.ModelInfo
type WebSearchConfig = domain.WebSearchConfig
type WireProtocol = bridge.WireProtocol
type ProtocolDecision = bridge.ProtocolDecision
type BridgeWarning = bridge.BridgeWarning
type BridgePlan = bridge.BridgePlan
type BridgePlanRequest = bridge.BridgePlanRequest
type BridgeFidelity = bridge.BridgeFidelity
type Capability = bridge.Capability
type CapabilityResult = bridge.CapabilityResult
type CapabilityOutcome = bridge.CapabilityOutcome
type BridgeMode = domain.BridgeMode

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses

	WireChat      = bridge.WireChat
	WireAnthropic = bridge.WireAnthropic
	WireResponses = bridge.WireResponses

	BridgePathPassthrough = bridge.BridgePathPassthrough
	BridgePathPairwise    = bridge.BridgePathPairwise
	BridgePathPivot       = bridge.BridgePathPivot

	BridgeModeCompatible = domain.BridgeModeCompatible
	BridgeModeStrict     = domain.BridgeModeStrict

	FidelityExact     = bridge.FidelityExact
	FidelityPatched   = bridge.FidelityPatched
	FidelityConverted = bridge.FidelityConverted
	FidelityEmulated  = bridge.FidelityEmulated
	FidelityRejected  = bridge.FidelityRejected

	CapabilityStreaming        = bridge.CapabilityStreaming
	CapabilityToolCalls        = bridge.CapabilityToolCalls
	CapabilityReasoning        = bridge.CapabilityReasoning
	CapabilityStructuredOutput = bridge.CapabilityStructuredOutput
	CapabilityCustomTools      = bridge.CapabilityCustomTools
	CapabilityHostedWebSearch  = bridge.CapabilityHostedWebSearch
	CapabilityStatefulContext  = bridge.CapabilityStatefulContext
	CapabilityResponseStore    = bridge.CapabilityResponseStore
	CapabilityItemReferences   = bridge.CapabilityItemReferences
	CapabilityBackground       = bridge.CapabilityBackground
	CapabilityEncryptedReason  = bridge.CapabilityEncryptedReason
	CapabilityPromptCaching    = bridge.CapabilityPromptCaching

	CapabilityPreserved  = bridge.CapabilityPreserved
	CapabilityDowngraded = bridge.CapabilityDowngraded
	CapabilityEmulated   = bridge.CapabilityEmulated
	CapabilityRejected   = bridge.CapabilityRejected

	internalAnthropicRequestKey = chat.InternalAnthropicRequestKey
)

var (
	requestCount   atomic.Int64
	debugMode      bool
	debugLogBodies bool
)

func SetDebug(enabled, logBodies bool) {
	debugMode = enabled
	debugLogBodies = logBodies
	convert.SetDebug(enabled, logBodies)
	upstream.SetDebug(enabled, logBodies)
	bridgestream.SetDebug(enabled, logBodies)
}

func readAPIRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	const maxAPIRequestBodyBytes int64 = 10 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err == nil {
		return body, 0, nil
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", maxAPIRequestBodyBytes)
	}
	return nil, http.StatusBadRequest, err
}

func writeExternalAPIError(w http.ResponseWriter, path string, status int, errorType, message string) {
	writeClientAPIError(w, bridge.ClientProtocolFromPath(path), status, errorType, message)
}

func writeClientAPIError(w http.ResponseWriter, client WireProtocol, status int, errorType, message string) {
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

func writeModelNotFoundError(w http.ResponseWriter, path, model string) {
	errorType := "invalid_request_error"
	if path == "/v1/messages" {
		errorType = "not_found_error"
	}
	writeExternalAPIError(w, path, http.StatusNotFound, errorType,
		fmt.Sprintf("model %q is not configured in model mappings or on any upstream", model))
}

func resolveRequestModel(model string) (string, ModelAlias, string, *UpstreamConfig, bool, bool) {
	return routing.ResolveRequestModel(model)
}

func getReasoningEffortMapForAlias(alias ModelAlias) map[string]string {
	return routing.ReasoningEffortMapForAlias(alias)
}

func shouldForwardReasoningParameters(alias ModelAlias, matched bool) bool {
	return routing.ShouldForwardReasoningParameters(alias, matched)
}

func getWebSearchConfig() WebSearchConfig { return config.GetWebSearchConfig() }

func decideProtocolBridge(client WireProtocol, target *UpstreamConfig, mode BridgeMode) ProtocolDecision {
	return bridge.Decide(client, target, mode)
}

func buildBridgePlan(request BridgePlanRequest) BridgePlan { return bridge.BuildPlan(request) }

func buildBridgePlanForUpstream(client WireProtocol, target *UpstreamConfig, mode BridgeMode, requirements ...Capability) BridgePlan {
	return bridge.BuildPlanForUpstream(client, target, mode, requirements...)
}

func useBridgePivot(decision *ProtocolDecision) { bridge.UsePivot(decision) }

func markBridgePatched(decision *ProtocolDecision) { bridge.MarkPatched(decision) }

func evaluateBridgeCapabilities(decision *ProtocolDecision, upstream *UpstreamConfig, requirements ...Capability) {
	bridge.EvaluateCapabilities(decision, upstream, requirements...)
}

// capabilityBridgeWarnings turns planner outcomes into the same warning
// shape used by the protocol converters. Handlers use this for strict-mode
// rejection so an explicitly unsupported provider capability cannot silently
// proceed through an otherwise native-looking path.
func capabilityBridgeWarnings(decision ProtocolDecision) []BridgeWarning {
	return bridge.CapabilityWarnings(decision)
}

// Explicit capability declarations are authoritative even on a pairwise
// bridge. Protocol-level gaps remain converter-owned because some features
// (notably hosted search) are negotiated dynamically at runtime.
func explicitCapabilityBridgeWarnings(decision ProtocolDecision) []BridgeWarning {
	var warnings []BridgeWarning
	for _, result := range decision.Capabilities {
		if result.Outcome != CapabilityRejected ||
			!strings.Contains(result.Reason, "provider declaration disables") {
			continue
		}
		warnings = appendBridgeWarning(warnings, BridgeWarning{
			Code:    "capability_rejected",
			Path:    "capabilities." + string(result.Capability),
			Message: result.Reason,
		})
	}
	return warnings
}

func conversionCapabilityBridgeWarnings(decision ProtocolDecision, skipHostedSearch bool) []BridgeWarning {
	warnings := capabilityBridgeWarnings(decision)
	if !skipHostedSearch {
		return warnings
	}
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning.Path == "capabilities."+string(CapabilityHostedWebSearch) {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func effectiveBridgeMode(r *http.Request, target *UpstreamConfig) BridgeMode {
	return bridge.EffectiveMode(r, target)
}

func applyDecisionHeaders(header http.Header, decision ProtocolDecision, warnings []BridgeWarning) {
	bridge.ApplyDecisionHeaders(header, decision, warnings)
}

func rejectStrictBridgeWarnings(w http.ResponseWriter, r *http.Request, warnings []BridgeWarning) bool {
	return bridge.RejectStrictWarnings(w, r, warnings)
}

func logBridgeWarnings(scope, upstreamName, model string, warnings []BridgeWarning) {
	bridge.LogWarnings(scope, upstreamName, model, warnings)
}

func appendBridgeWarning(warnings []BridgeWarning, warning BridgeWarning) []BridgeWarning {
	return bridge.AppendWarning(warnings, warning)
}

func appendBridgeWarnings(warnings []BridgeWarning, additions ...[]BridgeWarning) []BridgeWarning {
	return bridge.AppendWarnings(warnings, additions...)
}

func writeBridgeWarningHeaders(header http.Header, warnings []BridgeWarning) {
	bridge.WriteWarningHeaders(header, warnings)
}

func bridgeWarningsError(warnings []BridgeWarning) error { return bridge.WarningsError(warnings) }

func bridgeString(value any) string { return convert.BridgeString(value) }

func effectiveUpstreamName(name string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "default"
}

type requestUsageAccumulator struct {
	inner *stats.RequestUsageAccumulator
}

func newRequestUsageAccumulator(model string, route ...string) *requestUsageAccumulator {
	return &requestUsageAccumulator{inner: stats.NewRequestUsageAccumulator(model, route...)}
}

func newRequestUsageAccumulatorForContext(ctx context.Context, model string, route ...string) *requestUsageAccumulator {
	return &requestUsageAccumulator{inner: stats.NewRequestUsageAccumulatorForContext(ctx, model, route...)}
}

func usageIdentity(model, upstreamName, upstreamModel string) string {
	return stats.UsageIdentity(model, upstreamName, upstreamModel)
}

func usageIdentityForContext(ctx context.Context, model, upstreamName, upstreamModel string) string {
	return stats.UsageIdentityForContext(ctx, model, upstreamName, upstreamModel)
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

func getRoutableModelInfos() []ModelInfo { return catalog.GetRoutableModelInfos() }

func chatBridgeWarnings(request *OpenAIRequest, target *UpstreamConfig) []BridgeWarning {
	return bridge.ChatWarnings(request, target)
}
