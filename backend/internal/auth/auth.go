package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"llmrelay/backend/internal/domain"
)

// ======================== 管理面板认证 ========================

var (
	adminPassword string
	sessions      = map[string]time.Time{}
	sessionsMu    sync.Mutex
)

const adminSessionLifetime = 24 * time.Hour

func RequestUsesHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if adminPassword == "" {
			next(w, r)
			return
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		sessionsMu.Lock()
		expiresAt, ok := sessions[cookie.Value]
		if ok && time.Now().After(expiresAt) {
			delete(sessions, cookie.Value)
			ok = false
		}
		sessionsMu.Unlock()
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func RequireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(r.Header.Get("x-api-key"))
		if provided == "" {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			if len(authorization) >= 7 && strings.EqualFold(authorization[:7], "Bearer ") {
				provided = strings.TrimSpace(authorization[7:])
			}
		}
		identity, configured := matchAPIKey(provided)
		if !configured {
			next(w, r)
			return
		}
		if identity.ID != "" {
			next(w, r.WithContext(WithAPIKey(r.Context(), identity)))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if r.URL.Path == "/v1/messages" {
			json.NewEncoder(w).Encode(map[string]any{
				"type":  "error",
				"error": map[string]any{"type": "authentication_error", "message": "invalid API key"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"type": "authentication_error", "message": "invalid API key"},
		})
	}
}

func matchAPIKey(provided string) (APIKeyIdentity, bool) {
	apiKeysMu.RLock()
	legacy := apiAccessKey
	managed := append([]domain.APIKey(nil), apiKeys...)
	apiKeysMu.RUnlock()
	configured := len(managed) > 0 || strings.TrimSpace(legacy) != ""
	for _, value := range managed {
		if value.Disabled || strings.TrimSpace(value.Key) == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(value.Key)) == 1 {
			return APIKeyIdentity{ID: value.ID, Name: value.Name}, configured
		}
	}
	if strings.TrimSpace(legacy) != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(legacy)) == 1 {
		return APIKeyIdentity{ID: "legacy-default", Name: "默认密钥"}, configured
	}
	return APIKeyIdentity{}, configured
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GenerateAPIKey() (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	return "sk-relay-" + token, nil
}

func GenerateAPIKeyID() (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	return "key_" + token[:16], nil
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if adminPassword == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			RenderLoginPage(w, "表单解析失败")
			return
		}
		if r.FormValue("password") != adminPassword {
			RenderLoginPage(w, "密码错误")
			return
		}
		token, err := GenerateToken()
		if err != nil {
			RenderLoginPage(w, "创建会话失败")
			return
		}
		sessionsMu.Lock()
		sessions[token] = time.Now().Add(adminSessionLifetime)
		sessionsMu.Unlock()
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   RequestUsesHTTPS(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(adminSessionLifetime.Seconds()),
		})
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	RenderLoginPage(w, "")
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		sessionsMu.Lock()
		delete(sessions, cookie.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, Secure: RequestUsesHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusFound)
}
