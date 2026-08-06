package gateway

import (
	"testing"

	"llmrelay/backend/internal/bridge"
)

func TestDecideProtocolBridgePaths(t *testing.T) {
	cases := []struct {
		name    string
		client  WireProtocol
		apiType UpstreamType
		want    BridgePath
	}{
		{"chat_passthrough", WireChat, UpstreamOpenAI, BridgePathPassthrough},
		{"anthropic_passthrough", WireAnthropic, UpstreamAnthropic, BridgePathPassthrough},
		{"responses_passthrough", WireResponses, UpstreamResponses, BridgePathPassthrough},
		{"chat_to_anthropic", WireChat, UpstreamAnthropic, BridgePathPairwise},
		{"chat_to_responses", WireChat, UpstreamResponses, BridgePathPairwise},
		{"anthropic_to_chat", WireAnthropic, UpstreamOpenAI, BridgePathPairwise},
		{"responses_to_chat", WireResponses, UpstreamOpenAI, BridgePathPairwise},
		{"anthropic_to_responses", WireAnthropic, UpstreamResponses, BridgePathPairwise},
		{"responses_to_anthropic", WireResponses, UpstreamAnthropic, BridgePathPairwise},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideProtocolBridge(tc.client, &UpstreamConfig{APIType: tc.apiType}, BridgeModeCompatible)
			if got.Path != tc.want {
				t.Fatalf("path=%q want %q", got.Path, tc.want)
			}
		})
	}
}

func TestChooseStreamDispatchMatrix(t *testing.T) {
	cases := []struct {
		name    string
		client  WireProtocol
		apiType UpstreamType
		want    streamDispatchKind
	}{
		{"chat_passthrough", WireChat, UpstreamOpenAI, streamKindChatPassthrough},
		{"anthropic_passthrough", WireAnthropic, UpstreamAnthropic, streamKindAnthropicPassthrough},
		{"responses_passthrough", WireResponses, UpstreamResponses, streamKindResponsesPassthrough},
		{"chat_from_anthropic", WireChat, UpstreamAnthropic, streamKindAnthropicToChat},
		{"chat_from_responses", WireChat, UpstreamResponses, streamKindResponsesToChat},
		{"anthropic_from_chat", WireAnthropic, UpstreamOpenAI, streamKindChatToAnthropic},
		{"anthropic_from_responses", WireAnthropic, UpstreamResponses, streamKindResponsesToAnthropic},
		{"responses_from_chat", WireResponses, UpstreamOpenAI, streamKindChatToResponses},
		{"responses_from_anthropic", WireResponses, UpstreamAnthropic, streamKindAnthropicToResponses},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &UpstreamConfig{APIType: tc.apiType}
			decision := decideProtocolBridge(tc.client, upstream, BridgeModeCompatible)
			if got := chooseStreamDispatch(tc.client, decision, upstream); got != tc.want {
				t.Fatalf("dispatch=%q want %q (%s)", got, tc.want, decision)
			}
		})
	}
}

func TestStrictCapabilityLossBecomesARejectableWarning(t *testing.T) {
	upstream := &UpstreamConfig{
		APIType:      UpstreamOpenAI,
		Capabilities: map[string]bool{"streaming": false},
	}
	decision := decideProtocolBridge(
		WireChat,
		upstream,
		BridgeModeStrict,
	)
	decision.EvaluateCapabilities(upstream, bridge.CapabilityStreaming)
	warnings := bridge.CapabilityWarnings(decision)
	if len(warnings) != 1 || warnings[0].Code != "capability_rejected" || warnings[0].Path != "capabilities.streaming" {
		t.Fatalf("warnings=%#v decision=%#v", warnings, decision)
	}
}
