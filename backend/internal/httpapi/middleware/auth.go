// Package middleware 提供 HTTP 路由使用的鉴权中间件。
package middleware

import (
	"net/http"

	"llmrelay/backend/internal/auth"
)

func Admin(next http.HandlerFunc) http.HandlerFunc { return auth.RequireAuth(next) }

func API(next http.HandlerFunc) http.HandlerFunc { return auth.RequireAPIAuth(next) }
