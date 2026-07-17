// Package httpapi 负责 HTTP 路由装配，不包含协议和上游业务逻辑。
package httpapi

import (
	"net/http"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/httpapi/admin"
	"llmrelay/backend/internal/httpapi/middleware"
	"llmrelay/backend/internal/httpapi/public"
	"llmrelay/backend/internal/websearch"
)

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", middleware.API(public.Chat))
	mux.HandleFunc("/v1/responses", middleware.API(public.Responses))
	mux.HandleFunc("/v1/messages", middleware.API(public.Anthropic))
	mux.HandleFunc("/v1/models", middleware.API(public.Models))
	mux.HandleFunc("/login", auth.LoginHandler)
	mux.HandleFunc("/logout", auth.LogoutHandler)
	mux.HandleFunc("/api/config", middleware.Admin(admin.AdminConfigHandler))
	mux.HandleFunc("/api/stats", middleware.Admin(admin.AdminStatsHandler))
	mux.HandleFunc("/api/reload", middleware.Admin(admin.ReloadHandler))
	mux.HandleFunc("/api/test-model", middleware.Admin(admin.AdminTestModelHandler))
	mux.HandleFunc("/api/searxng/instances", middleware.Admin(websearch.SearxngInstancesHandler))
	mux.Handle("/assets/", admin.FrontendAssetsHandler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			middleware.Admin(admin.AdminPageHandler)(w, r)
			return
		}
		http.NotFound(w, r)
	})
	return mux
}
