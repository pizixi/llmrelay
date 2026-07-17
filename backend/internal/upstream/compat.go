package upstream

import (
	"context"
	"crypto/rand"
	"net/http"
	"regexp"
	"strings"

	"llmrelay/backend/internal/bridge/convert"
	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/stats"
)

type UpstreamType = domain.UpstreamType
type UpstreamConfig = domain.UpstreamConfig

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses
)

var (
	debugMode      bool
	debugLogBodies bool
)

func SetDebug(enabled, logBodies bool) {
	debugMode = enabled
	debugLogBodies = logBodies
}

func EffectiveUpstreamName(name string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "default"
}

func GetHTTPClient(stream bool) *http.Client { return netproxy.Client(stream) }

func GetHTTPClientWithExit(stream bool) (*http.Client, string) {
	return netproxy.ClientWithExit(stream)
}

func RotateSocks5OnRateLimit() { netproxy.RotateOnRateLimit() }

func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[b[i]%byte(len(letters))]
	}
	return string(b)
}

var reBillingHeader = regexp.MustCompile(`(?m)^x-anthropic-billing-header:\s*.*$`)

func OpenAIToAnthropicRequest(body []byte) []byte { return convert.OpenAIToAnthropicRequest(body) }

func OpenAIToResponsesRequest(body []byte, upstream *UpstreamConfig) []byte {
	return convert.OpenAIToResponsesRequest(body, upstream)
}

func ConvertResponsesToChat(body []byte, model string) []byte {
	return convert.ConvertResponsesToChat(body, model)
}

func IsAnthropicFormat(body []byte) bool { return convert.IsAnthropicFormat(body) }

func ConvertAnthropicToOpenAI(body []byte, model string) []byte {
	return convert.ConvertAnthropicToOpenAI(body, model)
}

func MapConfiguredReasoningEffort(effort string, configuredMaps ...map[string]string) string {
	return convert.MapConfiguredReasoningEffort(effort, configuredMaps...)
}

func TruncatePreview(value string, limit int) string { return convert.TruncatePreview(value, limit) }

func ResponsesUsageToChatUsage(usage map[string]any) map[string]any {
	return convert.ResponsesUsageToChatUsage(usage)
}

func GetFloat(values map[string]any, keys ...string) (float64, bool) {
	return convert.GetFloat(values, keys...)
}

func SetSSEHeaders(header http.Header) { sse.SetHeaders(header) }

type requestUsageAccumulator struct {
	inner *stats.RequestUsageAccumulator
}

func NewRequestUsageAccumulator(model string) *requestUsageAccumulator {
	return &requestUsageAccumulator{inner: stats.NewRequestUsageAccumulator(model)}
}

func (a *requestUsageAccumulator) observeMap(usage map[string]any) {
	if a != nil {
		a.inner.ObserveMap(usage)
	}
}

func (a *requestUsageAccumulator) commit() {
	if a != nil {
		a.inner.Commit()
	}
}

type nativeProtocolHeadersContextKey struct{}

// WithNativeProtocolHeaders 仅传递协议协商与幂等性请求头。
// 客户端凭据和逐跳请求头始终由网关管理。
func WithNativeProtocolHeaders(ctx context.Context, headers http.Header) context.Context {
	forwarded := make(http.Header)
	for _, name := range []string{
		"Idempotency-Key",
		"OpenAI-Organization",
		"OpenAI-Project",
		"OpenAI-Beta",
		"Anthropic-Version",
		"Anthropic-Beta",
		"User-Agent",
	} {
		for _, value := range headers.Values(name) {
			if strings.TrimSpace(value) != "" {
				forwarded.Add(name, value)
			}
		}
	}
	if len(forwarded) == 0 {
		return ctx
	}
	return context.WithValue(ctx, nativeProtocolHeadersContextKey{}, forwarded)
}

func ApplyNativeProtocolHeadersFromContext(req *http.Request) {
	forwarded, ok := req.Context().Value(nativeProtocolHeadersContextKey{}).(http.Header)
	if !ok {
		return
	}
	for name, values := range forwarded {
		req.Header.Del(name)
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
}
