package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"llmrelay/backend/internal/bridge"
	"llmrelay/backend/internal/bridge/convert"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/protocol/responses"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/upstream"
)

type OpenAIRequest = chat.Request
type Message = chat.Message
type ToolCall = chat.ToolCall
type FunctionCall = chat.FunctionCall
type Tool = chat.Tool
type ToolFunction = chat.ToolFunction
type ClaudeTool = anthropic.Tool
type ClaudeContent = anthropic.Content
type ResponsesTool = responses.Tool
type ResponseToolNameMapping = responses.ToolNameMapping
type UpstreamConfig = domain.UpstreamConfig
type WebSearchConfig = domain.WebSearchConfig
type BridgeWarning = bridge.BridgeWarning

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses

	WireAnthropic   = bridge.WireAnthropic
	WireResponses   = bridge.WireResponses
	applicationName = "LLM Relay"
)

var runtimeConfig = struct {
	sync.RWMutex
	value WebSearchConfig
}{}

func SetConfig(config WebSearchConfig) {
	runtimeConfig.Lock()
	runtimeConfig.value = config
	runtimeConfig.Unlock()
	ResetHostedWebSearchCapabilityCache()
}

func GetWebSearchConfig() WebSearchConfig {
	runtimeConfig.RLock()
	defer runtimeConfig.RUnlock()
	return runtimeConfig.value
}

func BridgeString(value any) string { return convert.BridgeString(value) }

func BridgeArray(value any) []any { return convert.BridgeArray(value) }

func CloneAnyMap(source map[string]any) map[string]any { return convert.CloneAnyMap(source) }

func GetFloat(values map[string]any, keys ...string) (float64, bool) {
	return convert.GetFloat(values, keys...)
}

func AppendBridgeWarning(warnings []BridgeWarning, warning BridgeWarning) []BridgeWarning {
	return bridge.AppendWarning(warnings, warning)
}

func AppendBridgeWarnings(warnings []BridgeWarning, additions ...[]BridgeWarning) []BridgeWarning {
	return bridge.AppendWarnings(warnings, additions...)
}

func LookupResponseToolNameMapping(name string, mappings map[string]ResponseToolNameMapping) (ResponseToolNameMapping, bool) {
	return convert.LookupResponseToolNameMapping(name, mappings)
}

func BuildUpstreamBody(req *OpenAIRequest, withReasoning ...bool) []byte {
	return convert.BuildUpstreamBody(req, withReasoning...)
}

func CallUpstream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig, rawResponse ...bool) ([]byte, int, http.Header, error) {
	return upstream.CallUpstream(ctx, body, upstreamName, model, target, rawResponse...)
}

func EffectiveUpstreamName(name string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "default"
}

func RandomString(n int) string { return convert.RandomString(n) }

func SseDataPayload(line string) (string, bool) { return upstream.SseDataPayload(line) }

func SetSSEHeaders(header http.Header) { sse.SetHeaders(header) }

func EmitSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data map[string]any) {
	convert.EmitSSEEvent(w, flusher, event, data)
}

func GetHTTPClient(stream bool) *http.Client { return netproxy.Client(stream) }

func WriteClientAPIError(w http.ResponseWriter, client bridge.WireProtocol, status int, errorType, message string) {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	if errorType == "" {
		errorType = "api_error"
	}
	if message == "" {
		message = "request failed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if client == WireAnthropic {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": errorType, "message": message},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"type": errorType, "message": message},
	})
}
