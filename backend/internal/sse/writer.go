// Package sse 提供网关流式响应共用的 SSE HTTP 设置。
package sse

import "net/http"

func SetHeaders(header http.Header) {
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
}
