package catalog

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

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
