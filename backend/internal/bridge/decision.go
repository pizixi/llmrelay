package bridge

import (
	"fmt"
	"net/http"

	"llmrelay/backend/internal/domain"
)

// WireProtocol 表示客户端或上游使用的外部 API 协议。
type WireProtocol string

const (
	WireChat      WireProtocol = "openai-chat"
	WireAnthropic WireProtocol = "anthropic-messages"
	WireResponses WireProtocol = "openai-responses"
)

// BridgePath 表示单次转发所选的转换策略。
type BridgePath string

const (
	BridgePathPassthrough BridgePath = "passthrough"
	BridgePathPairwise    BridgePath = "pairwise"
	BridgePathPivot       BridgePath = "pivot"
)

// ProtocolDecision 记录请求应如何进行协议桥接。
type ProtocolDecision struct {
	Client       WireProtocol
	Upstream     WireProtocol
	Path         BridgePath
	Mode         domain.BridgeMode
	Fidelity     BridgeFidelity
	Pivot        WireProtocol
	Capabilities []CapabilityResult
	Plan         BridgePlan
}

func wireProtocolFromUpstream(apiType domain.UpstreamType) WireProtocol {
	switch apiType {
	case domain.UpstreamAnthropic:
		return WireAnthropic
	case domain.UpstreamResponses:
		return WireResponses
	default:
		return WireChat
	}
}

// decideProtocolBridge 为客户端与上游组合选择转换路径。
//
// 混合策略：
//  1. 协议相同 -> 透传
//  2. 可直接桥接 -> 成对转换器
//  3. 其他情况 -> 经 Chat 中转降级
func decideProtocolBridge(client WireProtocol, upstream *domain.UpstreamConfig, mode domain.BridgeMode) ProtocolDecision {
	up := wireProtocolFromUpstream(domain.UpstreamOpenAI)
	if upstream != nil {
		up = wireProtocolFromUpstream(upstream.APIType)
	}
	plan := BuildBridgePlan(BridgePlanRequest{
		Client: client, Upstream: up, Mode: mode, UpstreamConfig: upstream,
	})
	return ProtocolDecision{
		Client: plan.Client, Upstream: plan.Upstream, Path: plan.Path,
		Mode: plan.Mode, Fidelity: plan.Fidelity, Pivot: plan.Pivot,
		Capabilities: append([]CapabilityResult(nil), plan.Capabilities...), Plan: plan,
	}
}

// UsePivot is the single mutation point for runtime fallbacks (for example a
// provider that rejected native hosted search). Keeping Plan and legacy fields
// synchronized prevents headers and diagnostics from disagreeing.
func (d *ProtocolDecision) UsePivot() {
	if d == nil {
		return
	}
	d.Path = BridgePathPivot
	d.Fidelity = FidelityEmulated
	d.Pivot = WireChat
	d.Plan.Path = BridgePathPivot
	d.Plan.Fidelity = FidelityEmulated
	d.Plan.Pivot = WireChat
}

// MarkPatched records a same-protocol request whose body must be changed for
// routing or an explicitly configured provider option. The wire response is
// still native, but it is no longer byte-for-byte request passthrough.
func (d *ProtocolDecision) MarkPatched() {
	if d == nil || d.Path != BridgePathPassthrough || d.Fidelity == FidelityRejected {
		return
	}
	d.Fidelity = FidelityPatched
	d.Plan.Fidelity = FidelityPatched
}

// EvaluateCapabilities refreshes the planner portion after request fields
// have been decoded. Existing path selection is retained; this method only
// adds explicit outcomes so handlers can expose them and strict mode can
// reject them through their normal warning machinery.
func (d *ProtocolDecision) EvaluateCapabilities(upstream *domain.UpstreamConfig, requirements ...Capability) {
	if d == nil || len(requirements) == 0 {
		return
	}
	plan := BuildBridgePlan(BridgePlanRequest{
		Client: d.Client, Upstream: d.Upstream, Mode: d.Mode,
		UpstreamConfig: upstream, Requirements: requirements,
		ForcePivot: d.Path == BridgePathPivot, PatchRequired: d.Fidelity == FidelityPatched,
	})
	// Runtime fallback can have changed d.Path since the initial decision;
	// preserve that explicit choice while adopting the fresh outcomes.
	if d.Path == BridgePathPivot {
		plan.Path = BridgePathPivot
		plan.Pivot = WireChat
		if plan.Fidelity != FidelityRejected {
			plan.Fidelity = FidelityEmulated
		}
	}
	d.Capabilities = append([]CapabilityResult(nil), plan.Capabilities...)
	d.Plan.Capabilities = append([]CapabilityResult(nil), plan.Capabilities...)
	if plan.Fidelity == FidelityRejected {
		d.Fidelity = FidelityRejected
		d.Plan.Fidelity = FidelityRejected
	}
}

func hasPairwiseBridge(client, upstream WireProtocol) bool {
	switch {
	case client == WireChat && (upstream == WireAnthropic || upstream == WireResponses):
		return true
	case upstream == WireChat && (client == WireAnthropic || client == WireResponses):
		return true
	case client == WireAnthropic && upstream == WireResponses:
		return true
	case client == WireResponses && upstream == WireAnthropic:
		return true
	default:
		return false
	}
}

func (d ProtocolDecision) String() string {
	return fmt.Sprintf("client=%s upstream=%s path=%s mode=%s", d.Client, d.Upstream, d.Path, d.Mode)
}

func protocolBridgeModeHeader(path BridgePath, mode domain.BridgeMode) string {
	if path == BridgePathPassthrough {
		return "passthrough"
	}
	if mode == domain.BridgeModeStrict {
		return string(domain.BridgeModeStrict)
	}
	return string(domain.BridgeModeCompatible)
}

func writeProtocolBridgeHeaders(header http.Header, decision ProtocolDecision) {
	if header == nil {
		return
	}
	header.Set("X-Llm2api-Bridge-Path", string(decision.Path))
	header.Set("X-Llm2api-Bridge-Client", string(decision.Client))
	header.Set("X-Llm2api-Bridge-Upstream", string(decision.Upstream))
	header.Set("X-Llm2api-Bridge-Mode", protocolBridgeModeHeader(decision.Path, decision.Mode))
	if decision.Fidelity != "" {
		header.Set("X-Llm2api-Bridge-Fidelity", string(decision.Fidelity))
	}
	header.Del("X-Llm2api-Capability")
	for _, capability := range decision.Capabilities {
		header.Add("X-Llm2api-Capability", string(capability.Capability)+"="+string(capability.Outcome))
	}
}

func applyDecisionHeaders(header http.Header, decision ProtocolDecision, warnings []BridgeWarning) {
	writeBridgeWarningHeaders(header, warnings)
	writeProtocolBridgeHeaders(header, decision)
}

// streamDispatchKind 标识应运行的流转换器或代理。
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
	streamKindUnknown              streamDispatchKind = "unknown"
)

func chooseStreamDispatch(client WireProtocol, decision ProtocolDecision, upstream *domain.UpstreamConfig) streamDispatchKind {
	if decision.Path == BridgePathPassthrough {
		switch client {
		case WireChat:
			return streamKindChatPassthrough
		case WireAnthropic:
			return streamKindAnthropicPassthrough
		case WireResponses:
			return streamKindResponsesPassthrough
		default:
			return streamKindUnknown
		}
	}
	up := wireProtocolFromUpstream(domain.UpstreamOpenAI)
	if upstream != nil {
		up = wireProtocolFromUpstream(upstream.APIType)
	}
	switch client {
	case WireChat:
		switch up {
		case WireAnthropic:
			return streamKindAnthropicToChat
		case WireResponses:
			return streamKindResponsesToChat
		default:
			return streamKindChatPassthrough
		}
	case WireAnthropic:
		switch up {
		case WireResponses:
			return streamKindResponsesToAnthropic
		case WireChat:
			return streamKindChatToAnthropic
		default:
			return streamKindAnthropicPassthrough
		}
	case WireResponses:
		switch up {
		case WireAnthropic:
			return streamKindAnthropicToResponses
		case WireChat:
			return streamKindChatToResponses
		default:
			return streamKindResponsesPassthrough
		}
	default:
		return streamKindUnknown
	}
}

func clientProtocolFromPath(path string) WireProtocol {
	switch path {
	case "/v1/messages":
		return WireAnthropic
	case "/v1/responses":
		return WireResponses
	default:
		return WireChat
	}
}
