package stream

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
