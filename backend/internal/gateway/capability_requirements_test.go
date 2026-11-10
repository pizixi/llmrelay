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

func TestRequestCapabilitiesSeparatesOutputFormatFromReasoning(t *testing.T) {
	tests := []struct {
		name       string
		client     WireProtocol
		body       string
		structured bool
		reasoning  bool
	}{
		{"anthropic effort", WireAnthropic, `{"output_config":{"effort":"high"}}`, false, true},
		{"anthropic format", WireAnthropic, `{"output_config":{"format":{"type":"json_schema"}}}`, true, false},
		{"anthropic both", WireAnthropic, `{"output_config":{"effort":"low","format":{"type":"json_schema"}}}`, true, true},
		{"anthropic empty", WireAnthropic, `{"output_config":{}}`, false, false},
		{"anthropic null", WireAnthropic, `{"output_config":null}`, false, false},
		{"responses verbosity", WireResponses, `{"text":{"verbosity":"low"}}`, false, false},
		{"responses plain text", WireResponses, `{"text":{"format":{"type":"text"}}}`, false, false},
		{"responses format", WireResponses, `{"text":{"format":{"type":"json_schema"}}}`, true, false},
		{"chat plain text", WireChat, `{"response_format":{"type":"text"}}`, false, false},
		{"chat null", WireChat, `{"response_format":null}`, false, false},
		{"chat json", WireChat, `{"response_format":{"type":"json_object"}}`, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestCapabilities([]byte(tt.body), tt.client)
			seen := map[Capability]bool{}
			for _, capability := range got {
				seen[capability] = true
			}
			if seen[CapabilityStructuredOutput] != tt.structured || seen[CapabilityReasoning] != tt.reasoning {
				t.Fatalf("capabilities=%v, want structured=%v reasoning=%v", got, tt.structured, tt.reasoning)
			}
		})
	}
}
