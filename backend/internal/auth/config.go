package auth

import "net/http"

var (
	apiAccessKey  string
	loginRenderer = func(w http.ResponseWriter, _ string) {
		http.Error(w, "login page is not configured", http.StatusInternalServerError)
	}
)

func Configure(admin, apiKey string) {
	adminPassword = admin
	apiAccessKey = apiKey
}

func SetLoginRenderer(renderer func(http.ResponseWriter, string)) {
	if renderer != nil {
		loginRenderer = renderer
	}
}

func RenderLoginPage(w http.ResponseWriter, message string) { loginRenderer(w, message) }
