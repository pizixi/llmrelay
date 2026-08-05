package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
)

func TestAdminAPIKeysCRUD(t *testing.T) {
	previousPath := config.Path()
	previousConfig := currentConfig()
	t.Cleanup(func() {
		config.SetPath(previousPath)
		applyConfig(previousConfig)
		auth.Configure("", "")
	})
	config.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	applyConfig(AppConfig{
		ModelAlias:         map[string]domain.ModelAlias{},
		ReasoningEffortMap: map[string]string{},
		Upstreams:          map[string]*UpstreamConfig{},
	})
	auth.Configure("", "")
	auth.SetAPIKeys(nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/api-keys", bytes.NewBufferString(`{"name":"测试客户端"}`))
	request.Header.Set("Content-Type", "application/json")
	AdminAPIKeysHandler(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Key APIKey `json:"key"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Key.ID == "" || created.Key.Key == "" || created.Key.Name != "测试客户端" {
		t.Fatalf("unexpected created key: %#v", created.Key)
	}

	authorized := auth.RequireAPIAuth(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	checkRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	checkRequest.Header.Set("x-api-key", created.Key.Key)
	checkResponse := httptest.NewRecorder()
	authorized(checkResponse, checkRequest)
	if checkResponse.Code != http.StatusNoContent {
		t.Fatalf("created key auth status = %d", checkResponse.Code)
	}

	patchRecorder := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/api-keys/"+created.Key.ID, bytes.NewBufferString(`{"name":"已改名","disabled":true}`))
	patchRequest.Header.Set("Content-Type", "application/json")
	AdminAPIKeysHandler(patchRecorder, patchRequest)
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchRecorder.Code, patchRecorder.Body.String())
	}

	blockedResponse := httptest.NewRecorder()
	authorized(blockedResponse, checkRequest)
	if blockedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("disabled key auth status = %d", blockedResponse.Code)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/api-keys/"+created.Key.ID, nil)
	AdminAPIKeysHandler(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}
