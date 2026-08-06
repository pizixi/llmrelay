package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/stats"
)

func TestServeNativeProtocolRecordsJSONResponseForStreamRequest(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const responseBody = `{"id":"chatcmpl_test","object":"chat.completion","usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	ServeNativeProtocol(response, NativeProxyRequest{
		Client:         WireChat,
		Body:           []byte(`{"model":"gpt-test","stream":true}`),
		UpstreamName:   "primary",
		Model:          "gpt-test",
		UsageModel:     "chat-alias",
		Stream:         true,
		Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
		RequestContext: context.Background(),
	})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != responseBody {
		t.Fatalf("response body = %q, want %q", response.Body.String(), responseBody)
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.RequestModel != "chat-alias" || item.UpstreamName != "primary" || item.UpstreamModel != "gpt-test" {
		t.Fatalf("usage identity = %#v", item)
	}
	if item.RequestCount != 1 || item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v", item)
	}
}

func TestServeNativeProtocolSniffsJSONWhenStreamResponseOmitsContentType(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const responseBody = `{"id":"chatcmpl_test","object":"chat.completion","usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	ServeNativeProtocol(response, NativeProxyRequest{
		Client:         WireChat,
		Body:           []byte(`{"model":"gpt-test","stream":true}`),
		UpstreamName:   "primary",
		Model:          "gpt-test",
		UsageModel:     "gpt-test",
		Stream:         true,
		Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
		RequestContext: context.Background(),
	})
	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response = status %d body %q, want status %d body %q", response.Code, response.Body.String(), http.StatusOK, responseBody)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response content type = %q, want application/json", response.Header().Get("Content-Type"))
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v, want prompt=12 completion=8 total=20", item)
	}
}

func TestServeNativeProtocolSniffsJSONWhenStreamResponseHasSSEContentType(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const responseBody = `{"id":"chatcmpl_test","object":"chat.completion","usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	ServeNativeProtocol(response, NativeProxyRequest{
		Client:         WireChat,
		Body:           []byte(`{"model":"gpt-test","stream":true}`),
		UpstreamName:   "primary",
		Model:          "gpt-test",
		UsageModel:     "gpt-test",
		Stream:         true,
		Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
		RequestContext: context.Background(),
	})
	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response = status %d body %q, want status %d body %q", response.Code, response.Body.String(), http.StatusOK, responseBody)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response content type = %q, want application/json", response.Header().Get("Content-Type"))
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v, want prompt=12 completion=8 total=20", item)
	}
}

func TestServeNativeProtocolSniffsSSEWhenStreamResponseHasJSONContentType(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const responseBody = "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":8,\"total_tokens\":20}}\n\n" +
		"data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	ServeNativeProtocol(response, NativeProxyRequest{
		Client:         WireChat,
		Body:           []byte(`{"model":"gpt-test","stream":true}`),
		UpstreamName:   "primary",
		Model:          "gpt-test",
		UsageModel:     "gpt-test",
		Stream:         true,
		Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
		RequestContext: context.Background(),
	})
	if response.Code != http.StatusOK || response.Body.String() != responseBody {
		t.Fatalf("response = status %d body %q, want status %d body %q", response.Code, response.Body.String(), http.StatusOK, responseBody)
	}
	if response.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response content type = %q, want text/event-stream", response.Header().Get("Content-Type"))
	}

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v, want prompt=12 completion=8 total=20", item)
	}
}

func TestServeNativeProtocolStreamsJSONFallbackBeforeBodyCompletes(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	const firstPart = `{"id":"chatcmpl_test","usage":{"prompt_tokens":12,`
	const secondPart = `"completion_tokens":8,"total_tokens":20}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(5 * time.Millisecond)
		_, _ = io.WriteString(w, firstPart)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(10 * time.Millisecond)
		_, _ = io.WriteString(w, secondPart)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	handler := stats.TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		ServeNativeProtocol(w, NativeProxyRequest{
			Client:         WireChat,
			Body:           []byte(`{"model":"same-model","stream":true}`),
			UpstreamName:   "primary",
			Model:          "same-model",
			UsageModel:     "same-model",
			Stream:         true,
			Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
			RequestContext: r.Context(),
		})
	})
	handler(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if response.Code != http.StatusOK || response.Body.String() != firstPart+secondPart {
		t.Fatalf("response = status %d body %q, want status %d body %q", response.Code, response.Body.String(), http.StatusOK, firstPart+secondPart)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response content type = %q, want application/json", response.Header().Get("Content-Type"))
	}
	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v, want prompt=12 completion=8 total=20", item)
	}
	if item.FirstByteMS <= 0 || item.DurationMS <= item.FirstByteMS {
		t.Fatalf("timing values = %#v, want positive duration greater than first byte", item)
	}
}

func TestServeNativeProtocolRecordsDirectSameModelStreamingUsageAndTiming(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(5 * time.Millisecond)
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl_test","choices":[{"delta":{"content":"hello"}}]}`+"\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(10 * time.Millisecond)
		_, _ = io.WriteString(w, `data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	handler := stats.TrackRequest(func(w http.ResponseWriter, r *http.Request) {
		ServeNativeProtocol(w, NativeProxyRequest{
			Client:         WireChat,
			Body:           []byte(`{"model":"same-model","stream":true}`),
			UpstreamName:   "primary",
			Model:          "same-model",
			UsageModel:     "same-model",
			Stream:         true,
			Upstream:       &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI},
			RequestContext: r.Context(),
		})
	})
	handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.RequestModel != "same-model" || item.UpstreamName != "primary" || item.UpstreamModel != "same-model" {
		t.Fatalf("usage identity = %#v", item)
	}
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v", item)
	}
	if item.FirstByteMS <= 0 || item.DurationMS <= item.FirstByteMS {
		t.Fatalf("timing values = %#v, want positive duration greater than first byte", item)
	}
}

func TestChatHandlerAutomaticDirectRouteRecordsSameModelUsageAndTiming(t *testing.T) {
	previousConfig := config.Snapshot()
	t.Cleanup(func() { config.ApplyConfig(previousConfig) })
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "llmrelay.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })
	stats.LoadTokenStats()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(5 * time.Millisecond)
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl_test","choices":[{"delta":{"content":"hello"}}]}`+"\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(10 * time.Millisecond)
		_, _ = io.WriteString(w, `data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	config.ApplyConfig(config.AppConfig{
		Upstreams: map[string]*config.UpstreamConfig{
			"primary": {
				BaseURL:      server.URL,
				APIType:      config.UpstreamOpenAI,
				CustomModels: []string{"same-model"},
			},
		},
		UpstreamOrder: []string{"primary"},
	})

	handler := stats.TrackRequest(ChatCompletionsHandler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"same-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	handler(response, request)

	page, err := stats.ListUsageRecords(stats.UsageQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("usage records = %#v, want one record", page)
	}
	item := page.Items[0]
	if item.RequestModel != "same-model" || item.UpstreamName != "primary" || item.UpstreamModel != "same-model" {
		t.Fatalf("usage identity = %#v", item)
	}
	if item.PromptTokens != 12 || item.CompletionTokens != 8 || item.TotalTokens != 20 {
		t.Fatalf("usage values = %#v", item)
	}
	if item.FirstByteMS <= 0 || item.DurationMS <= item.FirstByteMS {
		t.Fatalf("timing values = %#v, want positive duration greater than first byte", item)
	}
}
