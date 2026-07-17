package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeRequestBodyStaysByteIdenticalWithoutPatch(t *testing.T) {
	raw := []byte("{ \n  \"model\" : \"same-model\", \"future\" : 900719925474099312345 \n}")
	for name, prepare := range map[string]func([]byte, string) ([]byte, error){
		"responses": prepareResponsesPassthroughBody,
		"anthropic": prepareAnthropicPassthroughBody,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := prepare(raw, "same-model")
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(raw) {
				t.Fatalf("native request changed without a patch:\n got %q\nwant %q", got, raw)
			}
		})
	}
}

func TestNativeModelPatchPreservesLargeUnknownNumbers(t *testing.T) {
	raw := []byte(`{"model":"alias","future":{"integer":900719925474099312345,"decimal":1.2300000000000000001}}`)
	got, err := prepareResponsesPassthroughBody(raw, "resolved")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(got)))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		t.Fatal(err)
	}
	future := body["future"].(map[string]any)
	if future["integer"].(json.Number).String() != "900719925474099312345" {
		t.Fatalf("large integer changed: %v", future["integer"])
	}
	if future["decimal"].(json.Number).String() != "1.2300000000000000001" {
		t.Fatalf("decimal changed: %v", future["decimal"])
	}
}

func TestServeNativeProtocolPreservesStatusBodyHeadersAndRequestMetadata(t *testing.T) {
	var gotIdempotency, gotProject string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdempotency = r.Header.Get("Idempotency-Key")
		gotProject = r.Header.Get("OpenAI-Project")
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("X-Request-Id", "req_native")
		w.Header().Set("X-Unsafe-Upstream", "secret")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, `{"error":{"type":"native_error","message":"native body"}}`)
	}))
	defer upstreamServer.Close()

	headers := make(http.Header)
	headers.Set("Idempotency-Key", "idem-1")
	headers.Set("OpenAI-Project", "project-1")
	headers.Set("Authorization", "Bearer client-secret")
	recorder := httptest.NewRecorder()
	serveNativeProtocol(recorder, nativeProxyRequest{
		Client: WireChat,
		Body:   []byte(`{"model":"m","messages":[]}`),
		Model:  "m",
		Upstream: &UpstreamConfig{
			BaseURL: upstreamServer.URL,
			APIType: UpstreamOpenAI,
		},
		RequestContext: withNativeProtocolHeaders(context.Background(), headers),
	})

	if recorder.Code != http.StatusTeapot {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusTeapot)
	}
	if recorder.Body.String() != `{"error":{"type":"native_error","message":"native body"}}` {
		t.Fatalf("body=%q", recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type=%q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Header().Get("X-Request-Id") != "req_native" {
		t.Fatalf("request id=%q", recorder.Header().Get("X-Request-Id"))
	}
	if recorder.Header().Get("X-Unsafe-Upstream") != "" {
		t.Fatalf("unsafe header leaked: %q", recorder.Header().Get("X-Unsafe-Upstream"))
	}
	if gotIdempotency != "idem-1" || gotProject != "project-1" {
		t.Fatalf("forwarded headers: idempotency=%q project=%q", gotIdempotency, gotProject)
	}
}
