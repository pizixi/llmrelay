package stream

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"llmrelay/backend/internal/stats"
)

func TestChatBridgesHandleUnexpectedEOF(t *testing.T) {
	previousPath := stats.Path()
	stats.SetPath(filepath.Join(t.TempDir(), "usage.db"))
	t.Cleanup(func() { stats.SetPath(previousPath) })

	content := `data: {"choices":[{"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n"
	finish := `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`
	usage := `data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`
	for _, target := range []struct {
		name     string
		run      func(http.ResponseWriter, io.ReadCloser)
		terminal string
		failure  string
	}{
		{
			name: "anthropic",
			run: func(w http.ResponseWriter, body io.ReadCloser) {
				ClaudeStreamHandler(w, body, "test", "test")
			},
			terminal: "event: message_stop\n", failure: "event: error\n",
		},
		{
			name: "responses",
			run: func(w http.ResponseWriter, body io.ReadCloser) {
				ResponsesStreamHandler(w, nil, &http.Response{Body: body}, "test", "test", nil, nil, nil, nil, nil)
			},
			terminal: "event: response.completed\n", failure: "event: response.failed\n",
		},
	} {
		for _, tc := range []struct {
			name    string
			raw     string
			err     error
			success bool
			usage   bool
		}{
			{name: "finished with trailing usage", raw: content + finish + "\n\n" + usage + "\n\n", success: true, usage: true},
			{name: "unterminated finish event", raw: content + finish, success: true},
			{name: "unterminated usage event", raw: content + finish + "\n\n" + usage, success: true, usage: true},
			{name: "premature EOF", raw: strings.TrimRight(content, "\n")},
			{name: "truncated finish JSON", raw: content + `data: {"choices":[{"finish_reason":"st`},
			{name: "timeout after finish", raw: content + finish + "\n\n", err: errors.New("upstream stream idle timeout")},
			{name: "error event after finish", raw: content + finish + "\n\ndata: {\"error\":{\"message\":\"upstream failed\"}}\n\n"},
		} {
			t.Run(target.name+"/"+tc.name, func(t *testing.T) {
				readErr := tc.err
				if readErr == nil {
					readErr = fmt.Errorf("transport: %w", io.ErrUnexpectedEOF)
				}
				body := io.NopCloser(iotest.DataErrReader(io.MultiReader(strings.NewReader(tc.raw), iotest.ErrReader(readErr))))
				defer body.Close()
				recorder := httptest.NewRecorder()
				target.run(recorder, body)
				got := recorder.Body.String()
				if !strings.Contains(got, "partial") {
					t.Fatalf("partial content was lost: %s", got)
				}
				if tc.success {
					if strings.Count(got, target.terminal) != 1 || strings.Contains(got, target.failure) {
						t.Fatalf("completed response was reported as failed: %s", got)
					}
					if tc.usage && !strings.Contains(got, `"output_tokens":7`) {
						t.Fatalf("trailing usage was lost: %s", got)
					}
				} else if !strings.Contains(got, target.failure) || strings.Contains(got, target.terminal) {
					t.Fatalf("interrupted response was reported as completed: %s", got)
				}
			})
		}
	}
}

func TestClaudeStreamReportsReadTimeoutWithoutSuccessfulTerminal(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	defer reader.Close()
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		ClaudeStreamHandler(recorder, reader, "test", "test")
		close(done)
	}()
	if _, err := io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n"); err != nil {
		t.Fatal(err)
	}
	_ = writer.CloseWithError(errors.New("upstream stream idle timeout: no data received for 2m0s"))
	select {
	case <-done:
	case <-time.After(time.Second):
		_ = reader.Close()
		<-done
		t.Fatal("converter did not return after upstream read timeout")
	}
	got := recorder.Body.String()
	if !strings.Contains(got, `"text":"partial"`) || !strings.Contains(got, "event: error\n") || !strings.Contains(got, "upstream stream idle timeout") {
		t.Fatalf("timeout was not reported with partial content: %s", got)
	}
	if strings.Contains(got, "event: message_stop\n") {
		t.Fatalf("timeout was incorrectly reported as successful completion: %s", got)
	}
}

func TestConvertedChatStreamsFinishWithoutUpstreamEOF(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		run  func(http.ResponseWriter, io.ReadCloser)
	}{
		{
			name: "anthropic",
			raw: "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			run: func(w http.ResponseWriter, body io.ReadCloser) {
				AnthropicStreamToChatHandler(w, body, "test", "test", false)
			},
		},
		{
			name: "responses",
			raw:  "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n",
			run: func(w http.ResponseWriter, body io.ReadCloser) {
				ResponsesStreamToChatHandler(w, body, "test", "test", false)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			defer reader.Close()
			recorder := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				tc.run(recorder, reader)
				close(done)
			}()
			if _, err := io.WriteString(writer, tc.raw); err != nil {
				t.Fatal(err)
			}
			// 写端保持打开，模拟上游已经发送终态但不关闭 HTTP 响应。
			select {
			case <-done:
			case <-time.After(time.Second):
				_ = reader.Close()
				<-done
				t.Fatal("converter waited for EOF after upstream terminal event")
			}
			if got := recorder.Body.String(); !strings.Contains(got, "data: [DONE]\n\n") || strings.Contains(got, `"error"`) {
				t.Fatalf("invalid converted terminal stream: %s", got)
			}
		})
	}
}
