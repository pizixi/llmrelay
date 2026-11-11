package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"llmrelay/backend/internal/sse"
)

func TestNativeStreamsKeepTerminalAtUnexpectedEOF(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol UpstreamType
		terminal string
	}{
		{"chat", UpstreamOpenAI, "data: [DONE]"},
		{"anthropic", UpstreamAnthropic, `data: {"type":"message_stop"}`},
		{"responses", UpstreamResponses, `data: {"type":"response.completed"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, complete := range []bool{false, true} {
				raw := tc.terminal
				if !complete {
					raw = strings.TrimSuffix(raw, "}")
					if tc.protocol == UpstreamOpenAI {
						raw = "data: [DON"
					}
				}
				body := io.NopCloser(io.MultiReader(strings.NewReader(raw), iotest.ErrReader(io.ErrUnexpectedEOF)))
				recorder := httptest.NewRecorder()
				err := proxyNativePassthroughStream(recorder, body, tc.protocol, nil)
				if complete && err != nil {
					t.Fatalf("complete terminal event returned %v", err)
				}
				if !complete && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("truncated terminal event returned %v, want unexpected EOF", err)
				}
				if recorder.Body.String() != raw {
					t.Fatalf("passthrough changed bytes: %q, want %q", recorder.Body.String(), raw)
				}
			}
		})
	}
}

func TestUpstreamStreamReadErrorIncludesRoutingContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = io.WriteString(w, "data: partial\n\n")
	}))
	defer server.Close()
	zero := 0
	body, _, _, err := CallPreparedUpstreamStream(context.Background(), []byte(`{"stream":true}`), "broken-upstream", "broken-model", &UpstreamConfig{
		BaseURL: server.URL, APIType: UpstreamOpenAI, APIKey: "secret-key", MaxRetries: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error=%v, want unexpected EOF", err)
	}
	for _, want := range []string{"broken-upstream", "broken-model", "direct", fmt.Sprintf("received=%d", len(data))} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error=%q, missing context %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("read error exposed the upstream API key: %v", err)
	}
}

func TestUpstreamStreamIdleTimeoutCancelsStalledReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		prefix string
	}{
		{name: "before first byte", status: http.StatusOK},
		{name: "after partial output", status: http.StatusOK, prefix: "data: partial\n\n"},
		{name: "unfinished error body", status: http.StatusBadGateway, prefix: `{"error":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canceled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.prefix)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(canceled)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			zero := 0
			body, status, _, err := callPreparedUpstreamStream(ctx, []byte(`{"stream":true}`), "idle-test", "test", &UpstreamConfig{
				BaseURL: server.URL, APIType: UpstreamOpenAI, MaxRetries: &zero,
			}, 80*time.Millisecond, true)
			var received []byte
			if body != nil {
				defer body.Close()
				if err == nil {
					received, err = io.ReadAll(body)
				}
			}
			if !errors.Is(err, errUpstreamStreamIdleTimeout) {
				t.Fatalf("error = %v, want stream idle timeout", err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("idle timeout was misclassified as client cancellation: %v", err)
			}
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			if tc.status == http.StatusOK && string(received) != tc.prefix {
				t.Fatalf("partial output = %q, want %q", received, tc.prefix)
			}
			if ctx.Err() != nil {
				t.Fatalf("upstream idle timeout canceled the parent context: %v", ctx.Err())
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("idle timeout did not cancel the upstream HTTP request")
			}
		})
	}
}

func TestUpstreamStreamIdleTimeoutAllowsLongActiveStream(t *testing.T) {
	const chunks = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for range chunks {
			_, _ = io.WriteString(w, "data: active\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-time.After(40 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	body, _, _, err := callPreparedUpstreamStream(ctx, []byte(`{"stream":true}`), "active-test", "test", &UpstreamConfig{
		BaseURL: server.URL, APIType: UpstreamOpenAI,
	}, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	received, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("active stream failed: %v", err)
	}
	if want := strings.Repeat("data: active\n\n", chunks); string(received) != want {
		t.Fatalf("stream = %q, want %q", received, want)
	}
}

func TestUpstreamStreamClientCancellationRemainsDistinct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body, _, _, err := callPreparedUpstreamStream(ctx, []byte(`{"stream":true}`), "cancel-test", "test", &UpstreamConfig{
		BaseURL: server.URL, APIType: UpstreamOpenAI,
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	cancel()
	_, err = io.ReadAll(body)
	if !errors.Is(err, context.Canceled) || errors.Is(err, errUpstreamStreamIdleTimeout) {
		t.Fatalf("error = %v, want client cancellation", err)
	}
}

func TestNativeStreamsFinishWithoutWaitingForConnectionClose(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol UpstreamType
		terminal string
	}{
		{"chat", UpstreamOpenAI, "data: [DONE]\n\n"},
		{"anthropic", UpstreamAnthropic, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
		{"responses completed", UpstreamResponses, "data: {\"type\":\"response.completed\"}\n\n"},
		{"responses incomplete", UpstreamResponses, "data: {\"type\":\"response.incomplete\"}\n\n"},
		{"responses failed", UpstreamResponses, "event: response.failed\ndata: {\"error\":{\"message\":\"failed\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := ": keepalive\r\n\r\nevent: vendor.notice\ndata: {\"future\":true}\n\n" + tc.terminal
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, raw)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			body, _, _, err := callPreparedUpstreamStream(ctx, []byte(`{"stream":true}`), "terminal-test", "test", &UpstreamConfig{
				BaseURL: server.URL, APIType: tc.protocol,
			}, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			observedTerminal := false
			if err := proxyNativePassthroughStream(recorder, body, tc.protocol, func(event sse.Event) {
				observedTerminal = isNativeStreamTerminal(event, tc.protocol) || observedTerminal
			}); err != nil {
				t.Fatalf("proxy waited past terminal event: %v", err)
			}
			if recorder.Body.String() != raw || !observedTerminal {
				t.Fatalf("stream = %q, terminal observed = %t", recorder.Body.String(), observedTerminal)
			}
		})
	}
}

func TestChatPassthroughPreservesDelayedUsageAfterFinishReason(t *testing.T) {
	finish := "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	tail := "data: {\"choices\":[],\"usage\":{\"completion_tokens\":5}}\n\ndata: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, finish)
		w.(http.Flusher).Flush()
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, tail)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	body, _, _, err := callPreparedUpstreamStream(ctx, []byte(`{"stream":true}`), "usage-test", "test", &UpstreamConfig{
		BaseURL: server.URL, APIType: UpstreamOpenAI,
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	if err := ProxyChatPassthroughStream(recorder, body, "test", false); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != finish+tail {
		t.Fatalf("lost trailing usage: %q", recorder.Body.String())
	}
}
