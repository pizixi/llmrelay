package gateway

import (
	"encoding/json"
	"testing"
)

func TestRequestCapabilitiesRecognizesProtocolSpecificFeatures(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"stream":        true,
		"output_config": map[string]any{"format": map[string]any{"type": "json_schema"}},
		"messages": []any{map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "cache_control": map[string]any{"type": "ephemeral"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := requestCapabilities(body, WireAnthropic)
	seen := make(map[Capability]bool, len(got))
	for _, capability := range got {
		seen[capability] = true
	}
	for _, want := range []Capability{CapabilityStreaming, CapabilityStructuredOutput, CapabilityPromptCaching} {
		if !seen[want] {
			t.Fatalf("capability %q missing from %#v", want, got)
		}
	}
}
