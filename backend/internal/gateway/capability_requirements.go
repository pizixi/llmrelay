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
				// Anthropic custom 是 JSON Schema 函数工具；OpenAI custom
				// 才是自由格式工具，不能混用同一能力判断。
				if client != WireAnthropic {
					add(CapabilityCustomTools)
				}
			case "web_search", "web_search_preview", "computer_use_preview", "tool_search":
				add(CapabilityHostedWebSearch)
			}
		}
	}
	// format 才声明输出约束；text.verbosity 和 output_config.effort
	// 分别控制详细程度和推理，不能据此外推结构化输出能力。
	var outputFormat any
	switch client {
	case WireChat:
		outputFormat = fields["response_format"]
	case WireResponses:
		if text, ok := fields["text"].(map[string]any); ok {
			outputFormat = text["format"]
		}
	case WireAnthropic:
		if config, ok := fields["output_config"].(map[string]any); ok {
			outputFormat = config["format"]
			if effort, _ := config["effort"].(string); strings.TrimSpace(effort) != "" {
				add(CapabilityReasoning)
			}
		}
	}
	if format, ok := outputFormat.(map[string]any); ok && format["type"] != nil && format["type"] != "" && format["type"] != "text" {
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
