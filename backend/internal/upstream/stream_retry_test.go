package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"llmrelay/backend/internal/sse"
)

func replayTestChunk(delta map[string]any, finish any) string {
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
	return "data: " + string(data) + "\n\n"
}

func replayTestTool(id, args string) string {
	return replayTestChunk(map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "read_file", "arguments": args},
	}}}, nil)
}

func TestChatStreamReplayPreservesDeliveredOutput(t *testing.T) {
	text := func(s string) string { return replayTestChunk(map[string]any{"content": s}, nil) }
	finish := replayTestChunk(map[string]any{}, "stop")
	usage := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":7}}\n\n"
	for _, tc := range []struct {
		name        string
		attempts    []string
		firstError  error
		wantText    string
		wantThought string
		wantArgs    string
		wantError   string
		wantTries   int
		wantUsage   bool
	}{
		{name: "different chunk boundaries", attempts: []string{text("abc"), text("a") + text("bcdef") + finish + usage + "data: [DONE]\n\n"}, wantText: "abcdef", wantTries: 2, wantUsage: true},
		{name: "final event without separator", attempts: []string{strings.TrimRight(text("abc"), "\n"), text("abcdef") + finish}, wantText: "abcdef", wantTries: 2},
		{name: "truncated JSON", attempts: []string{text("abc") + `data: {"choices":[{"delta":{"content":"lost`, text("abcdef") + finish}, wantText: "abcdef", wantTries: 2},
		{name: "reasoning alias", attempts: []string{replayTestChunk(map[string]any{"reasoning": "think"}, nil) + text("abc"), replayTestChunk(map[string]any{"reasoning_content": "th"}, nil) + replayTestChunk(map[string]any{"reasoning_content": "ink"}, nil) + text("abcdef") + finish}, wantText: "abcdef", wantThought: "think", wantTries: 2},
		{name: "tool arguments and changing call ID", attempts: []string{replayTestTool("call_original", `{"path":"a`), replayTestTool("call_replayed", `{"path":"a.go"}`) + replayTestChunk(map[string]any{}, "tool_calls")}, wantArgs: `{"path":"a.go"}`, wantTries: 2},
		{name: "multiple interruptions", attempts: []string{text("ab"), text("abcd"), text("a") + text("bcdef") + finish}, wantText: "abcdef", wantTries: 3},
		{name: "clean premature EOF", attempts: []string{text("abc"), text("abcdef") + finish}, firstError: io.EOF, wantText: "abcdef", wantTries: 2},
		{name: "idle timeout", attempts: []string{text("abc"), text("abcdef") + finish}, firstError: errUpstreamStreamIdleTimeout, wantText: "abcdef", wantTries: 2},
		{name: "HTTP2 transport failure", attempts: []string{text("abc"), text("abcdef") + finish}, firstError: &upstreamStreamReadError{detail: "upstream stream interrupted", cause: errors.New("stream error: INTERNAL_ERROR")}, wantText: "abcdef", wantTries: 2},
		{name: "DONE before finish", attempts: []string{text("abc") + "data: [DONE]\n\n", text("abcdef") + finish}, wantText: "abcdef", wantTries: 2},
		{name: "divergent regeneration", attempts: []string{text("abc"), text("alternate") + finish}, wantText: "abc", wantError: "differs", wantTries: 2},
		{name: "regeneration too short", attempts: []string{text("abc"), text("ab") + finish}, wantText: "abc", wantError: "ended before", wantTries: 2},
		{name: "finish followed by EOF", attempts: []string{text("abc") + finish + usage}, wantText: "abc", wantError: "unexpected EOF", wantTries: 1, wantUsage: true},
		{name: "unsupported semantic field", attempts: []string{text("abc") + replayTestChunk(map[string]any{"annotations": []any{map[string]any{"url": "example"}}}, nil), text("abcdef") + finish}, wantText: "abc", wantError: "unexpected EOF", wantTries: 1},
		{name: "provider error", attempts: []string{text("abc") + "data: {\"error\":{\"message\":\"provider failure\"}}\n\n", text("abcdef") + finish}, wantText: "abc", wantError: "unexpected EOF", wantTries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			attempt := 0
			source := func(index int) io.ReadCloser {
				err := io.ErrUnexpectedEOF
				if index == 0 && tc.firstError != nil {
					err = tc.firstError
				}
				if index == len(tc.attempts)-1 && len(tc.attempts) > 1 {
					err = io.EOF
				}
				return io.NopCloser(iotest.DataErrReader(io.MultiReader(strings.NewReader(tc.attempts[index]), iotest.ErrReader(err))))
			}
			body := &retryStreamBody{ctx: ctx, cancel: cancel, current: source(0), reopen: func(cause error) (io.ReadCloser, error) {
				if attempt+1 >= len(tc.attempts) {
					return nil, cause
				}
				attempt++
				return source(attempt), nil
			}}
			stream := newChatReplayStream(body)
			defer stream.Close()
			raw, err := io.ReadAll(stream)
			if tc.wantError == "" && err != nil {
				t.Fatalf("unexpected read error: %v\n%s", err, raw)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("error=%v, want %q", err, tc.wantError)
			}
			if attempt+1 != tc.wantTries {
				t.Fatalf("attempts=%d, want %d", attempt+1, tc.wantTries)
			}
			reader := sse.NewReader(strings.NewReader(string(raw)))
			var content, thought, arguments, names strings.Builder
			var ids []string
			usageSeen := false
			pingSeen := false
			for {
				event, err := reader.Next()
				if err != nil {
					break
				}
				var chunk map[string]any
				if event.Name == "ping" {
					pingSeen = true
				}
				if json.Unmarshal([]byte(event.Data), &chunk) != nil {
					continue
				}
				if usage, ok := chunk["usage"].(map[string]any); ok && usage["completion_tokens"] == float64(7) {
					usageSeen = true
				}
				choices, _ := chunk["choices"].([]any)
				if len(choices) == 0 {
					continue
				}
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				value, _ := delta["content"].(string)
				content.WriteString(value)
				value, _ = delta["reasoning_content"].(string)
				if value == "" {
					value, _ = delta["reasoning"].(string)
				}
				thought.WriteString(value)
				calls, _ := delta["tool_calls"].([]any)
				for _, raw := range calls {
					call, _ := raw.(map[string]any)
					if id, _ := call["id"].(string); id != "" {
						ids = append(ids, id)
					}
					function, _ := call["function"].(map[string]any)
					value, _ := function["arguments"].(string)
					arguments.WriteString(value)
					value, _ = function["name"].(string)
					names.WriteString(value)
				}
			}
			if content.String() != tc.wantText || thought.String() != tc.wantThought || arguments.String() != tc.wantArgs {
				t.Fatalf("output changed or duplicated: text=%q thought=%q arguments=%q\n%s", content.String(), thought.String(), arguments.String(), raw)
			}
			if tc.wantArgs != "" && (names.String() != "read_file" || len(ids) != 1 || ids[0] != "call_original") {
				t.Fatalf("tool replay changed identity: names=%q IDs=%v", names.String(), ids)
			}
			if tc.wantUsage && !usageSeen {
				t.Fatalf("terminal usage was lost: %s", raw)
			}
			if tc.name == "different chunk boundaries" && !pingSeen {
				t.Fatal("replay did not keep the connection alive while verifying the prefix")
			}
		})
	}
}

func TestChatStreamReplayRemainsLive(t *testing.T) {
	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	stream := newChatReplayStream(&retryStreamBody{ctx: ctx, cancel: cancel, current: reader})
	defer stream.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1024)
		n, err := stream.Read(buffer)
		if err == nil && !strings.Contains(string(buffer[:n]), "live-content") {
			err = errors.New("first event was lost")
		}
		done <- err
	}()
	_, _ = io.WriteString(writer, replayTestChunk(map[string]any{"content": "live-content"}, nil))
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream buffered output until completion")
	}
}

func TestStreamReadRetriesShareBudgetAndRotateKeys(t *testing.T) {
	for _, initialStatusFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "body failure", true: "status and body failure"}[initialStatusFailure], func(t *testing.T) {
			var attempts atomic.Int32
			keys := make(chan string, 3)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := attempts.Add(1)
				keys <- r.Header.Get("Authorization")
				if initialStatusFailure && n == 1 {
					w.WriteHeader(http.StatusBadGateway)
					_, _ = io.WriteString(w, `{"error":{"message":"temporary"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if n == 1 || initialStatusFailure {
					w.Header().Set("Content-Length", "1000")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					return
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			one := 1
			body, _, _, err := CallPreparedUpstreamStream(context.Background(), []byte(`{"stream":true}`), "retry-"+t.Name(), "test", &UpstreamConfig{
				BaseURL: server.URL, APIType: UpstreamOpenAI, APIKey: "first-key\nsecond-key", MaxRetries: &one,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()
			data, err := io.ReadAll(body)
			if initialStatusFailure {
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("error=%v, want exhausted body retry", err)
				}
			} else if err != nil || string(data) != "data: [DONE]\n\n" {
				t.Fatalf("retry response=%q error=%v", data, err)
			}
			if attempts.Load() != 2 {
				t.Fatalf("attempts=%d, want shared budget of two", attempts.Load())
			}
			if first, second := <-keys, <-keys; first != "Bearer first-key" || second != "Bearer second-key" {
				t.Fatalf("key rotation=%q -> %q", first, second)
			}
		})
	}
}

func TestCloseStopsStreamRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	body := &retryStreamBody{
		ctx: ctx, cancel: cancel, current: io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF)),
		reopen: func(error) (io.ReadCloser, error) {
			close(entered)
			return nil, WaitForRetry(ctx, 30*time.Second, "")
		},
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(body); done <- err }()
	<-entered
	_ = body.Close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not stop retry backoff")
	}
}

func TestChatStreamReplayBoundsRetainedPrefix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := newChatReplayStream(&retryStreamBody{ctx: ctx, cancel: cancel, current: io.NopCloser(strings.NewReader(""))})
	defer stream.Close()
	stream.remember(chatReplayPart{kind: "content", value: strings.Repeat("a", maxChatReplayBytes)})
	if !stream.canReplay || stream.prefixBytes != maxChatReplayBytes {
		t.Fatal("prefix limit should be inclusive")
	}
	stream.remember(chatReplayPart{kind: "content", value: "b"})
	if stream.canReplay || stream.prefix != nil {
		t.Fatal("excessive prefix should disable replay and release retained output")
	}
}

type closeObservedStream struct {
	io.Reader
	closed chan struct{}
	once   sync.Once
}

func (r *closeObservedStream) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestCloseDiscardsReplacementStreamOpenedDuringCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	replacement := &closeObservedStream{Reader: strings.NewReader("must not be sent"), closed: make(chan struct{})}
	body := &retryStreamBody{
		ctx: ctx, cancel: cancel, current: io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF)),
		reopen: func(error) (io.ReadCloser, error) {
			close(entered)
			<-release
			return replacement, nil
		},
	}
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(body); done <- err }()
	<-entered
	_ = body.Close()
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("error=%v, want closed stream", err)
		}
	case <-time.After(time.Second):
		t.Fatal("read did not stop after cancellation")
	}
	select {
	case <-replacement.closed:
	default:
		t.Fatal("replacement HTTP body was leaked")
	}
}

func TestRepeatedReadDoesNotRestartExhaustedStream(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	one := 1
	body, _, _, err := CallPreparedUpstreamStream(context.Background(), []byte(`{"stream":true}`), "exhausted", "test", &UpstreamConfig{
		BaseURL: server.URL, APIType: UpstreamOpenAI, MaxRetries: &one,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	_, firstError := io.ReadAll(body)
	if firstError == nil {
		t.Fatal("expected an exhausted retry error")
	}
	for range 3 {
		if _, err := body.Read(make([]byte, 1)); err != firstError {
			t.Fatalf("terminal read error changed: %v -> %v", firstError, err)
		}
	}
	if attempts.Load() != 2 {
		t.Fatalf("retry budget exceeded after repeated Read: %d requests", attempts.Load())
	}
}

func TestChatStreamReplayRecoversAfterMultiMegabyteOutput(t *testing.T) {
	const prefixBytes = 3900122
	prefix := strings.Repeat("0123456789", prefixBytes/10) + "01"
	const suffix = "remaining-output"
	encode := func(text string, chunkSize int) string {
		var frames strings.Builder
		for len(text) > 0 {
			size := min(len(text), chunkSize)
			frames.WriteString(replayTestChunk(map[string]any{"content": text[:size]}, nil))
			text = text[size:]
		}
		return frames.String()
	}
	first := encode(prefix, 8191)
	for _, matches := range []bool{true, false} {
		name := "matching generation"
		if !matches {
			name = "generation differs near the end"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			candidate := prefix
			if !matches {
				candidate = prefix[:len(prefix)-1] + "x"
			}
			second := encode(candidate+suffix, 131071) + replayTestChunk(map[string]any{}, "stop") + "data: [DONE]\n\n"
			attempts := 1
			stream := newChatReplayStream(&retryStreamBody{
				ctx: ctx, cancel: cancel,
				current: io.NopCloser(io.MultiReader(strings.NewReader(first), iotest.ErrReader(io.ErrUnexpectedEOF))),
				reopen: func(cause error) (io.ReadCloser, error) {
					if attempts != 1 {
						return nil, cause
					}
					attempts++
					return io.NopCloser(strings.NewReader(second)), nil
				},
			})
			defer stream.Close()
			reader := sse.NewReader(stream)
			digest := sha256.New()
			delivered, finishes := 0, 0
			var readError error
			for {
				event, err := reader.Next()
				if err != nil {
					readError = err
					break
				}
				var chunk map[string]any
				if json.Unmarshal([]byte(event.Data), &chunk) != nil {
					continue
				}
				if chatChunkFinished(chunk) {
					finishes++
				}
				choices, _ := chunk["choices"].([]any)
				if len(choices) == 0 {
					continue
				}
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				text, _ := delta["content"].(string)
				_, _ = digest.Write([]byte(text))
				delivered += len(text)
			}
			want := prefix
			if matches {
				want += suffix
				if !errors.Is(readError, io.EOF) || finishes != 1 {
					t.Fatalf("matching replay failed: error=%v finishes=%d", readError, finishes)
				}
			} else if readError == nil || !strings.Contains(readError.Error(), "differs") || finishes != 0 {
				t.Fatalf("divergent replay was not rejected: error=%v finishes=%d", readError, finishes)
			}
			wantDigest := sha256.Sum256([]byte(want))
			if delivered != len(want) || string(digest.Sum(nil)) != string(wantDigest[:]) || attempts != 2 {
				t.Fatalf("large response was duplicated or corrupted: delivered=%d want=%d attempts=%d", delivered, len(want), attempts)
			}
			t.Logf("initial_wire_bytes=%d semantic_prefix_bytes=%d delivered_bytes=%d attempts=%d", len(first), prefixBytes, delivered, attempts)
		})
	}
}
