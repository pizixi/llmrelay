// Package chat 定义 OpenAI Chat Completions 协议的数据结构。
package chat

import "encoding/json"

type Request struct {
	Model           string         `json:"model"`
	Messages        []Message      `json:"messages"`
	Stream          bool           `json:"stream"`
	Temperature     *float64       `json:"temperature,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	TopP            *float64       `json:"top_p,omitempty"`
	Thinking        any            `json:"thinking,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	ExtraBody       map[string]any `json:"extra_body,omitempty"`
	StreamOptions   any            `json:"stream_options,omitempty"`
	Tools           []Tool         `json:"tools,omitempty"`
	ToolChoice      any            `json:"tool_choice,omitempty"`
	// AdditionalFields 保留网关未显式建模的标准或供应商特有 Chat 字段。
	// 缺少它时，即使 OpenAI 到 OpenAI 的请求也会静默丢失 stop、response_format、
	// seed、logprobs、penalties、modalities、service_tier 和 user 等字段。
	AdditionalFields map[string]any `json:"-"`
	// RawTools 保留原始工具对象，用于同协议转发。
	// Tools 则保留跨协议适配器所用的规范化函数工具视图。
	RawTools []any `json:"-"`
	// ConfiguredReasoningEffortMap 在解析模型别名后选定。
	// 非 nil 映射对当前请求具有最终决定权，包括空映射。
	ConfiguredReasoningEffortMap map[string]string `json:"-"`
}

const InternalAnthropicRequestKey = "_llm2api_anthropic_request"

func (r *Request) UnmarshalJSON(data []byte) error {
	type requestAlias Request
	var decoded requestAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Request(decoded)

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if rawTools, ok := fields["tools"].([]any); ok {
		r.RawTools = rawTools
	}
	for _, key := range []string{
		"model", "messages", "stream", "temperature", "max_tokens", "top_p",
		"thinking", "reasoning_effort", "extra_body", "stream_options", "tools", "tool_choice",
	} {
		delete(fields, key)
	}
	if len(fields) > 0 {
		r.AdditionalFields = fields
	}
	return nil
}

type Message struct {
	Role                      string         `json:"role,omitempty"`
	Content                   any            `json:"content,omitempty"`
	ToolCalls                 []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID                string         `json:"tool_call_id,omitempty"`
	Name                      string         `json:"name,omitempty"`
	ReasoningContent          *string        `json:"reasoning_content,omitempty"`
	ReasoningSignature        string         `json:"reasoning_signature,omitempty"`
	ReasoningEncryptedContent string         `json:"reasoning_encrypted_content,omitempty"`
	AdditionalFields          map[string]any `json:"-"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	type messageAlias Message
	var decoded messageAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Message(decoded)
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{
		"role", "content", "tool_calls", "tool_call_id", "name",
		"reasoning_content", "reasoning_signature", "reasoning_encrypted_content",
	} {
		delete(fields, key)
	}
	if len(fields) > 0 {
		m.AdditionalFields = fields
	}
	return nil
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      *bool          `json:"strict,omitempty"`
}
