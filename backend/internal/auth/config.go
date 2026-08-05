package auth

import (
	"context"
	"net/http"
	"sync"

	"llmrelay/backend/internal/domain"
)

var (
	apiAccessKey  string
	apiKeys       []domain.APIKey
	apiKeysMu     sync.RWMutex
	loginRenderer = func(w http.ResponseWriter, _ string) {
		http.Error(w, "login page is not configured", http.StatusInternalServerError)
	}
)

func Configure(admin, apiKey string) {
	adminPassword = admin
	apiKeysMu.Lock()
	apiAccessKey = apiKey
	apiKeys = nil
	apiKeysMu.Unlock()
}

// SetAPIKeys replaces the managed public API credentials used by the gateway.
// The caller owns persistence; this function only updates the in-process
// authenticator after a successful configuration change.
func SetAPIKeys(values []domain.APIKey) {
	apiKeysMu.Lock()
	apiKeys = append([]domain.APIKey(nil), values...)
	apiKeysMu.Unlock()
}

type APIKeyIdentity struct {
	ID   string
	Name string
}

type apiKeyContextKey struct{}

func WithAPIKey(ctx context.Context, identity APIKeyIdentity) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, identity)
}

func APIKeyFromContext(ctx context.Context) (APIKeyIdentity, bool) {
	if ctx == nil {
		return APIKeyIdentity{}, false
	}
	identity, ok := ctx.Value(apiKeyContextKey{}).(APIKeyIdentity)
	return identity, ok && identity.ID != ""
}

func SetLoginRenderer(renderer func(http.ResponseWriter, string)) {
	if renderer != nil {
		loginRenderer = renderer
	}
}

func RenderLoginPage(w http.ResponseWriter, message string) { loginRenderer(w, message) }
