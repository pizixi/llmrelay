// Package public 暴露网关的公开 API Handler。
package public

import (
	"net/http"

	"llmrelay/backend/internal/gateway"
)

func Chat(w http.ResponseWriter, r *http.Request) { gateway.ChatCompletionsHandler(w, r) }

func Anthropic(w http.ResponseWriter, r *http.Request) { gateway.ClaudeMessagesHandler(w, r) }

func Responses(w http.ResponseWriter, r *http.Request) { gateway.ResponsesHandler(w, r) }

func Models(w http.ResponseWriter, r *http.Request) { gateway.ListModelsHandler(w, r) }
