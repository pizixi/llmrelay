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
)

func TestCredentialFailureTriesRemainingKeysWithinRetryBudget(t *testing.T) {
	for _, status := range []int{401, 402, 403} {
		for _, stream := range []bool{false, true} {
			for _, retries := range []int{0, 1} {
				t.Run(fmt.Sprintf("status=%d/stream=%t/retries=%d", status, stream, retries), func(t *testing.T) {
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Header.Get("Authorization") != "Bearer good-key" {
							w.WriteHeader(status)
							_, _ = io.WriteString(w, `{"error":{"message":"credential rejected"}}`)
							return
						}
						_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
					}))
					defer server.Close()
					target := &UpstreamConfig{BaseURL: server.URL, APIType: UpstreamOpenAI, APIKey: "bad-key\ngood-key", MaxRetries: &retries}
					var body []byte
					var result int
					var err error
					if stream {
						var reader io.ReadCloser
						reader, result, _, err = CallPreparedUpstreamStream(context.Background(), []byte(`{"model":"test"}`), t.Name(), "test", target, true)
						if reader != nil {
							body, _ = io.ReadAll(reader)
							_ = reader.Close()
						}
					} else {
						body, result, _, err = CallPreparedUpstream(context.Background(), []byte(`{"model":"test"}`), t.Name(), "test", target, true)
					}
					if retries == 0 {
						if result != status || err == nil || calls.Load() != 1 {
							t.Fatalf("disabled retries status=%d err=%v calls=%d", result, err, calls.Load())
						}
					} else if result != 200 || err != nil || calls.Load() != 2 || !strings.Contains(string(body), "ok") {
						t.Fatalf("credential retry status=%d err=%v calls=%d body=%s", result, err, calls.Load(), body)
					}
				})
			}
		}
	}
}
