package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func installRoutingTestConfig(t *testing.T, upstreamURL string) {
	t.Helper()
	configMu.Lock()
	oldAliases := modelAlias
	oldUpstreamCfgs := upstreamCfgs
	oldDefaultName := defaultUpstreamName
	oldUpstreamCfg := upstreamCfg
	modelAlias = map[string]ModelAlias{
		"alias-only": {TargetModel: "alias-target", Upstream: "mapped"},
		"both":       {TargetModel: "mapped-target", Upstream: "mapped"},
	}
	upstreamCfgs = map[string]*UpstreamConfig{
		"default": {
			BaseURL:      upstreamURL,
			APIType:      UpstreamOpenAI,
			CustomModels: []string{"default-only", "both"},
		},
		"mapped": {
			BaseURL: upstreamURL,
			APIType: UpstreamOpenAI,
		},
	}
	defaultUpstreamName = "default"
	upstreamCfg = cloneUpstreamConfig(upstreamCfgs[defaultUpstreamName])
	configMu.Unlock()
	syncLegacyConfig()
	t.Cleanup(func() {
		configMu.Lock()
		modelAlias = oldAliases
		upstreamCfgs = oldUpstreamCfgs
		defaultUpstreamName = oldDefaultName
		upstreamCfg = oldUpstreamCfg
		configMu.Unlock()
		syncLegacyConfig()
	})
}

func TestResolveRequestModelRoutingQuadrants(t *testing.T) {
	installRoutingTestConfig(t, "https://example.test/v1")
	tests := []struct {
		name         string
		requested    string
		wantMatched  bool
		wantModel    string
		wantUpstream string
	}{
		{name: "neither", requested: "missing", wantMatched: false, wantModel: "missing", wantUpstream: "default"},
		{name: "default only", requested: "default-only", wantMatched: true, wantModel: "default-only", wantUpstream: "default"},
		{name: "mapping only", requested: "alias-only", wantMatched: true, wantModel: "alias-target", wantUpstream: "mapped"},
		{name: "both prefers mapping", requested: "both", wantMatched: true, wantModel: "mapped-target", wantUpstream: "mapped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, _, upstreamName, upstream, _, matched := resolveRequestModel(tt.requested)
			if matched != tt.wantMatched || model != tt.wantModel || upstreamName != tt.wantUpstream {
				t.Fatalf("resolveRequestModel(%q) = model %q, upstream %q, matched %t; want %q, %q, %t", tt.requested, model, upstreamName, matched, tt.wantModel, tt.wantUpstream, tt.wantMatched)
			}
			if upstream == nil {
				t.Fatal("resolved upstream is nil")
			}
		})
	}
}

func TestResolveRequestModelAllowsAnyModelWhenDefaultAllowlistIsEmpty(t *testing.T) {
	installRoutingTestConfig(t, "https://example.test/v1")
	configMu.Lock()
	upstreamCfgs["default"].CustomModels = nil
	configMu.Unlock()
	syncLegacyConfig()

	model, alias, upstreamName, upstream, aliasMatched, matched := resolveRequestModel("deepseek-v4-flash")
	if !matched {
		t.Fatal("unmapped model must be allowed when the default upstream has no model allowlist")
	}
	if aliasMatched || model != "deepseek-v4-flash" || !reflect.DeepEqual(alias, ModelAlias{}) || upstreamName != "default" || upstream == nil {
		t.Fatalf("unexpected route: model=%q alias=%#v upstream=%q config=%#v", model, alias, upstreamName, upstream)
	}
}

func TestReasoningForwardingPolicyDistinguishesDirectModelsFromAliases(t *testing.T) {
	tests := []struct {
		name         string
		alias        ModelAlias
		aliasMatched bool
		want         bool
	}{
		{name: "direct model preserves explicit reasoning", aliasMatched: false, want: true},
		{name: "alias explicitly disables reasoning", aliasMatched: true, want: false},
		{name: "alias enables reasoning", alias: ModelAlias{WithReasoning: true}, aliasMatched: true, want: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := shouldForwardReasoningParameters(testCase.alias, testCase.aliasMatched); got != testCase.want {
				t.Fatalf("shouldForwardReasoningParameters()=%t, want %t", got, testCase.want)
			}
		})
	}
}

func TestAnthropicUnmappedModelReachesUnrestrictedDefaultUpstream(t *testing.T) {
	var calls atomic.Int32
	var upstreamPath string
	var upstreamBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		upstreamPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"deepseek-v4-flash","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	installRoutingTestConfig(t, server.URL+"/v1")
	configMu.Lock()
	upstreamCfgs["default"].APIType = UpstreamAnthropic
	upstreamCfgs["default"].CustomModels = nil
	upstreamCfg = cloneUpstreamConfig(upstreamCfgs["default"])
	configMu.Unlock()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"deepseek-v4-flash","max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`))
	claudeMessagesHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls.Load() != 1 || upstreamPath != "/v1/messages" {
		t.Fatalf("upstream calls=%d path=%q", calls.Load(), upstreamPath)
	}
	if upstreamBody["model"] != "deepseek-v4-flash" {
		t.Fatalf("upstream model=%#v", upstreamBody["model"])
	}
}

func TestUnknownModelIsRejectedBeforeEveryProtocolUpstreamCall(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	installRoutingTestConfig(t, server.URL)

	tests := []struct {
		name    string
		path    string
		body    string
		handler http.HandlerFunc
		wantTyp string
	}{
		{
			name:    "chat completions",
			path:    "/v1/chat/completions",
			body:    `{"model":"missing","messages":[{"role":"user","content":"hello"}]}`,
			handler: chatCompletionsHandler,
			wantTyp: "invalid_request_error",
		},
		{
			name:    "responses",
			path:    "/v1/responses",
			body:    `{"model":"missing","input":"hello"}`,
			handler: responsesHandler,
			wantTyp: "invalid_request_error",
		},
		{
			name:    "anthropic messages",
			path:    "/v1/messages",
			body:    `{"model":"missing","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`,
			handler: claudeMessagesHandler,
			wantTyp: "not_found_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			tt.handler(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid error JSON: %v", err)
			}
			errorObject, _ := payload["error"].(map[string]any)
			if errorObject["type"] != tt.wantTyp || !strings.Contains(errorObject["message"].(string), "missing") {
				t.Fatalf("error = %#v, want type %q mentioning model", errorObject, tt.wantTyp)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("unknown-model requests reached upstream %d time(s), want 0", got)
	}
}

func TestRoutableModelListIncludesDefaultModelsAndAliases(t *testing.T) {
	installRoutingTestConfig(t, "https://example.test/v1")
	models := getRoutableModelInfos()
	gotIDs := make([]string, len(models))
	gotOwners := make(map[string]string, len(models))
	for i, model := range models {
		gotIDs[i] = model.ID
		gotOwners[model.ID] = model.OwnedBy
	}
	if want := []string{"alias-only", "both", "default-only"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("model IDs = %#v, want %#v", gotIDs, want)
	}
	if gotOwners["both"] != "alias" {
		t.Fatalf("duplicate model owner = %q, want alias precedence", gotOwners["both"])
	}
}
