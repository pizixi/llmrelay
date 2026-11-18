package catalog

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
)

func TestFetchModelsAcceptsCompleteModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("request path = %q, want /v1/models", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"model-a"}]}`)
	}))
	defer server.Close()

	models, err := FetchModelsFromUpstream("example", &domain.UpstreamConfig{
		BaseURL: server.URL + "/v1/models",
		APIType: domain.UpstreamOpenAI,
	}, false)
	if err != nil {
		t.Fatalf("fetch models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "model-a" {
		t.Fatalf("models = %#v", models)
	}
}

func TestFetchModelsFallsBackFromRootToOpenAIV1(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"model-a"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	models, err := FetchModelsFromUpstream("example", &domain.UpstreamConfig{
		BaseURL: server.URL,
		APIType: domain.UpstreamOpenAI,
	}, false)
	if err != nil {
		t.Fatalf("fetch models: %v", err)
	}
	if len(models) != 1 || models[0].ID != "model-a" {
		t.Fatalf("models = %#v", models)
	}
	if want := []string{"/models", "/v1/models"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestDecodeUpstreamModelIDsAcceptsCommonShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{name: "OpenAI data", body: `{"data":[{"id":"model-a"},{"id":"model-a"}]}`, want: []string{"model-a"}},
		{name: "models with names", body: `{"models":[{"name":"model-b"}]}`, want: []string{"model-b"}},
		{name: "direct string array", body: `["model-c","model-d"]`, want: []string{"model-c", "model-d"}},
		{name: "BOM direct array", body: "\ufeff\n[\"model-e\"]", want: []string{"model-e"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeUpstreamModelIDs([]byte(test.body))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ids = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecodeUpstreamModelIDsRejectsUnrecognizedSuccessEnvelope(t *testing.T) {
	if _, err := decodeUpstreamModelIDs([]byte(`{"status":"ok"}`)); err == nil {
		t.Fatal("decode unexpectedly accepted an envelope without a model list")
	}
}

func TestGetRoutableModelInfosExposesOnlyAliasesByDefault(t *testing.T) {
	previous := config.Snapshot()
	t.Cleanup(func() { config.ApplyConfig(previous) })
	config.ApplyConfig(config.AppConfig{
		Upstreams: map[string]*domain.UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"upstream-only", "both"}},
		},
		UpstreamOrder: []string{"primary"},
		ModelAlias: map[string]config.ModelAlias{
			"public-only": {Targets: []config.ModelAliasTarget{{Upstream: "primary", TargetModel: "upstream-only", Weight: 1, Enabled: true}}},
			"both":        {Targets: []config.ModelAliasTarget{{Upstream: "primary", TargetModel: "upstream-only", Weight: 1, Enabled: true}}},
		},
	})

	var ids []string
	for _, model := range GetRoutableModelInfos() {
		ids = append(ids, model.ID)
	}
	if want := []string{"both", "public-only"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("model IDs = %#v, want %#v", ids, want)
	}
}

func TestGetRoutableModelInfosIncludesDirectModelsWhenEnabled(t *testing.T) {
	previous := config.Snapshot()
	t.Cleanup(func() { config.ApplyConfig(previous) })
	config.ApplyConfig(config.AppConfig{
		Upstreams: map[string]*domain.UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"upstream-only", "both"}},
		},
		UpstreamOrder:     []string{"primary"},
		AllowDirectModels: true,
		ModelAlias: map[string]config.ModelAlias{
			"both": {Targets: []config.ModelAliasTarget{{Upstream: "primary", TargetModel: "upstream-only", Weight: 1, Enabled: true}}},
		},
	})

	owners := map[string]string{}
	for _, model := range GetRoutableModelInfos() {
		owners[model.ID] = model.OwnedBy
	}
	want := map[string]string{"upstream-only": "primary", "both": "alias"}
	if !reflect.DeepEqual(owners, want) {
		t.Fatalf("models = %#v, want %#v", owners, want)
	}
}
