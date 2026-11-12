package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"llmrelay/backend/internal/bridge/convert"
	"llmrelay/backend/internal/sse"
)

type chatStreamReplayContextKey struct{}

// WithChatStreamReplay enables verified replay for Chat protocol converters.
// Native passthrough does not opt in: its original framing must stay intact.
func WithChatStreamReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, chatStreamReplayContextKey{}, true)
}

func chatStreamReplayEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(chatStreamReplayContextKey{}).(bool)
	return enabled
}

// retryStreamBody shares the request's connection/status retry budget with body
// retries. Close also cancels backoff and any replacement HTTP request.
type retryStreamBody struct {
	ctx         context.Context
	cancel      context.CancelFunc
	reopen      func(error) (io.ReadCloser, error)
	mu          sync.Mutex
	current     io.ReadCloser
	closed      bool
	delivered   bool
	terminalErr error
	closeOnce   sync.Once
	closeErr    error
	requestID   string
}

func (r *retryStreamBody) readAttempt(p []byte) (int, error) {
	r.mu.Lock()
	body, closed := r.current, r.closed
	r.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	return body.Read(p)
}

func (r *retryStreamBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.terminalErr != nil {
		return 0, r.terminalErr
	}
	for {
		n, err := r.readAttempt(p)
		if n > 0 {
			r.delivered = true
		}
		if err == nil || r.delivered || !retryableStreamReadError(err) {
			r.terminalErr = err
			return n, err
		}
		if err := r.retry(err); err != nil {
			r.terminalErr = err
			return 0, err
		}
	}
}

func (r *retryStreamBody) retry(cause error) error {
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	r.mu.Lock()
	body, closed := r.current, r.closed
	r.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}
	_ = body.Close()
	next, err := r.reopen(cause)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = next.Close()
		return io.ErrClosedPipe
	}
	r.current = next
	r.mu.Unlock()
	return nil
}

func (r *retryStreamBody) Close() error {
	r.closeOnce.Do(func() {
		r.cancel()
		r.mu.Lock()
		r.closed = true
		body := r.current
		r.mu.Unlock()
		r.closeErr = body.Close()
	})
	return r.closeErr
}

func retryableStreamReadError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var networkError net.Error
	var transportError *upstreamStreamReadError
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, errUpstreamStreamIdleTimeout) || errors.As(err, &transportError) || errors.As(err, &networkError) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

// Only semantic deltas are compared. Request IDs, tool IDs and SSE/network chunk
// boundaries may change on a new generation. Already delivered text, reasoning,
// tool names and argument bytes must match exactly before any suffix is sent.
type chatReplayPart struct {
	kind  string
	index int
	value string
	id    string
}

type chatReplayPrefixPart struct {
	kind  string
	index int
	value []byte
}

const maxChatReplayBytes = 8 << 20
const maxChatReplayParts = 65536

type chatReplayStream struct {
	body        *retryStreamBody
	reader      *sse.Reader
	pending     []byte
	err         error
	finished    bool
	canReplay   bool
	prefix      []chatReplayPrefixPart
	prefixBytes int
	tools       map[int]bool
	replaying   bool
	matchPart   int
	matchByte   int
	matchLimit  int
	lastPing    time.Time
}

type streamAttemptReader struct{ body *retryStreamBody }

func (r streamAttemptReader) Read(p []byte) (int, error) { return r.body.readAttempt(p) }

func newChatReplayStream(body *retryStreamBody) *chatReplayStream {
	return &chatReplayStream{
		body: body, reader: sse.NewReader(streamAttemptReader{body}),
		canReplay: true, tools: make(map[int]bool),
	}
}

func (r *chatReplayStream) Close() error { return r.body.Close() }

func (r *chatReplayStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 && r.err == nil {
		event, err := r.reader.Next()
		if err != nil {
			if !r.finished && r.canReplay && retryableStreamReadError(err) {
				err = r.restart(err)
				if err == nil {
					continue
				}
			}
			r.err = err
			break
		}
		if strings.TrimSpace(event.Data) == "[DONE]" && !r.finished {
			if r.canReplay {
				if err := r.restart(fmt.Errorf("upstream stream ended before finish_reason: %w", io.ErrUnexpectedEOF)); err == nil {
					continue
				} else {
					r.err = err
					break
				}
			}
		}
		if event.Data == "" || r.finished {
			r.pending = encodeStreamEvent(event)
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(event.Data), &chunk) != nil {
			// A truncated last JSON event has no semantic output to replay.
			// Keep its frame boundary intact for the converter, then handle the
			// original transport error on the next iteration.
			if !r.replaying {
				r.pending = encodeStreamEvent(event)
			}
			continue
		}
		if chunk["error"] != nil || chunk["type"] == "error" || chunk["type"] == "response.failed" {
			r.canReplay = false
			r.pending = encodeStreamEvent(event)
			continue
		}
		var parts []chatReplayPart
		supported := true
		if r.canReplay {
			parts, supported = r.parts(chunk)
		}
		if !supported {
			r.canReplay = false
			r.prefix = nil
			if r.replaying {
				r.err = errors.New("cannot resume upstream stream: replay contains unsupported deltas")
				break
			}
		}
		finish := chatChunkFinished(chunk)
		if r.replaying {
			newDelta, changed, err := r.match(parts)
			if err != nil || (finish && r.replaying) {
				if err == nil {
					err = errors.New("cannot resume upstream stream: regenerated response ended before the delivered prefix")
				}
				r.err = err
				break
			}
			if !changed && !finish {
				// Usage preceding the replayed prefix is superseded by the new
				// attempt's terminal usage; it must not create duplicate output.
				if time.Since(r.lastPing) >= 10*time.Second {
					r.lastPing = time.Now()
					r.pending = encodeStreamEvent(sse.Event{Name: "ping", Data: `{"type":"ping"}`})
				}
				continue
			}
			choices, _ := chunk["choices"].([]any)
			if len(choices) > 0 {
				choice, _ := choices[0].(map[string]any)
				choice["delta"] = newDelta
			}
			data, _ := json.Marshal(chunk)
			event.Data = string(data)
		} else if r.canReplay {
			for _, part := range parts {
				r.remember(part)
			}
		}
		if finish {
			r.finished = true
			r.prefix = nil
		}
		r.pending = encodeStreamEvent(event)
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *chatReplayStream) restart(cause error) error {
	if err := r.body.retry(cause); err != nil {
		return err
	}
	r.reader = sse.NewReader(streamAttemptReader{r.body})
	r.tools = make(map[int]bool)
	r.matchPart, r.matchByte, r.matchLimit = 0, 0, len(r.prefix)
	r.replaying = r.matchLimit > 0
	r.lastPing = time.Time{}
	return nil
}

func (r *chatReplayStream) remember(part chatReplayPart) {
	if !r.canReplay {
		return
	}
	r.prefixBytes += len(part.value)
	if r.prefixBytes > maxChatReplayBytes || len(r.prefix) >= maxChatReplayParts {
		r.canReplay = false
		r.prefix = nil
		return
	}
	n := len(r.prefix)
	if n > 0 && r.prefix[n-1].kind == part.kind && r.prefix[n-1].index == part.index {
		r.prefix[n-1].value = append(r.prefix[n-1].value, part.value...)
		return
	}
	r.prefix = append(r.prefix, chatReplayPrefixPart{kind: part.kind, index: part.index, value: []byte(part.value)})
}

func (r *chatReplayStream) match(parts []chatReplayPart) (map[string]any, bool, error) {
	delta := map[string]any{}
	tools := map[int]map[string]any{}
	changed := false
	for _, part := range parts {
		value := part.value
		for len(value) > 0 && r.replaying {
			expected := r.prefix[r.matchPart]
			if part.kind != expected.kind || part.index != expected.index {
				return nil, false, fmt.Errorf("cannot resume upstream stream: regenerated %s differs from delivered %s", part.kind, expected.kind)
			}
			count := min(len(value), len(expected.value)-r.matchByte)
			if value[:count] != string(expected.value[r.matchByte:r.matchByte+count]) {
				return nil, false, fmt.Errorf("cannot resume upstream stream: regenerated %s differs from already delivered output", part.kind)
			}
			value = value[count:]
			r.matchByte += count
			if r.matchByte == len(expected.value) {
				r.matchPart++
				r.matchByte = 0
				if r.matchPart == r.matchLimit {
					r.replaying = false
					log.Printf("[上游流恢复] 请求=%s 已校验重复输出=%d bytes，继续转发剩余内容", r.body.requestID, r.prefixBytes)
				}
			}
		}
		if value == "" {
			continue
		}
		part.value = value
		r.remember(part)
		changed = true
		if !strings.HasPrefix(part.kind, "tool_") {
			delta[part.kind] = value
			continue
		}
		tool := tools[part.index]
		if tool == nil {
			tool = map[string]any{"index": part.index, "function": map[string]any{}}
			tools[part.index] = tool
			calls, _ := delta["tool_calls"].([]any)
			delta["tool_calls"] = append(calls, tool)
		}
		function := tool["function"].(map[string]any)
		switch part.kind {
		case "tool_start":
			tool["id"], tool["type"] = part.id, "function"
		case "tool_name":
			previous, _ := function["name"].(string)
			function["name"] = previous + value
		case "tool_arguments":
			previous, _ := function["arguments"].(string)
			function["arguments"] = previous + value
		}
	}
	return delta, changed, nil
}

func (r *chatReplayStream) parts(chunk map[string]any) ([]chatReplayPart, bool) {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil, true
	}
	if len(choices) != 1 {
		return nil, false
	}
	choice, _ := choices[0].(map[string]any)
	if choice["logprobs"] != nil {
		return nil, false
	}
	delta, _ := choice["delta"].(map[string]any)
	for field, value := range delta {
		if value == nil {
			continue
		}
		switch field {
		case "role", "content", "reasoning", "reasoning_content", "reasoning_signature", "refusal", "tool_calls":
		default:
			return nil, false
		}
	}
	convert.NormalizeReasoningContent(delta)
	var parts []chatReplayPart
	for _, field := range []string{"reasoning_content", "reasoning_signature", "content", "refusal"} {
		if delta[field] == nil {
			continue
		}
		value, ok := delta[field].(string)
		if !ok {
			return nil, false
		}
		if value != "" {
			parts = append(parts, chatReplayPart{kind: field, value: value})
		}
	}
	if delta["tool_calls"] == nil {
		return parts, true
	}
	calls, ok := delta["tool_calls"].([]any)
	if !ok {
		return nil, false
	}
	for _, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		indexNumber, ok := call["index"].(float64)
		index := int(indexNumber)
		if !ok || indexNumber != float64(index) || index < 0 {
			return nil, false
		}
		if !r.tools[index] {
			r.tools[index] = true
			id, _ := call["id"].(string)
			parts = append(parts, chatReplayPart{kind: "tool_start", index: index, value: "\x00", id: id})
		}
		function, _ := call["function"].(map[string]any)
		for _, field := range []string{"name", "arguments"} {
			if function[field] == nil {
				continue
			}
			value, ok := function[field].(string)
			if !ok {
				return nil, false
			}
			if value != "" {
				parts = append(parts, chatReplayPart{kind: "tool_" + field, index: index, value: value})
			}
		}
	}
	return parts, true
}

func chatChunkFinished(chunk map[string]any) bool {
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return false
	}
	choice, _ := choices[0].(map[string]any)
	finish, _ := choice["finish_reason"].(string)
	switch finish {
	case "stop", "length", "tool_calls", "function_call", "content_filter":
		return true
	}
	return false
}

func encodeStreamEvent(event sse.Event) []byte {
	var result strings.Builder
	if event.Name != "" {
		result.WriteString("event: " + event.Name + "\n")
	}
	for _, line := range strings.Split(event.Data, "\n") {
		result.WriteString("data: " + line + "\n")
	}
	result.WriteByte('\n')
	return []byte(result.String())
}
