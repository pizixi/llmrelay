package bridge

import "fmt"

// OpenAIOutputFormatToAnthropic 转换 Chat 的 response_format 或 Responses 的
// text.format。只重排协议外层，保留原始 Schema 的所有约束，由上游验证支持范围。
func OpenAIOutputFormatToAnthropic(value any, path string) (map[string]any, []BridgeWarning) {
	if value == nil {
		return nil, nil
	}
	var warnings []BridgeWarning
	warn := func(code, field, message string) {
		warnings = appendBridgeWarning(warnings, BridgeWarning{Code: code, Path: field, Message: message})
	}
	format, ok := value.(map[string]any)
	if !ok {
		warn("request_field_ignored", path, "OpenAI output format must be an object and was ignored")
		return nil, warnings
	}
	switch typ := bridgeString(format["type"]); typ {
	case "", "text":
		return nil, nil
	case "json_schema":
		if path == "response_format" {
			format, _ = format["json_schema"].(map[string]any)
			path += ".json_schema"
		}
		if schema, ok := format["schema"].(map[string]any); !ok || schema == nil {
			warn("request_field_ignored", path+".schema", "JSON output schema must be an object and was ignored")
			return nil, warnings
		}
		if strict, ok := format["strict"].(bool); ok && !strict {
			warn("structured_output_strictness_changed", path+".strict", "Anthropic JSON output always enforces the schema; explicit strict=false cannot be preserved")
		}
		if description := bridgeString(format["description"]); description != "" {
			warn("output_hint_ignored", path+".description", "output format description has no Anthropic equivalent and was omitted; descriptions inside the schema were retained")
		}
		return map[string]any{"type": "json_schema", "schema": format["schema"]}, warnings
	case "json_object":
		warn("request_field_ignored", path, "schema-free JSON object mode has no native Anthropic equivalent and was ignored; supply a json_schema to preserve structured output")
	default:
		warn("request_field_ignored", path, fmt.Sprintf("OpenAI output format type %q has no Anthropic equivalent and was ignored", typ))
	}
	return nil, warnings
}

// AnthropicOutputFormatToOpenAI 返回 Responses 使用的平铺格式。Anthropic 原生
// JSON 输出执行 Schema 约束，因此映射为 strict=true，并补齐 OpenAI 必需的 name。
func AnthropicOutputFormatToOpenAI(value any) (map[string]any, []BridgeWarning) {
	if value == nil {
		return nil, nil
	}
	warn := func(path, message string) (map[string]any, []BridgeWarning) {
		return nil, appendBridgeWarning(nil, BridgeWarning{Code: "request_field_ignored", Path: path, Message: message})
	}
	config, ok := value.(map[string]any)
	if !ok {
		return warn("output_config", "Anthropic output_config must be an object and was ignored")
	}
	value = config["format"]
	if value == nil {
		return nil, nil
	}
	format, ok := value.(map[string]any)
	if !ok {
		return warn("output_config.format", "Anthropic output format must be an object and was ignored")
	}
	if bridgeString(format["type"]) != "json_schema" {
		return warn("output_config.format", fmt.Sprintf("Anthropic output format type %q has no OpenAI equivalent and was ignored", bridgeString(format["type"])))
	}
	if schema, ok := format["schema"].(map[string]any); !ok || schema == nil {
		return warn("output_config.format.schema", "JSON output schema must be an object and was ignored")
	}
	return map[string]any{"type": "json_schema", "name": "response", "strict": true, "schema": format["schema"]}, nil
}

func AnthropicOutputConfigToChatResponseFormat(value any) (map[string]any, []BridgeWarning) {
	format, warnings := AnthropicOutputFormatToOpenAI(value)
	if format == nil {
		return nil, warnings
	}
	delete(format, "type")
	return map[string]any{"type": "json_schema", "json_schema": format}, warnings
}
