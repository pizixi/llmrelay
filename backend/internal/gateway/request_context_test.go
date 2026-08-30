package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llmrelay/backend/internal/config"
)

func TestModelMappingUpdateCancelsInFlightStream(t *testing.T) {
	started := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("data: {\"choices\":[]}\n\n"))
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer upstream.Close()

	previous := config.Snapshot()
	t.Cleanup(func() { config.ApplyConfig(previous) })
	base := config.AppConfig{
		ModelAlias: map[string]config.ModelAlias{
			"chat": {TargetModel: "old-model", Upstream: "primary"},
		},
		Upstreams: map[string]*config.UpstreamConfig{
			"primary": {BaseURL: upstream.URL, APIType: config.UpstreamOpenAI},
		},
	}
	config.ApplyConfig(base)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"chat","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	done := make(chan struct{})
	go func() {
		ChatCompletionsHandler(recorder, request)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream stream did not start")
	}

	next := base
	next.ModelAlias = map[string]config.ModelAlias{
		"chat": {TargetModel: "new-model", Upstream: "primary"},
	}
	config.ApplyConfig(next)
	select {
	case <-upstreamCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight upstream stream was not canceled after mapping update")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("gateway handler did not finish after mapping update")
	}
}
