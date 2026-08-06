package bridge

import (
	"testing"

	"llmrelay/backend/internal/domain"
)

func TestBuildBridgePlanOrderAndFidelity(t *testing.T) {
	tests := []struct {
		name     string
		request  BridgePlanRequest
		path     BridgePath
		fidelity BridgeFidelity
	}{
		{name: "exact", request: BridgePlanRequest{Client: WireChat, Upstream: WireChat}, path: BridgePathPassthrough, fidelity: FidelityExact},
		{name: "patched", request: BridgePlanRequest{Client: WireChat, Upstream: WireChat, PatchRequired: true}, path: BridgePathPassthrough, fidelity: FidelityPatched},
		{name: "pairwise", request: BridgePlanRequest{Client: WireResponses, Upstream: WireAnthropic}, path: BridgePathPairwise, fidelity: FidelityConverted},
		{name: "pivot", request: BridgePlanRequest{Client: WireResponses, Upstream: WireAnthropic, ForcePivot: true}, path: BridgePathPivot, fidelity: FidelityEmulated},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan := BuildBridgePlan(testCase.request)
			if plan.Path != testCase.path || plan.Fidelity != testCase.fidelity {
				t.Fatalf("plan=%+v, want path=%q fidelity=%q", plan, testCase.path, testCase.fidelity)
			}
		})
	}
}

func TestBuildBridgePlanCapabilityOutcomes(t *testing.T) {
	plan := BuildBridgePlan(BridgePlanRequest{
		Client: WireResponses, Upstream: WireChat, Mode: domain.BridgeModeCompatible,
		Requirements: []Capability{CapabilityStatefulContext, CapabilityHostedWebSearch},
	})
	if plan.Fidelity != FidelityConverted {
		t.Fatalf("plan fidelity=%q, want converted", plan.Fidelity)
	}
	results := map[Capability]CapabilityOutcome{}
	for _, result := range plan.Capabilities {
		results[result.Capability] = result.Outcome
	}
	if results[CapabilityStatefulContext] != CapabilityEmulated {
		t.Fatalf("stateful outcome=%q, want emulated", results[CapabilityStatefulContext])
	}
	if results[CapabilityHostedWebSearch] != CapabilityDowngraded {
		t.Fatalf("search outcome=%q, want downgraded", results[CapabilityHostedWebSearch])
	}

	strict := BuildBridgePlan(BridgePlanRequest{
		Client: WireResponses, Upstream: WireChat, Mode: domain.BridgeModeStrict,
		Requirements: []Capability{CapabilityStatefulContext},
	})
	if strict.Fidelity != FidelityRejected || strict.Capabilities[0].Outcome != CapabilityRejected {
		t.Fatalf("strict plan=%+v, want rejection", strict)
	}

	declaredUnsupported := BuildBridgePlan(BridgePlanRequest{
		Client: WireChat, Upstream: WireChat,
		UpstreamConfig: &domain.UpstreamConfig{Capabilities: map[string]bool{"streaming": false}},
		Requirements:   []Capability{CapabilityStreaming},
	})
	if declaredUnsupported.Fidelity != FidelityRejected {
		t.Fatalf("declared unsupported plan=%+v, want rejection", declaredUnsupported)
	}
}

func TestCapabilityMatrixReturnsIndependentCopy(t *testing.T) {
	first := CapabilityMatrix()
	first[WireChat][WireChat][CapabilityStreaming] = CapabilityRejected
	second := CapabilityMatrix()
	if second[WireChat][WireChat][CapabilityStreaming] != CapabilityPreserved {
		t.Fatal("capability matrix was mutable across calls")
	}
}

func TestEvaluateCapabilitiesKeepsPivotRejection(t *testing.T) {
	decision := Decide(WireResponses, &domain.UpstreamConfig{APIType: domain.UpstreamOpenAI}, domain.BridgeModeStrict)
	decision.UsePivot()
	decision.EvaluateCapabilities(&domain.UpstreamConfig{APIType: domain.UpstreamOpenAI}, CapabilityEncryptedReason)
	if decision.Path != BridgePathPivot || decision.Fidelity != FidelityRejected || decision.Plan.Fidelity != FidelityRejected {
		t.Fatalf("decision=%+v, want rejected pivot", decision)
	}
}
