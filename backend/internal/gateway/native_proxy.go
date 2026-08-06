package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type NativeRequestEnvelope struct {
	Model           string `json:"model"`
	Stream          bool   `json:"stream"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

func DecodeNativeRequestEnvelope(body []byte) (NativeRequestEnvelope, error) {
	var envelope NativeRequestEnvelope
	err := json.Unmarshal(body, &envelope)
	return envelope, err
}

type NativeProxyRequest struct {
	Client         WireProtocol
	Body           []byte
	UpstreamName   string
	Model          string
	UsageModel     string
	Stream         bool
	Upstream       *UpstreamConfig
	RequestContext context.Context
}

// ServeNativeProtocol 执行同协议请求，不经过任何协议适配器解码。
// 响应状态、错误信封和安全的供应商响应头均保持原生格式；用量观测仅是附带操作。
func ServeNativeProtocol(w http.ResponseWriter, request NativeProxyRequest) {
	ctx := request.RequestContext
	if ctx == nil {
		ctx = context.Background()
	}
	usageModel := request.UsageModel
	if usageModel == "" {
		usageModel = request.Model
	}

	if request.Stream {
		body, status, headers, err := callPreparedUpstreamStream(
			ctx, request.Body, request.UpstreamName, request.Model, request.Upstream, true,
		)
		status = normalizeUpstreamStatus(status)
		copyFilteredResponseHeaders(w.Header(), headers)
		if err != nil || status < 200 || status >= 300 {
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(status)
			if body != nil {
				defer body.Close()
				_, _ = io.Copy(w, body)
			}
			return
		}
		if !nativeResponseIsSSE(headers) {
			// Some compatible upstreams return a normal JSON body even when the
			// request asked for streaming. Preserve that native response instead
			// of changing its Content-Type to SSE.
			w.WriteHeader(status)
			if body != nil {
				defer body.Close()
				_, _ = io.Copy(w, body)
			}
			return
		}
		setNativeSSEHeaders(w.Header())
		w.WriteHeader(status)
		usageRoute := usageIdentityForContext(ctx, usageModel, request.UpstreamName, request.Model)
		switch request.Client {
		case WireAnthropic:
			_ = proxyAnthropicPassthroughStream(w, body, usageRoute)
		case WireResponses:
			_ = proxyResponsesPassthroughStream(w, body, usageRoute)
		default:
			_ = proxyChatPassthroughStream(w, body, usageRoute)
		}
		return
	}

	body, status, headers, _ := callPreparedUpstream(
		ctx, request.Body, request.UpstreamName, request.Model, request.Upstream, true,
	)
	status = normalizeUpstreamStatus(status)
	copyFilteredResponseHeaders(w.Header(), headers)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	if status >= 200 && status < 300 {
		usageStats := newRequestUsageAccumulatorForContext(ctx, usageModel, request.UpstreamName, request.Model)
		var decoded map[string]any
		if json.Unmarshal(body, &decoded) == nil {
			if usage, ok := decoded["usage"].(map[string]any); ok {
				usageStats.observeMap(usage)
			}
		}
		usageStats.commit()
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func nativeResponseIsSSE(headers http.Header) bool {
	contentType := strings.ToLower(strings.TrimSpace(headers.Get("Content-Type")))
	return contentType == "" || strings.HasPrefix(contentType, "text/event-stream")
}

func setNativeSSEHeaders(header http.Header) {
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "text/event-stream")
	}
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-cache")
	}
	if header.Get("Connection") == "" {
		header.Set("Connection", "keep-alive")
	}
	if header.Get("X-Accel-Buffering") == "" {
		header.Set("X-Accel-Buffering", "no")
	}
}
