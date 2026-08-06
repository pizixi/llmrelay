package gateway

import "testing"

func TestExplicitCapabilityDeclarationRejectsPairwiseStrictPlan(t *testing.T) {
	upstream := &UpstreamConfig{
		APIType:      UpstreamAnthropic,
		Capabilities: map[string]bool{"streaming": false},
	}
	decision := decideProtocolBridge(WireChat, upstream, BridgeModeStrict)
	decision.EvaluateCapabilities(upstream, CapabilityStreaming)
	warnings := explicitCapabilityBridgeWarnings(decision)
	if len(warnings) != 1 || warnings[0].Code != "capability_rejected" || warnings[0].Path != "capabilities.streaming" {
		t.Fatalf("decision=%+v warnings=%#v", decision, warnings)
	}
}
