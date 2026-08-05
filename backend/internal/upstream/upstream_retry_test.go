package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"llmrelay/backend/internal/netproxy"
)

func TestRateLimitRetriesAreCappedPerExit(t *testing.T) {
	proxies, active := netproxy.Snapshot()
	netproxy.Configure(nil, "")
	t.Cleanup(func() { netproxy.Configure(proxies, active) })

	for _, stream := range []bool{false, true} {
		name := "non-stream"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempt := attempts.Add(1)
				w.Header().Set("X-Upstream-Attempt", fmt.Sprint(attempt))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			}))
			defer server.Close()

			maxRetries := 10
			target := &UpstreamConfig{
				BaseURL:    server.URL,
				APIKey:     "key-1\nkey-2",
				APIType:    UpstreamOpenAI,
				MaxRetries: &maxRetries,
			}

			var (
				body   []byte
				status int
				header http.Header
				err    error
			)
			if stream {
				var reader io.ReadCloser
				reader, status, header, err = CallPreparedUpstreamStream(context.Background(), []byte(`{"model":"test"}`), name, "test", target, true)
				if reader != nil {
					body, _ = io.ReadAll(reader)
					_ = reader.Close()
				}
			} else {
				body, status, header, err = CallPreparedUpstream(context.Background(), []byte(`{"model":"test"}`), name, "test", target, true)
			}

			if err == nil {
				t.Fatal("expected rate limit error")
			}
			if status != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want %d", status, http.StatusTooManyRequests)
			}
			if got := attempts.Load(); got != 4 {
				t.Fatalf("attempts = %d, want 4 (initial request plus three retries)", got)
			}
			if got := header.Get("X-Upstream-Attempt"); got != "4" {
				t.Fatalf("terminal response header = %q, want 4", got)
			}
			if !strings.Contains(string(body), "rate limited") {
				t.Fatalf("terminal response body = %q", body)
			}
		})
	}
}
