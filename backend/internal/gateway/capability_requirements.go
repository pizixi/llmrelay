package gateway

import (
	"encoding/json"
	"strings"
)

func requestCapabilities(body []byte, client WireProtocol) []Capability {
	var fields map[string]any
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return nil
	}
	var capabilities []Capability
	add := func(capability Capability) {
		for _, existing := range capabilities {
			if existing == capability {
				return
			}
		}
		capabilities = append(capabilities, capability)
	}
	if stream, _ := fields["stream"].(bool); stream {
		add(CapabilityStreaming)
	}
	if _, exists := fields["reasoning"]; exists {
		add(CapabilityReasoning)
	}
	if _, exists := fields["reasoning_effort"]; exists {
		add(CapabilityReasoning)
	}
	if _, exists := fields["thinking"]; exists {
		add(CapabilityReasoning)
	}
	if tools, ok := fields["tools"].([]any); ok && len(tools) > 0 {
		add(CapabilityToolCalls)
		for _, rawTool := range tools {
			tool, _ := rawTool.(map[string]any)
			typ, _ := tool["type"].(string)
			typ = strings.ToLower(strings.TrimSpace(typ))
			switch typ {
			case "custom":
				add(CapabilityCustomTools)
			case "web_search", "web_search_preview", "computer_use_preview", "tool_search":
				add(CapabilityHostedWebSearch)
			}
		}
	}
	if _, exists := fields["response_format"]; exists {
		add(CapabilityStructuredOutput)
	}
	if _, exists := fields["text"]; exists {
		add(CapabilityStructuredOutput)
	}
	if _, exists := fields["output_config"]; exists {
		add(CapabilityStructuredOutput)
	}
	if _, exists := fields["prompt_cache_key"]; exists {
		add(CapabilityPromptCaching)
	}
	if _, exists := fields["prompt_cache_retention"]; exists {
		add(CapabilityPromptCaching)
	}
	if _, exists := fields["prompt_cache_options"]; exists {
		add(CapabilityPromptCaching)
	}
	if containsJSONField(fields, "cache_control") {
		add(CapabilityPromptCaching)
	}
	if client == WireResponses {
		if value, _ := fields["previous_response_id"].(string); strings.TrimSpace(value) != "" {
			add(CapabilityStatefulContext)
		}
		if value, exists := fields["conversation"]; exists && value != nil {
			add(CapabilityStatefulContext)
		}
		if value, exists := fields["prompt"]; exists && value != nil {
			add(CapabilityStatefulContext)
		}
		if containsResponseItemReference(fields["input"]) {
			add(CapabilityItemReferences)
		}
		if stored, _ := fields["store"].(bool); stored {
			add(CapabilityResponseStore)
		}
		if background, _ := fields["background"].(bool); background {
			add(CapabilityBackground)
		}
		if include, ok := fields["include"].([]any); ok {
			for _, raw := range include {
				if value, _ := raw.(string); value == "reasoning.encrypted_content" {
					add(CapabilityEncryptedReason)
				}
			}
		}
	}
	return capabilities
}

func containsResponseItemReference(value any) bool {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if containsResponseItemReference(child) {
				return true
			}
		}
	case map[string]any:
		if typ, _ := item["type"].(string); typ == "item_reference" || typ == "response_reference" {
			return true
		}
		if id, _ := item["id"].(string); strings.TrimSpace(id) != "" {
			return true
		}
		for _, child := range item {
			if containsResponseItemReference(child) {
				return true
			}
		}
	}
	return false
}

func containsJSONField(value any, field string) bool {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if containsJSONField(child, field) {
				return true
			}
		}
	case map[string]any:
		if _, exists := item[field]; exists {
			return true
		}
		for _, child := range item {
			if containsJSONField(child, field) {
				return true
			}
		}
	}
	return false
}
