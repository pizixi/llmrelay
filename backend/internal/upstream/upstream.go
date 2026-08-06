package upstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"llmrelay/backend/internal/sse"
)

// ======================== 上游端点 ========================

func GetUpstreamEndpoint(upstream *UpstreamConfig) string {
	if upstream == nil || upstream.BaseURL == "" {
		return ""
	}
	base := strings.TrimRight(upstream.BaseURL, "/")
	appendEndpoint := func(endpoint string) string {
		if strings.HasSuffix(base, endpoint) {
			return base
		}
		return base + endpoint
	}
	switch upstream.APIType {
	case UpstreamOpenAI:
		return appendEndpoint("/chat/completions")
	case UpstreamAnthropic:
		if strings.HasSuffix(base, "/messages") {
			return base
		}
		if strings.HasSuffix(base, "/v1") {
			return base + "/messages"
		}
		return base + "/v1/messages"
	case UpstreamResponses:
		return appendEndpoint("/responses")
	default:
		return appendEndpoint("/chat/completions")
	}
}

func GetUpstreamModelsEndpoint(upstream *UpstreamConfig) string {
	if upstream == nil || upstream.BaseURL == "" {
		return ""
	}
	base := strings.TrimRight(upstream.BaseURL, "/")
	for _, suffix := range []string{"/chat/completions", "/messages", "/responses"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if upstream.APIType == UpstreamAnthropic && !strings.HasSuffix(base, "/v1") {
		return base + "/v1/models"
	}
	return base + "/models"
}

// ttfbReadCloser 包装 io.ReadCloser，在首次 Read 时记录首字节耗时，
// 后续调用全部委托给内部 ReadCloser。
type ttfbReadCloser struct {
	inner     io.ReadCloser
	once      sync.Once
	start     time.Time
	upstream  string
	model     string
	keySlot   string
	exitLabel string
}

func (r *ttfbReadCloser) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	r.once.Do(func() {
		ttfb := time.Since(r.start)
		log.Printf("[首字节耗时] 上游=%s 模型=%s 密钥=%s 出口=%s 耗时=%s", r.upstream, r.model, r.keySlot, r.exitLabel, ttfb.Round(time.Millisecond))
	})
	return n, err
}

func (r *ttfbReadCloser) Close() error {
	return r.inner.Close()
}

func BuildUpstreamRequest(endpoint, apiKey string, body []byte, upstream *UpstreamConfig) (*http.Request, error) {
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if upstream != nil && upstream.APIType == UpstreamAnthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("anthropic-beta", "prompt-caching-2025-01-31")
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	} else if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

type anthropicProtocolHeadersContextKey struct{}

type anthropicProtocolHeaders struct {
	Version string
	Beta    string
}

func WithAnthropicProtocolHeaders(ctx context.Context, headers http.Header) context.Context {
	protocolHeaders := anthropicProtocolHeaders{
		Version: strings.TrimSpace(headers.Get("anthropic-version")),
		Beta:    strings.TrimSpace(headers.Get("anthropic-beta")),
	}
	if protocolHeaders.Version == "" && protocolHeaders.Beta == "" {
		return ctx
	}
	return context.WithValue(ctx, anthropicProtocolHeadersContextKey{}, protocolHeaders)
}

func ApplyAnthropicProtocolHeadersFromContext(req *http.Request) {
	headers, ok := req.Context().Value(anthropicProtocolHeadersContextKey{}).(anthropicProtocolHeaders)
	if !ok {
		return
	}
	if headers.Version != "" {
		req.Header.Set("anthropic-version", headers.Version)
	}
	if headers.Beta != "" {
		req.Header.Set("anthropic-beta", headers.Beta)
	}
}

func PrepareOpenAIUpstreamBody(reqBody []byte, modelID string, upstream *UpstreamConfig) ([]byte, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(reqBody, &bodyMap); err != nil {
		return nil, fmt.Errorf("invalid request body")
	}
	bodyMap["model"] = modelID
	if upstream == nil || upstream.APIType != UpstreamAnthropic {
		for key := range bodyMap {
			if strings.HasPrefix(key, "_llm2api_") {
				delete(bodyMap, key)
			}
		}
	}
	marshaled, _ := json.Marshal(bodyMap)
	tryBody := marshaled
	if upstream != nil {
		switch upstream.APIType {
		case UpstreamAnthropic:
			tryBody = OpenAIToAnthropicRequest(marshaled)
		case UpstreamResponses:
			tryBody = OpenAIToResponsesRequest(marshaled, upstream)
		}
	}
	return tryBody, nil
}

func CallPreparedUpstream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawResponse ...bool) ([]byte, int, http.Header, error) {
	if upstream == nil || upstream.BaseURL == "" {
		return nil, 500, nil, fmt.Errorf("upstream not configured")
	}

	apiKey, apiKeyIndex, apiKeys := SelectUpstreamAPIKey(upstreamName, upstream)
	retryDelay := 1 * time.Second
	attemptLimit := UpstreamAttemptLimit(upstream, len(apiKeys))
	rateLimitRetriesByExit := make(map[string]int)
	for attempt := 0; attempt < attemptLimit; attempt++ {
		select {
		case <-ctx.Done():
			return nil, 0, nil, ctx.Err()
		default:
		}
		up, err := BuildUpstreamRequest(GetUpstreamEndpoint(upstream), apiKey, preparedBody, upstream)
		if err != nil {
			return nil, http.StatusInternalServerError, nil, fmt.Errorf("build upstream request: %w", err)
		}
		// 每次尝试都将请求重新绑定到当前调用的 context。BuildUpstreamRequest 已绑定 ctx，
		// 但这样即使辅助函数以后发生变化，取消信号仍具有最终决定权。
		up = up.WithContext(ctx)
		ApplyNativeProtocolHeadersFromContext(up)
		if upstream.APIType == UpstreamAnthropic {
			ApplyAnthropicProtocolHeadersFromContext(up)
		}
		startTTFB := time.Now()
		client, exitLabel := GetHTTPClientWithExit(false)
		resp, err := client.Do(up)
		if err != nil {
			if attempt+1 >= attemptLimit {
				return nil, http.StatusBadGateway, nil, fmt.Errorf("upstream request failed after %d attempt(s): %w", attempt+1, err)
			}
			if err := WaitForRetry(ctx, retryDelay, ""); err != nil {
				return nil, 0, nil, err
			}
			retryDelay = NextRetryDelay(retryDelay)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			ttfb := time.Since(startTTFB)
			log.Printf("[首字节耗时] 上游=%s 模型=%s 密钥=%s 出口=%s 耗时=%s", EffectiveUpstreamName(upstreamName), modelID, FormatUpstreamAPIKeySlot(apiKeyIndex, len(apiKeys)), exitLabel, ttfb.Round(time.Millisecond))
			b, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, 0, nil, readErr
			}
			if len(rawResponse) > 0 && rawResponse[0] {
				// rawResponse：跳过转换并原样返回。
			} else if upstream != nil && upstream.APIType == UpstreamResponses {
				b = ConvertResponsesToChat(b, modelID)
			} else if IsAnthropicFormat(b) {
				b = ConvertAnthropicToOpenAI(b, modelID)
			}
			return b, resp.StatusCode, resp.Header, nil
		}
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		preserveRaw := len(rawResponse) > 0 && rawResponse[0]
		if ShouldRetryUpstreamStatus(resp.StatusCode) {
			log.Printf("[上游重试] 上游=%s 模型=%s 密钥=%s 出口=%s 状态码=%d Retry-After=%q", EffectiveUpstreamName(upstreamName), modelID, FormatUpstreamAPIKeySlot(apiKeyIndex, len(apiKeys)), exitLabel, resp.StatusCode, resp.Header.Get("Retry-After"))
			if debugMode && debugLogBodies {
				log.Printf("[上游重试响应体] %s", TruncatePreview(string(errBody), 1024))
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				if rateLimitRetriesByExit[exitLabel] >= max429RetriesPerExit {
					if !preserveRaw {
						errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
					}
					return errBody, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream remained rate limited on exit %s after %d retries", exitLabel, max429RetriesPerExit)
				}
				rateLimitRetriesByExit[exitLabel]++
				RotateSocks5OnRateLimit()
			}
			if attempt+1 >= attemptLimit {
				if !preserveRaw {
					errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
				}
				return errBody, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream remained unavailable after %d attempt(s): %s", attempt+1, strings.TrimSpace(string(errBody)))
			}
			waitDelay := retryDelay
			if len(apiKeys) > 1 {
				apiKey, apiKeyIndex = RotateUpstreamAPIKey(apiKeys, apiKeyIndex)
				if strings.TrimSpace(resp.Header.Get("Retry-After")) == "" && waitDelay > 300*time.Millisecond {
					waitDelay = 300 * time.Millisecond
				}
			}
			if err := WaitForRetry(ctx, waitDelay, resp.Header.Get("Retry-After")); err != nil {
				return nil, 0, nil, err
			}
			retryDelay = NextRetryDelay(retryDelay)
			continue
		}
		// 不可重试错误：立即返回。同协议调用方保留供应商原生错误信封；
		// 桥接调用方接收转换层使用的规范化 OpenAI 风格信封。
		log.Printf("[上游错误] 上游=%s 模型=%s 状态码=%d 端点=%s", EffectiveUpstreamName(upstreamName), modelID, resp.StatusCode, GetUpstreamEndpoint(upstream))
		if !preserveRaw {
			errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
		}
		return errBody, resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream error: %s", string(errBody))
	}
	return nil, http.StatusBadGateway, nil, fmt.Errorf("upstream attempts exhausted")
}

func CallUpstream(ctx context.Context, reqBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawResponse ...bool) ([]byte, int, http.Header, error) {
	tryBody, err := PrepareOpenAIUpstreamBody(reqBody, modelID, upstream)
	if err != nil {
		return nil, 500, nil, err
	}
	return CallPreparedUpstream(ctx, tryBody, upstreamName, modelID, upstream, rawResponse...)
}

func CallPreparedUpstreamStream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawErrors ...bool) (io.ReadCloser, int, http.Header, error) {
	if upstream == nil || upstream.BaseURL == "" {
		return nil, 500, nil, fmt.Errorf("upstream not configured")
	}

	apiKey, apiKeyIndex, apiKeys := SelectUpstreamAPIKey(upstreamName, upstream)
	retryDelay := 1 * time.Second
	attemptLimit := UpstreamAttemptLimit(upstream, len(apiKeys))
	rateLimitRetriesByExit := make(map[string]int)
	for attempt := 0; attempt < attemptLimit; attempt++ {
		select {
		case <-ctx.Done():
			return nil, 0, nil, ctx.Err()
		default:
		}
		up, err := BuildUpstreamRequest(GetUpstreamEndpoint(upstream), apiKey, preparedBody, upstream)
		if err != nil {
			return nil, http.StatusInternalServerError, nil, fmt.Errorf("build upstream request: %w", err)
		}
		// 每次尝试都将请求重新绑定到当前调用的 context。BuildUpstreamRequest 已绑定 ctx，
		// 但这样即使辅助函数以后发生变化，取消信号仍具有最终决定权。
		up = up.WithContext(ctx)
		ApplyNativeProtocolHeadersFromContext(up)
		if upstream.APIType == UpstreamAnthropic {
			ApplyAnthropicProtocolHeadersFromContext(up)
		}
		startTTFB := time.Now()
		client, exitLabel := GetHTTPClientWithExit(true)
		resp, err := client.Do(up)
		if err != nil {
			if attempt+1 >= attemptLimit {
				return nil, http.StatusBadGateway, nil, fmt.Errorf("upstream stream request failed after %d attempt(s): %w", attempt+1, err)
			}
			if err := WaitForRetry(ctx, retryDelay, ""); err != nil {
				return nil, 0, nil, err
			}
			retryDelay = NextRetryDelay(retryDelay)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			wrappedBody := &ttfbReadCloser{
				inner:     resp.Body,
				start:     startTTFB,
				upstream:  EffectiveUpstreamName(upstreamName),
				model:     modelID,
				keySlot:   FormatUpstreamAPIKeySlot(apiKeyIndex, len(apiKeys)),
				exitLabel: exitLabel,
			}
			return wrappedBody, resp.StatusCode, resp.Header, nil
		}
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		preserveRaw := len(rawErrors) > 0 && rawErrors[0]
		if ShouldRetryUpstreamStatus(resp.StatusCode) {
			log.Printf("[上游重试] 上游=%s 模型=%s 密钥=%s 出口=%s 状态码=%d Retry-After=%q", EffectiveUpstreamName(upstreamName), modelID, FormatUpstreamAPIKeySlot(apiKeyIndex, len(apiKeys)), exitLabel, resp.StatusCode, resp.Header.Get("Retry-After"))
			if debugMode && debugLogBodies {
				log.Printf("[上游重试响应体] %s", TruncatePreview(string(errBody), 1024))
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				if rateLimitRetriesByExit[exitLabel] >= max429RetriesPerExit {
					if !preserveRaw {
						errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
					}
					return io.NopCloser(bytes.NewReader(errBody)), resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream remained rate limited on exit %s after %d retries", exitLabel, max429RetriesPerExit)
				}
				rateLimitRetriesByExit[exitLabel]++
				RotateSocks5OnRateLimit()
			}
			if attempt+1 >= attemptLimit {
				if !preserveRaw {
					errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
				}
				return io.NopCloser(bytes.NewReader(errBody)), resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream remained unavailable after %d attempt(s)", attempt+1)
			}
			waitDelay := retryDelay
			if len(apiKeys) > 1 {
				apiKey, apiKeyIndex = RotateUpstreamAPIKey(apiKeys, apiKeyIndex)
				if strings.TrimSpace(resp.Header.Get("Retry-After")) == "" && waitDelay > 300*time.Millisecond {
					waitDelay = 300 * time.Millisecond
				}
			}
			if err := WaitForRetry(ctx, waitDelay, resp.Header.Get("Retry-After")); err != nil {
				return nil, 0, nil, err
			}
			retryDelay = NextRetryDelay(retryDelay)
			continue
		}
		// 不可重试错误：立即返回。
		log.Printf("[上游错误] 上游=%s 模型=%s 状态码=%d 端点=%s", EffectiveUpstreamName(upstreamName), modelID, resp.StatusCode, GetUpstreamEndpoint(upstream))
		if !preserveRaw {
			errBody = MapUpstreamErrorBody(errBody, upstream.APIType)
		}
		return io.NopCloser(bytes.NewReader(errBody)), resp.StatusCode, resp.Header.Clone(), fmt.Errorf("upstream error")
	}
	return nil, http.StatusBadGateway, nil, fmt.Errorf("upstream stream attempts exhausted")
}

func CallUpstreamStream(ctx context.Context, reqBody []byte, upstreamName, modelID string, upstream *UpstreamConfig) (io.ReadCloser, int, http.Header, error) {
	tryBody, err := PrepareOpenAIUpstreamBody(reqBody, modelID, upstream)
	if err != nil {
		return nil, 500, nil, err
	}
	return CallPreparedUpstreamStream(ctx, tryBody, upstreamName, modelID, upstream)
}

func StripBillingHeaderText(s string) string {
	return strings.TrimSpace(reBillingHeader.ReplaceAllString(s, ""))
}

func StripBillingHeaderFromResponsesItems(items any) {
	arr, ok := items.([]any)
	if !ok {
		return
	}
	for _, item := range arr {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "system" && role != "developer" {
			continue
		}
		switch content := msg["content"].(type) {
		case string:
			msg["content"] = StripBillingHeaderText(content)
		case []any:
			for _, part := range content {
				pm, ok := part.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := pm["text"].(string); ok {
					pm["text"] = StripBillingHeaderText(text)
				}
			}
		}
	}
}

func PrepareResponsesPassthroughBody(body []byte, modelID string) ([]byte, error) {
	return PrepareResponsesPassthroughBodyWithEffort(body, modelID, nil, true)
}

func PrepareResponsesPassthroughBodyWithEffort(body []byte, modelID string, effortMap map[string]string, forwardReasoning bool) ([]byte, error) {
	return PatchNativeRequestBody(body, func(req map[string]any) bool {
		changed := SetNativeRequestField(req, "model", modelID)
		if !forwardReasoning {
			for _, key := range []string{"reasoning", "reasoning_effort"} {
				if _, exists := req[key]; exists {
					delete(req, key)
					changed = true
				}
			}
		} else if reasoning, ok := req["reasoning"].(map[string]any); ok {
			if effort, _ := reasoning["effort"].(string); effort != "" {
				mapped := MapConfiguredReasoningEffort(effort, effortMap)
				if mapped != effort {
					reasoning["effort"] = mapped
					changed = true
				}
			}
		}
		return changed
	})
}

func PrepareChatPassthroughBody(body []byte, modelID, reasoningEffort string, withReasoning bool) ([]byte, error) {
	return PatchNativeRequestBody(body, func(req map[string]any) bool {
		changed := SetNativeRequestField(req, "model", modelID)
		changed = EnsureChatToolMessageContent(req["messages"]) || changed
		if !withReasoning {
			if _, exists := req["thinking"]; exists {
				delete(req, "thinking")
				changed = true
			}
			if _, exists := req["reasoning_effort"]; exists {
				delete(req, "reasoning_effort")
				changed = true
			}
		}
		if extraBody, ok := req["extra_body"].(map[string]any); ok {
			for key, value := range extraBody {
				if !withReasoning && (key == "thinking" || key == "reasoning_effort") {
					continue
				}
				if _, exists := req[key]; !exists {
					req[key] = value
					changed = true
				}
			}
			delete(req, "extra_body")
			changed = true
		}
		if withReasoning && reasoningEffort != "" {
			changed = SetNativeRequestField(req, "reasoning_effort", reasoningEffort) || changed
		}
		// 流式 Chat 请求必须显式请求 usage 分块，否则兼容上游不会在流末尾
		// 返回 usage，导致同协议透传的用量统计输入/输出恒为 0。此处仅补齐
		// stream_options.include_usage，保留客户端已有的其它字段与自定义结构。
		if stream, _ := req["stream"].(bool); stream {
			streamOptions, _ := req["stream_options"].(map[string]any)
			if streamOptions == nil {
				streamOptions = map[string]any{}
				req["stream_options"] = streamOptions
				changed = true
			}
			if includeUsage, _ := streamOptions["include_usage"].(bool); !includeUsage {
				streamOptions["include_usage"] = true
				changed = true
			}
		}
		return changed
	})
}

func PrepareAnthropicPassthroughBody(body []byte, modelID string) ([]byte, error) {
	return PrepareAnthropicPassthroughBodyWithReasoning(body, modelID, true)
}

func PrepareAnthropicPassthroughBodyWithReasoning(body []byte, modelID string, forwardReasoning bool) ([]byte, error) {
	return PatchNativeRequestBody(body, func(req map[string]any) bool {
		changed := SetNativeRequestField(req, "model", modelID)
		if !forwardReasoning {
			if _, exists := req["thinking"]; exists {
				delete(req, "thinking")
				changed = true
			}
		}
		return changed
	})
}

// PatchNativeRequestBody 在路由控制字段未变化时，保持同协议请求逐字节一致。
// 当模型别名或网关显式覆盖要求修改时，UseNumber 会保留未知整数和小数值，
// 避免经 float64 往返转换造成静默变化。
func PatchNativeRequestBody(body []byte, mutate func(map[string]any) bool) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var req map[string]any
	if err := decoder.Decode(&req); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	if !mutate(req) {
		return body, nil
	}
	return json.Marshal(req)
}

func SetNativeRequestField(req map[string]any, key string, value any) bool {
	if current, exists := req[key]; exists && fmt.Sprint(current) == fmt.Sprint(value) {
		return false
	}
	req[key] = value
	return true
}

func EnsureChatToolMessageContent(rawMessages any) bool {
	messages, ok := rawMessages.([]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		_, hasContent := message["content"]
		contentMissing := !hasContent || message["content"] == nil
		if !contentMissing {
			continue
		}
		toolCalls, _ := message["tool_calls"].([]any)
		if (role == "assistant" && len(toolCalls) > 0) || role == "tool" {
			message["content"] = ""
			changed = true
		}
	}
	return changed
}

func ProxyResponsesPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	usageStats := NewRequestUsageAccumulator(model)
	defer usageStats.commit()
	return proxyNativePassthroughStream(w, body, func(event sse.Event) {
		if event.Data == "" || event.Data == "[DONE]" {
			return
		}
		var payload map[string]any
		if json.Unmarshal([]byte(event.Data), &payload) != nil {
			return
		}
		eventType, _ := payload["type"].(string)
		if eventType == "" {
			eventType = event.Name
		}
		if eventType == "response.completed" || eventType == "response.incomplete" {
			usageStats.observeMap(ExtractResponsesUsage(payload))
		}
	})
}

func SseDataPayload(line string) (string, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(trimmed, "data:") {
		return "", false
	}
	payload := strings.TrimPrefix(trimmed, "data:")
	payload = strings.TrimPrefix(payload, " ")
	return payload, true
}

func ProxyChatPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string, recordUsage ...bool) error {
	shouldRecordUsage := len(recordUsage) == 0 || recordUsage[0]
	var usageStats *requestUsageAccumulator
	if shouldRecordUsage {
		usageStats = NewRequestUsageAccumulator(model)
		defer usageStats.commit()
	}
	return proxyNativePassthroughStream(w, body, func(event sse.Event) {
		if usageStats == nil || event.Data == "" || event.Data == "[DONE]" {
			return
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(event.Data), &chunk) != nil {
			return
		}
		if usage, ok := chunk["usage"].(map[string]any); ok {
			usageStats.observeMap(usage)
		}
	})
}

func ProxyAnthropicPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	usageStats := NewRequestUsageAccumulator(model)
	defer usageStats.commit()
	return proxyNativePassthroughStream(w, body, func(event sse.Event) {
		if event.Data == "" {
			return
		}
		var payload map[string]any
		if json.Unmarshal([]byte(event.Data), &payload) != nil {
			return
		}
		if message, ok := payload["message"].(map[string]any); ok {
			if usage, ok := message["usage"].(map[string]any); ok {
				usageStats.observeMap(usage)
			}
		}
		if usage, ok := payload["usage"].(map[string]any); ok {
			usageStats.observeMap(usage)
		}
	})
}

// proxyNativePassthroughStream forwards every upstream read chunk immediately and
// feeds a bounded SSE observer as a side operation. Observation must never decide
// whether a raw chunk is written to the client.
func proxyNativePassthroughStream(w http.ResponseWriter, body io.ReadCloser, observe func(sse.Event)) error {
	if body == nil {
		return nil
	}
	defer body.Close()
	flusher, _ := w.(http.Flusher)
	parser := sse.NewParser(sse.DefaultMaxEventBytes)
	clientGone := false
	disconnectedBytes := 0
	buffer := make([]byte, 32*1024)
	consume := func(events []sse.Event) {
		if observe == nil {
			return
		}
		for _, event := range events {
			observe(event)
		}
	}
	for {
		n, err := body.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if clientGone {
				disconnectedBytes += n
				if disconnectedBytes > maxDisconnectedUsageDrainBytes {
					return nil
				}
			} else {
				if _, writeErr := w.Write(chunk); writeErr != nil {
					clientGone = true
				}
			}
			consume(parser.Feed(chunk))
			if !clientGone && flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			consume(parser.Flush())
			if err == io.EOF || clientGone {
				return nil
			}
			return err
		}
	}
}

// 下游断开连接后，允许有限地继续读取尾部，以统计最终用量，
// 但绝不持续排空无上限的模型响应。
const maxDisconnectedUsageDrainBytes = 1 << 20

func ExtractResponsesUsage(payload map[string]any) map[string]any {
	if u, ok := payload["usage"].(map[string]any); ok {
		return u
	}
	if resp, ok := payload["response"].(map[string]any); ok {
		if u, ok := resp["usage"].(map[string]any); ok {
			return u
		}
	}
	return nil
}

// StreamErrorObject 规范化 Chat、Anthropic 和 Responses 流使用的错误信封，
// 同时不丢弃供应商特有字段。
func StreamErrorObject(payload map[string]any, fallbackMessage string) map[string]any {
	var source map[string]any
	errorMessage := ""
	if raw, ok := payload["error"].(map[string]any); ok {
		source = raw
	} else if raw, ok := payload["error"].(string); ok {
		errorMessage = raw
	} else if response, ok := payload["response"].(map[string]any); ok {
		if raw, ok := response["error"].(map[string]any); ok {
			source = raw
		}
	}

	result := map[string]any{}
	for key, value := range source {
		result[key] = value
	}
	if message, ok := result["message"].(string); !ok || message == "" {
		if errorMessage != "" {
			result["message"] = errorMessage
		} else if message, ok := payload["message"].(string); ok && message != "" {
			result["message"] = message
		} else {
			result["message"] = fallbackMessage
		}
	}
	if errorType, ok := result["type"].(string); !ok || errorType == "" {
		if code, ok := result["code"].(string); ok && code != "" {
			result["type"] = code
		} else {
			result["type"] = "upstream_error"
		}
	}
	return result
}

func EmitOpenAIStreamError(w http.ResponseWriter, flusher http.Flusher, payload map[string]any, fallbackMessage string) {
	errorEnvelope := map[string]any{"error": StreamErrorObject(payload, fallbackMessage)}
	if usage := ResponsesUsageToChatUsage(ExtractResponsesUsage(payload)); usage != nil {
		errorEnvelope["usage"] = usage
	}
	data, err := json.Marshal(errorEnvelope)
	if err != nil {
		return
	}
	w.Write([]byte("data: " + string(data) + "\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

func ResponsesStreamToChatHandler(w http.ResponseWriter, respBody io.ReadCloser, model, usageModel string, recordUsage bool) {
	defer respBody.Close()
	var usageStats *requestUsageAccumulator
	if recordUsage {
		usageStats = NewRequestUsageAccumulator(usageModel)
		defer usageStats.commit()
	}
	SetSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(respBody)
	chunkID := "chatcmpl-" + RandomString(16)
	created := time.Now().Unix()
	currentEvent := ""
	roleSent := false
	doneSent := false
	terminalSeen := false
	streamFailed := false
	hasToolCalls := false
	toolIndexes := map[string]int{}
	customToolInputEmitted := map[string]bool{}
	nextToolIndex := 0

	emit := func(delta map[string]any, finishReason any, usage map[string]any) {
		if !roleSent {
			chunk := map[string]any{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
			}
			b, _ := json.Marshal(chunk)
			w.Write([]byte("data: " + string(b) + "\n\n"))
			roleSent = true
		}
		if delta == nil {
			delta = map[string]any{}
		}
		chunk := map[string]any{
			"id":      chunkID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finishReason}},
		}
		if usage != nil {
			chunk["usage"] = usage
		}
		b, _ := json.Marshal(chunk)
		w.Write([]byte("data: " + string(b) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	getToolIndex := func(item map[string]any) int {
		id, _ := item["id"].(string)
		if id == "" {
			id, _ = item["call_id"].(string)
		}
		if id == "" {
			id = fmt.Sprintf("tool_%d", nextToolIndex)
		}
		if idx, ok := toolIndexes[id]; ok {
			return idx
		}
		idx := nextToolIndex
		toolIndexes[id] = idx
		nextToolIndex++
		return idx
	}

readLoop:
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if strings.HasPrefix(trimmed, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				if data == "[DONE]" {
					if terminalSeen {
						w.Write([]byte("data: [DONE]\n\n"))
						if flusher != nil {
							flusher.Flush()
						}
						doneSent = true
					} else {
						EmitOpenAIStreamError(w, flusher, nil, "upstream Responses stream ended before a terminal response event")
						streamFailed = true
					}
					break readLoop
				}
				if data != "" {
					var payload map[string]any
					if json.Unmarshal([]byte(data), &payload) == nil {
						eventType, _ := payload["type"].(string)
						if eventType == "" {
							eventType = currentEvent
						}
						switch eventType {
						case "response.output_text.delta":
							if text, _ := payload["delta"].(string); text != "" {
								emit(map[string]any{"content": text}, nil, nil)
							}
						case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
							if text, _ := payload["delta"].(string); text != "" {
								emit(map[string]any{"reasoning_content": text}, nil, nil)
							}
						case "response.output_item.added":
							if item, ok := payload["item"].(map[string]any); ok {
								typ, _ := item["type"].(string)
								if typ == "function_call" || typ == "custom_tool_call" {
									idx := getToolIndex(item)
									callID, _ := item["call_id"].(string)
									if callID == "" {
										callID, _ = item["id"].(string)
									}
									name, _ := item["name"].(string)
									arguments := ""
									if typ == "custom_tool_call" {
										inputText, _ := item["input"].(string)
										if inputText != "" {
											encoded, _ := json.Marshal(map[string]any{"input": inputText})
											arguments = string(encoded)
											customToolInputEmitted[callID] = true
										}
									}
									hasToolCalls = true
									emit(map[string]any{"tool_calls": []map[string]any{{
										"index": float64(idx),
										"id":    callID,
										"type":  "function",
										"function": map[string]any{
											"name":      name,
											"arguments": arguments,
										},
									}}}, nil, nil)
								}
							}
						case "response.output_item.done":
							if item, ok := payload["item"].(map[string]any); ok {
								if typ, _ := item["type"].(string); typ == "custom_tool_call" {
									callID, _ := item["call_id"].(string)
									if callID == "" {
										callID, _ = item["id"].(string)
									}
									if !customToolInputEmitted[callID] {
										idx := getToolIndex(item)
										inputText, _ := item["input"].(string)
										if inputText != "" {
											encoded, _ := json.Marshal(map[string]any{"input": inputText})
											emit(map[string]any{"tool_calls": []map[string]any{{
												"index":    float64(idx),
												"function": map[string]any{"arguments": string(encoded)},
											}}}, nil, nil)
											customToolInputEmitted[callID] = true
										}
									}
								}
							}
						case "response.function_call_arguments.delta":
							itemID, _ := payload["item_id"].(string)
							if itemID == "" {
								itemID, _ = payload["call_id"].(string)
							}
							idx, ok := toolIndexes[itemID]
							if !ok {
								idx = nextToolIndex
								toolIndexes[itemID] = idx
								nextToolIndex++
							}
							if delta, _ := payload["delta"].(string); delta != "" {
								emit(map[string]any{"tool_calls": []map[string]any{{
									"index":    float64(idx),
									"function": map[string]any{"arguments": delta},
								}}}, nil, nil)
							}
						case "response.completed", "response.incomplete":
							terminalSeen = true
							usage := ResponsesUsageToChatUsage(ExtractResponsesUsage(payload))
							if usageStats != nil {
								usageStats.observeMap(usage)
							}
							finishReason := "stop"
							if hasToolCalls {
								finishReason = "tool_calls"
							}
							if eventType == "response.incomplete" {
								finishReason = "length"
							}
							emit(map[string]any{}, finishReason, usage)
						case "response.failed", "error", "response.error":
							if usageStats != nil {
								if usage := ExtractResponsesUsage(payload); usage != nil {
									usageStats.observeMap(usage)
								}
							}
							EmitOpenAIStreamError(w, flusher, payload, "upstream Responses stream failed")
							streamFailed = true
						}
						if streamFailed {
							break readLoop
						}
					}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				EmitOpenAIStreamError(w, flusher, map[string]any{"message": err.Error()}, "failed to read upstream Responses stream")
				streamFailed = true
			} else if !terminalSeen && !streamFailed {
				EmitOpenAIStreamError(w, flusher, nil, "upstream Responses stream ended before a terminal response event")
				streamFailed = true
			}
			break
		}
	}
	if !doneSent && !streamFailed {
		w.Write([]byte("data: [DONE]\n\n"))
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// ======================== 安全响应头过滤 ========================

var safeResponseHeaders = map[string]bool{
	"content-type":         true,
	"retry-after":          true,
	"request-id":           true,
	"x-request-id":         true,
	"openai-processing-ms": true,
	"openai-version":       true,
}

func IsSafeResponseHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if safeResponseHeaders[name] {
		return true
	}
	return strings.HasPrefix(name, "ratelimit-") ||
		strings.HasPrefix(name, "x-ratelimit-") ||
		strings.HasPrefix(name, "anthropic-ratelimit-")
}

func FilterResponseHeaders(h http.Header) http.Header {
	filtered := make(http.Header)
	for k, v := range h {
		if IsSafeResponseHeader(k) {
			filtered[k] = v
		}
	}
	return filtered
}

func CopyFilteredResponseHeaders(dst http.Header, src http.Header) {
	for k, values := range FilterResponseHeaders(src) {
		dst.Del(k)
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

func NormalizeUpstreamStatus(status int) int {
	if status < 100 || status > 999 {
		return http.StatusBadGateway
	}
	return status
}

func ApplyUpstreamErrorHeaders(w http.ResponseWriter, upstreamHeaders http.Header, status int) int {
	status = NormalizeUpstreamStatus(status)
	CopyFilteredResponseHeaders(w.Header(), upstreamHeaders)
	w.Header().Set("X-Upstream-Status", strconv.Itoa(status))
	if status == http.StatusTooManyRequests {
		w.Header().Set("X-Upstream-Rate-Limited", "true")
	}
	return status
}

// MapUpstreamErrorBody 将上游错误响应转换为标准 OpenAI 格式。
func MapUpstreamErrorBody(body []byte, upstreamType UpstreamType) []byte {
	if len(body) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		for _, line := range bytes.Split(trimmed, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
				continue
			}
			trimmed = payload
			break
		}
	}
	var parsed map[string]any
	if json.Unmarshal(trimmed, &parsed) != nil {
		return trimmed
	}
	if errObj, ok := parsed["error"].(map[string]any); ok {
		if _, ok := errObj["message"].(string); ok {
			if _, exists := errObj["type"]; !exists {
				errObj["type"] = string(upstreamType) + "_error"
			}
			b, _ := json.Marshal(map[string]any{
				"error": errObj,
			})
			return b
		}
	}
	// 顶层 message 字段。
	if msg, ok := parsed["message"].(string); ok {
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    parsed["type"],
				"code":    parsed["type"],
			},
		})
		return b
	}
	// msg 字段。
	if msg, ok := parsed["msg"].(string); ok {
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": msg,
			},
		})
		return b
	}
	return trimmed
}

func MapErrorBodyToClaude(body []byte, fallbackMessage string) []byte {
	var parsed map[string]any
	if json.Unmarshal(bytes.TrimSpace(body), &parsed) == nil {
		if parsed["type"] == "error" {
			if errObj, ok := parsed["error"].(map[string]any); ok && errObj["message"] != nil {
				return bytes.TrimSpace(body)
			}
		}
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if _, exists := errObj["type"]; !exists {
				errObj["type"] = "api_error"
			}
			if _, exists := errObj["message"]; !exists {
				errObj["message"] = fallbackMessage
			}
			result, _ := json.Marshal(map[string]any{"type": "error", "error": errObj})
			return result
		}
	}
	result, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "api_error", "message": fallbackMessage},
	})
	return result
}
