package gateway

import (
	"encoding/json"
	"strings"

	bridgestate "llmrelay/backend/internal/bridge/state"
	"llmrelay/backend/internal/sse"
)

// resolveLocalResponsesState only activates for a non-Responses upstream.
// Native Responses state remains provider-owned and is never shadowed by this
// process-local cache.
func resolveLocalResponsesState(fields map[string]any, upstream *UpstreamConfig) (string, []any) {
	if upstream == nil || upstream.APIType == UpstreamResponses || fields == nil {
		return "", nil
	}
	previousID, _ := fields["previous_response_id"].(string)
	previousID = strings.TrimSpace(previousID)
	if previousID == "" {
		return "", nil
	}
	items, ok := bridgestate.Default().ResolveOutputItems(previousID)
	if !ok {
		return previousID, nil
	}
	return previousID, items
}

func prependResponsesStateItems(input any, items []any) any {
	if len(items) == 0 {
		return input
	}
	merged := make([]any, 0, len(items)+1)
	merged = append(merged, items...)
	switch value := input.(type) {
	case nil:
		return merged
	case []any:
		return append(merged, value...)
	default:
		return append(merged, value)
	}
}

func replaceResponsesInput(body []byte, input any) []byte {
	// Keep every non-input field as its original JSON token. This matters for
	// provider extensions containing large integers: local state emulation
	// should not turn an otherwise untouched field into a float64 round-trip.
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return body
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return body
	}
	fields["input"] = encodedInput
	encoded, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return encoded
}

func removeBridgeWarning(warnings []BridgeWarning, code, path string) []BridgeWarning {
	if len(warnings) == 0 {
		return warnings
	}
	filtered := warnings[:0]
	for _, warning := range warnings {
		if warning.Code == code && warning.Path == path {
			continue
		}
		filtered = append(filtered, warning)
	}
	return filtered
}

func updateResponsesStoreResult(body []byte, stored bool, warnings []BridgeWarning) []byte {
	var response map[string]json.RawMessage
	if json.Unmarshal(body, &response) != nil || response == nil {
		return body
	}
	encodedStored, err := json.Marshal(stored)
	if err != nil {
		return body
	}
	response["store"] = encodedStored
	if len(warnings) > 0 {
		encodedWarnings, err := json.Marshal(warnings)
		if err != nil {
			return body
		}
		response["llm2api_warnings"] = encodedWarnings
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return body
	}
	return encoded
}

// storeResponsesStreamResponse observes a buffered converted Responses stream
// (for example the gateway Web Search fallback) and retains its terminal
// response without changing the bytes that will be sent to the client.
func storeResponsesStreamResponse(body []byte) bool {
	parser := sse.NewParser(sse.DefaultMaxEventBytes)
	events := parser.Feed(body)
	events = append(events, parser.Flush()...)
	for _, event := range events {
		if event.Data == "" || event.Data == "[DONE]" {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(event.Data), &payload) != nil {
			continue
		}
		eventType, _ := payload["type"].(string)
		if eventType == "" {
			eventType = event.Name
		}
		if eventType != "response.completed" && eventType != "response.incomplete" {
			continue
		}
		response, ok := payload["response"].(map[string]any)
		if !ok || response == nil {
			continue
		}
		if _, stored := bridgestate.Default().PutResponse(response); stored {
			return true
		}
	}
	return false
}
