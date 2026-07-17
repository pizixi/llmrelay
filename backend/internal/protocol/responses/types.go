// Package responses 定义 OpenAI Responses 协议的数据结构。
package responses

import "llmrelay/backend/internal/protocol/chat"

type Request struct {
	Model             string         `json:"model"`
	Input             any            `json:"input"`
	Messages          []chat.Message `json:"messages,omitempty"`
	Instructions      string         `json:"instructions,omitempty"`
	Stream            bool           `json:"stream,omitempty"`
	Temperature       *float64       `json:"temperature,omitempty"`
	MaxTokens         int            `json:"max_output_tokens,omitempty"`
	TopP              *float64       `json:"top_p,omitempty"`
	FrequencyPenalty  *float64       `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64       `json:"presence_penalty,omitempty"`
	Reasoning         ReasonEffort   `json:"reasoning,omitempty"`
	Include           []string       `json:"include,omitempty"`
	Store             *bool          `json:"store,omitempty"`
	Tools             []Tool         `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Stop              any            `json:"stop,omitempty"`
	User              string         `json:"user,omitempty"`
	StreamOptions     any            `json:"stream_options,omitempty"`
	Metadata          any            `json:"metadata,omitempty"`
}

type Tool struct {
	Type              string             `json:"type"`
	Name              string             `json:"name,omitempty"`
	Description       string             `json:"description,omitempty"`
	Parameters        map[string]any     `json:"parameters,omitempty"`
	Strict            *bool              `json:"strict,omitempty"`
	Format            any                `json:"format,omitempty"`
	Function          *chat.ToolFunction `json:"function,omitempty"`
	Tools             []Tool             `json:"tools,omitempty"`
	SearchContextSize string             `json:"search_context_size,omitempty"`
	UserLocation      any                `json:"user_location,omitempty"`
	Filters           any                `json:"filters,omitempty"`
	Execution         string             `json:"execution,omitempty"`
}

type ToolNameMapping struct {
	Kind      string
	Namespace string
	Name      string
	Format    any
	Execution string
}

type ReasonEffort struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}
