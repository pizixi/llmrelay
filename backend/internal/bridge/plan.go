package bridge

import (
	"sort"
	"strings"

	"llmrelay/backend/internal/domain"
)

// BridgeFidelity describes how much of the requested wire contract is kept.
// It is deliberately separate from BridgePath: a same-protocol request may
// still need a small gateway patch (for example, model routing), while a
// pivot is a compatibility emulation rather than a normal pairwise adapter.
type BridgeFidelity string

const (
	FidelityExact     BridgeFidelity = "exact"
	FidelityPatched   BridgeFidelity = "patched"
	FidelityConverted BridgeFidelity = "converted"
	FidelityEmulated  BridgeFidelity = "emulated"
	FidelityRejected  BridgeFidelity = "rejected"
)

// Capability is a protocol feature whose preservation can be evaluated before
// a request is sent. Unknown capabilities are intentionally accepted so newer
// provider features can be declared without requiring an older gateway build
// to reject the configuration.
type Capability string

const (
	CapabilityStreaming        Capability = "streaming"
	CapabilityToolCalls        Capability = "tool_calls"
	CapabilityReasoning        Capability = "reasoning"
	CapabilityStructuredOutput Capability = "structured_output"
	CapabilityCustomTools      Capability = "custom_tools"
	CapabilityHostedWebSearch  Capability = "hosted_web_search"
	CapabilityStatefulContext  Capability = "stateful_context"
	CapabilityResponseStore    Capability = "response_store"
	CapabilityItemReferences   Capability = "item_references"
	CapabilityBackground       Capability = "background"
	CapabilityEncryptedReason  Capability = "encrypted_reasoning"
	CapabilityPromptCaching    Capability = "prompt_caching"
)

// CapabilityOutcome is intentionally explicit in responses and diagnostics.
// "preserved" means the upstream implements the feature natively; the other
// values describe a downgrade, a gateway emulation, or a strict rejection.
type CapabilityOutcome string

const (
	CapabilityPreserved  CapabilityOutcome = "preserved"
	CapabilityDowngraded CapabilityOutcome = "downgraded"
	CapabilityEmulated   CapabilityOutcome = "emulated"
	CapabilityRejected   CapabilityOutcome = "rejected"
)

type CapabilityResult struct {
	Capability Capability        `json:"capability"`
	Outcome    CapabilityOutcome `json:"outcome"`
	Reason     string            `json:"reason,omitempty"`
}

// BridgePlanRequest contains only routing facts. Request-specific conversion
// code remains in bridge/convert and bridge/stream; this keeps planning pure
// and cheap enough to use for logging, headers, and tests.
type BridgePlanRequest struct {
	Client         WireProtocol
	Upstream       WireProtocol
	Mode           domain.BridgeMode
	UpstreamConfig *domain.UpstreamConfig
	PatchRequired  bool
	ForcePivot     bool
	Requirements   []Capability
}

// BridgePlan is the stable decision object shared by handlers and diagnostics.
type BridgePlan struct {
	Client       WireProtocol       `json:"client"`
	Upstream     WireProtocol       `json:"upstream"`
	Path         BridgePath         `json:"path"`
	Fidelity     BridgeFidelity     `json:"fidelity"`
	Mode         domain.BridgeMode  `json:"mode"`
	Pivot        WireProtocol       `json:"pivot,omitempty"`
	Capabilities []CapabilityResult `json:"capabilities,omitempty"`
}

// BuildBridgePlan selects exact passthrough first, then a patched passthrough,
// then a direct pairwise adapter, and only finally the Chat pivot. The order is
// important: a pivot is more expensive and loses more provider state.
func BuildBridgePlan(request BridgePlanRequest) BridgePlan {
	client := normalizeWireProtocol(request.Client)
	upstream := normalizeWireProtocol(request.Upstream)
	mode := request.Mode
	if mode == "" {
		mode = domain.BridgeModeCompatible
	}
	plan := BridgePlan{Client: client, Upstream: upstream, Mode: mode}

	if !knownWireProtocol(client) || !knownWireProtocol(upstream) {
		plan.Path = BridgePathPairwise
		plan.Fidelity = FidelityRejected
		plan.Capabilities = evaluateCapabilities(request, plan)
		return plan
	}

	switch {
	case request.ForcePivot:
		plan.Path = BridgePathPivot
		plan.Pivot = WireChat
		plan.Fidelity = FidelityEmulated
	case client == upstream && request.PatchRequired:
		plan.Path = BridgePathPassthrough
		plan.Fidelity = FidelityPatched
	case client == upstream:
		plan.Path = BridgePathPassthrough
		plan.Fidelity = FidelityExact
	case hasPairwiseBridge(client, upstream):
		plan.Path = BridgePathPairwise
		plan.Fidelity = FidelityConverted
	default:
		plan.Path = BridgePathPivot
		plan.Pivot = WireChat
		plan.Fidelity = FidelityEmulated
	}

	plan.Capabilities = evaluateCapabilities(request, plan)
	if hasRejectedCapability(plan.Capabilities) {
		plan.Fidelity = FidelityRejected
	}
	return plan
}

// BuildBridgePlanForUpstream is the common convenience entry point used by
// handlers that already have an UpstreamConfig.
func BuildBridgePlanForUpstream(client WireProtocol, upstream *domain.UpstreamConfig, mode domain.BridgeMode, requirements ...Capability) BridgePlan {
	protocol := WireChat
	if upstream != nil {
		protocol = wireProtocolFromUpstream(upstream.APIType)
	}
	return BuildBridgePlan(BridgePlanRequest{
		Client: client, Upstream: protocol, Mode: mode,
		UpstreamConfig: upstream, Requirements: requirements,
	})
}

func normalizeWireProtocol(value WireProtocol) WireProtocol {
	switch strings.ToLower(strings.TrimSpace(string(value))) {
	case string(WireAnthropic), "anthropic", "messages":
		return WireAnthropic
	case string(WireResponses), "responses", "openai-response":
		return WireResponses
	case string(WireChat), "chat", "openai":
		return WireChat
	default:
		return value
	}
}

func knownWireProtocol(value WireProtocol) bool {
	return value == WireChat || value == WireAnthropic || value == WireResponses
}

func hasRejectedCapability(results []CapabilityResult) bool {
	for _, result := range results {
		if result.Outcome == CapabilityRejected {
			return true
		}
	}
	return false
}

// NativeCapabilities returns the conservative built-in capability profile for
// a protocol. Provider configuration can override individual entries through
// UpstreamConfig.Capabilities; omitted entries keep this safe default.
func NativeCapabilities(protocol WireProtocol) map[Capability]bool {
	profile := map[Capability]bool{}
	switch normalizeWireProtocol(protocol) {
	case WireChat:
		profile = map[Capability]bool{
			CapabilityStreaming: true, CapabilityToolCalls: true,
			CapabilityReasoning: true, CapabilityStructuredOutput: true,
			CapabilityPromptCaching: true,
		}
	case WireAnthropic:
		profile = map[Capability]bool{
			CapabilityStreaming: true, CapabilityToolCalls: true,
			CapabilityReasoning: true, CapabilityHostedWebSearch: true,
			CapabilityPromptCaching: true,
		}
	case WireResponses:
		profile = map[Capability]bool{
			CapabilityStreaming: true, CapabilityToolCalls: true,
			CapabilityReasoning: true, CapabilityStructuredOutput: true,
			CapabilityCustomTools: true, CapabilityHostedWebSearch: true,
			CapabilityStatefulContext: true, CapabilityResponseStore: true,
			CapabilityItemReferences: true, CapabilityBackground: true,
			CapabilityEncryptedReason: true, CapabilityPromptCaching: true,
		}
	}
	return profile
}

// EffectiveCapabilities applies an optional provider declaration without
// mutating the config or the shared built-in matrix.
func EffectiveCapabilities(protocol WireProtocol, upstream *domain.UpstreamConfig) map[Capability]bool {
	profile := NativeCapabilities(protocol)
	if upstream == nil || len(upstream.Capabilities) == 0 {
		return profile
	}
	for raw, enabled := range upstream.Capabilities {
		name := Capability(strings.ToLower(strings.TrimSpace(raw)))
		if name != "" {
			profile[name] = enabled
		}
	}
	return profile
}

func evaluateCapabilities(request BridgePlanRequest, plan BridgePlan) []CapabilityResult {
	if len(request.Requirements) == 0 {
		return nil
	}
	source := NativeCapabilities(plan.Client)
	target := EffectiveCapabilities(plan.Upstream, request.UpstreamConfig)
	results := make([]CapabilityResult, 0, len(request.Requirements))
	seen := map[Capability]struct{}{}
	for _, capability := range request.Requirements {
		capability = Capability(strings.ToLower(strings.TrimSpace(string(capability))))
		if capability == "" {
			continue
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		sourceHas := source[capability]
		targetHas := target[capability]
		explicitlyDisabled := explicitCapabilityDisabled(request.UpstreamConfig, capability)
		result := CapabilityResult{Capability: capability, Outcome: CapabilityPreserved}
		switch {
		case !targetHas && plan.Client == plan.Upstream:
			result.Outcome = CapabilityRejected
			if explicitlyDisabled {
				result.Reason = "the selected provider declaration disables this native capability"
			} else {
				result.Reason = "the selected provider protocol disables this native capability"
			}
		case !targetHas && explicitlyDisabled:
			result.Outcome = CapabilityDowngraded
			result.Reason = "the selected provider declaration disables this native capability"
		case targetHas && (sourceHas || plan.Path == BridgePathPassthrough):
			result.Outcome = CapabilityPreserved
		case capabilityCanBeEmulated(capability):
			result.Outcome = CapabilityEmulated
			result.Reason = "the gateway keeps the request through local compatibility state or metadata"
		case targetHas:
			result.Outcome = CapabilityDowngraded
			result.Reason = "the client protocol does not expose the complete upstream capability"
		default:
			result.Outcome = CapabilityDowngraded
			result.Reason = "the selected upstream protocol has no native equivalent"
		}
		if result.Outcome != CapabilityPreserved && plan.Mode == domain.BridgeModeStrict {
			result.Outcome = CapabilityRejected
			if result.Reason == "" {
				result.Reason = "strict bridge mode does not allow capability loss"
			} else {
				result.Reason += "; strict bridge mode rejects the loss"
			}
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Capability < results[j].Capability })
	return results
}

func explicitCapabilityDisabled(upstream *domain.UpstreamConfig, capability Capability) bool {
	if upstream == nil {
		return false
	}
	for raw, enabled := range upstream.Capabilities {
		if enabled {
			continue
		}
		if Capability(strings.ToLower(strings.TrimSpace(raw))) == capability {
			return true
		}
	}
	return false
}

func capabilityCanBeEmulated(capability Capability) bool {
	switch capability {
	case CapabilityStatefulContext, CapabilityResponseStore, CapabilityItemReferences:
		return true
	default:
		return false
	}
}

// CapabilityMatrix returns a copy of the protocol-pair matrix. A copy keeps
// callers from mutating process-wide routing behavior while still making the
// compatibility policy inspectable by admin tooling and tests.
func CapabilityMatrix() map[WireProtocol]map[WireProtocol]map[Capability]CapabilityOutcome {
	protocols := []WireProtocol{WireChat, WireAnthropic, WireResponses}
	result := make(map[WireProtocol]map[WireProtocol]map[Capability]CapabilityOutcome, len(protocols))
	for _, client := range protocols {
		result[client] = make(map[WireProtocol]map[Capability]CapabilityOutcome, len(protocols))
		for _, upstream := range protocols {
			row := map[Capability]CapabilityOutcome{}
			for capability, sourceHas := range NativeCapabilities(client) {
				targetHas := NativeCapabilities(upstream)[capability]
				outcome := CapabilityDowngraded
				if client == upstream || (sourceHas && targetHas) {
					outcome = CapabilityPreserved
				} else if capabilityCanBeEmulated(capability) {
					outcome = CapabilityEmulated
				}
				row[capability] = outcome
			}
			result[client][upstream] = row
		}
	}
	return result
}

// ProviderCapabilityDeclaration is a serializable description suitable for an
// admin endpoint. It reports effective values, including explicit provider
// overrides, rather than exposing a mutable internal map.
type ProviderCapabilityDeclaration struct {
	Protocol     WireProtocol        `json:"protocol"`
	Capabilities map[Capability]bool `json:"capabilities"`
}

func DeclareProviderCapabilities(upstream *domain.UpstreamConfig) ProviderCapabilityDeclaration {
	protocol := WireChat
	if upstream != nil {
		protocol = wireProtocolFromUpstream(upstream.APIType)
	}
	return ProviderCapabilityDeclaration{Protocol: protocol, Capabilities: EffectiveCapabilities(protocol, upstream)}
}
