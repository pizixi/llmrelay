// Package anthropic 定义 Anthropic Messages 协议的数据结构。
package anthropic

type Request struct {
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	System        any       `json:"system,omitempty"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	Temperature   *float64  `json:"temperature,omitempty"`
	TopP          *float64  `json:"top_p,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
	Tools         []Tool    `json:"tools,omitempty"`
	ToolChoice    any       `json:"tool_choice,omitempty"`
	Metadata      any       `json:"metadata,omitempty"`
	Thinking      any       `json:"thinking,omitempty"`
	OutputConfig  any       `json:"output_config,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
	ServiceTier   any       `json:"service_tier,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Content struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
	Data      string `json:"data,omitempty"`
}

type Tool struct {
	Type         string `json:"type,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	InputSchema  any    `json:"input_schema"`
	UserLocation any    `json:"user_location,omitempty"`
}

type Response struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []Content `json:"content"`
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"`
	StopSequence any       `json:"stop_sequence"`
	Usage        *Usage    `json:"usage"`
}

type Usage struct {
	InputTokens              int              `json:"input_tokens"`
	OutputTokens             int              `json:"output_tokens"`
	CacheCreationInputTokens int              `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int              `json:"cache_read_input_tokens,omitempty"`
	ServerToolUse            *ServerToolUsage `json:"server_tool_use,omitempty"`
}

type ServerToolUsage struct {
	WebSearchRequests int `json:"web_search_requests"`
}
