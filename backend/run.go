// Package backend 暴露 LLM Relay 后端的稳定启动入口。
package backend

import (
	"embed"

	"llmrelay/backend/internal/app"
)

// Run 配置并启动 LLM Relay HTTP 服务。
func Run(assets embed.FS) { app.Run(assets) }
