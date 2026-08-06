package gateway

import (
	"bufio"
	"bytes"
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
		body, isSSE := sniffNativeResponse(headers, body)
		if !isSSE {
			// Some compatible upstreams return a normal JSON body even when the
			// request asked for streaming. Preserve that native response instead
			// of passing JSON through as SSE when the upstream header is wrong.
			setNativeJSONHeaders(w.Header())
			var responseBody bytes.Buffer
			w.WriteHeader(status)
			if body != nil {
				_, _ = io.Copy(w, io.TeeReader(body, &responseBody))
				_ = body.Close()
			}
			commitNativeResponseUsage(ctx, usageModel, request.UpstreamName, request.Model, responseBody.Bytes())
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
		commitNativeResponseUsage(ctx, usageModel, request.UpstreamName, request.Model, body)
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func commitNativeResponseUsage(ctx context.Context, usageModel, upstreamName, upstreamModel string, body []byte) {
	usageStats := newRequestUsageAccumulatorForContext(ctx, usageModel, upstreamName, upstreamModel)
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) == nil {
		if usage := nativeResponseUsage(decoded); usage != nil {
			usageStats.observeMap(usage)
		}
	}
	usageStats.commit()
}

func nativeResponseUsage(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	if usage, ok := payload["usage"].(map[string]any); ok {
		return usage
	}
	for _, key := range []string{"response", "message"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if usage, ok := nested["usage"].(map[string]any); ok {
				return usage
			}
		}
	}
	return nil
}

func nativeResponseIsSSE(headers http.Header) bool {
	contentType := strings.ToLower(strings.TrimSpace(headers.Get("Content-Type")))
	return contentType == "" || strings.HasPrefix(contentType, "text/event-stream")
}

const nativeResponseSniffLimit = 4 * 1024

type nativeResponseReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *nativeResponseReadCloser) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// sniffNativeResponse handles providers that advertise the wrong response
// content type. It reads only a small prefix, then replays that prefix before
// the remaining response body so streaming responses are not buffered.
func sniffNativeResponse(headers http.Header, body io.ReadCloser) (io.ReadCloser, bool) {
	isSSE := nativeResponseIsSSE(headers)
	if body == nil {
		return nil, isSSE
	}

	reader := bufio.NewReader(body)
	prefix := make([]byte, 0, 128)
	for len(prefix) < nativeResponseSniffLimit {
		value, err := reader.ReadByte()
		if err != nil {
			break
		}
		prefix = append(prefix, value)
		if kind, ok := nativeResponsePrefixKind(prefix); ok {
			isSSE = kind
			break
		}
	}
	if kind, ok := nativeResponsePrefixKind(prefix); ok {
		isSSE = kind
	}

	return &nativeResponseReadCloser{
		Reader: io.MultiReader(bytes.NewReader(prefix), reader),
		closer: body,
	}, isSSE
}

func nativeResponsePrefixKind(prefix []byte) (isSSE bool, known bool) {
	trimmed := bytes.TrimPrefix(prefix, []byte{0xef, 0xbb, 0xbf})
	trimmed = bytes.TrimLeft(trimmed, " \t\r\n")
	if len(trimmed) == 0 {
		return false, false
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return false, true
	}
	for _, marker := range []string{"data:", "event:", "id:", "retry:", ":"} {
		if bytes.HasPrefix(trimmed, []byte(marker)) {
			return true, true
		}
	}
	return false, false
}

func setNativeSSEHeaders(header http.Header) {
	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "text/event-stream") {
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

func setNativeJSONHeaders(header http.Header) {
	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	if contentType == "" || strings.HasPrefix(contentType, "text/event-stream") ||
		(!strings.HasPrefix(contentType, "application/json") && !strings.HasSuffix(contentType, "+json")) {
		header.Set("Content-Type", "application/json")
	}
}
