package admin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"llmrelay/backend/internal/bridge/convert"
)

// 只检查有限前缀，兼容错误的 Content-Type，同时保留 SSE 实时转发。
func modelTestResponseIsJSON(reader *bufio.Reader) bool {
	prefix, err := reader.Peek(1)
	if err != nil {
		return false
	}
	offset := 0
	if prefix[0] == 0xef {
		prefix, err = reader.Peek(3)
		if err != nil || !bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
			return false
		}
		offset = 3
	}
	for size := offset + 1; size <= reader.Size(); size++ {
		prefix, err = reader.Peek(size)
		if err != nil {
			return false
		}
		switch prefix[size-1] {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return prefix[size-1] == '{' || prefix[size-1] == '['
		}
	}
	return false
}

func proxyAdminModelTestResponse(w http.ResponseWriter, body io.ReadCloser, apiType UpstreamType, model string) {
	reader := bufio.NewReader(body)
	if modelTestResponseIsJSON(reader) {
		defer body.Close()
		raw, err := io.ReadAll(reader)
		if err != nil {
			writeAdminJSON(w, http.StatusBadGateway, map[string]any{"error": "failed to read upstream model test response"})
			return
		}
		raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil || payload == nil {
			writeAdminJSON(w, http.StatusBadGateway, map[string]any{"error": "invalid upstream model test JSON response"})
			return
		}
		// HTTP 200 也可能包含错误；先保留原始错误，再转换成功响应。
		if payload["error"] == nil {
			switch apiType {
			case UpstreamAnthropic:
				raw = convert.ConvertAnthropicToOpenAI(raw, model)
			case UpstreamResponses:
				raw = convert.ConvertResponsesToChat(raw, model)
			}
			if len(raw) == 0 {
				writeAdminJSON(w, http.StatusBadGateway, map[string]any{"error": "invalid upstream model test response"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
		return
	}

	// 前缀检查不消费数据；把缓冲区与原始 Close 一起交给流处理器。
	body = &struct {
		io.Reader
		io.Closer
	}{Reader: reader, Closer: body}
	switch apiType {
	case UpstreamAnthropic:
		anthropicStreamToChatHandler(w, body, model, "", false)
	case UpstreamResponses:
		responsesStreamToChatHandler(w, body, model, "", false)
	default:
		proxyAdminModelTestStream(w, body, model)
	}
}
