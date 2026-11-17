package admin

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"llmrelay/backend/internal/logs"
)

func TestAdminLogsHandlerReplaysBufferedLines(t *testing.T) {
	var console strings.Builder
	logs.Install(&console)
	t.Cleanup(func() { logs.Install(nil) })
	log.SetFlags(log.LstdFlags)
	log.Printf("[测试] 上游拉取失败")

	recorder := httptest.NewRecorder()
	AdminLogsHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/logs?limit=5", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Items []logs.Entry `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatalf("expected buffered entries, got none")
	}
	last := payload.Items[len(payload.Items)-1]
	if !strings.Contains(last.Message, "上游拉取失败") {
		t.Fatalf("last message = %q", last.Message)
	}
	if last.Level != "error" {
		t.Fatalf("level = %q, want error", last.Level)
	}
	if !strings.Contains(console.String(), "上游拉取失败") {
		t.Fatalf("console output was not preserved: %q", console.String())
	}
}

func TestAdminLogsHandlerStreamsNewLines(t *testing.T) {
	logs.Install(nil)
	t.Cleanup(func() { logs.Install(nil) })
	server := httptest.NewServer(middlewareForLogs(AdminLogsHandler))
	defer server.Close()

	// Skip the replay of older buffered lines so the assertion only sees the
	// line published after the stream connected.
	last := logs.Recent(1, 0)
	after := int64(0)
	if len(last) > 0 {
		after = last[0].Seq
	}
	response, err := http.Get(server.URL + "/api/logs?stream=1&after=" + strconv.FormatInt(after, 10))
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", response.StatusCode)
	}

	log.Printf("[测试] 流式日志")
	buffer := make([]byte, 4096)
	read, err := response.Body.Read(buffer)
	if err != nil && read == 0 {
		t.Fatalf("read stream: %v", err)
	}
	if !strings.Contains(string(buffer[:read]), "流式日志") {
		t.Fatalf("stream payload = %q", string(buffer[:read]))
	}
}

func middlewareForLogs(next http.HandlerFunc) http.Handler { return next }
