package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llmrelay/backend/internal/sse"
)

func TestSSEFramingAcrossProtocolMatrix(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			for _, ending := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}, {"CR", "\r"}} {
				t.Run(fmt.Sprintf("%s/%s/%s", surface.name, apiType, ending.name), func(t *testing.T) {
					parser := sse.NewParser(sse.DefaultMaxEventBytes)
					events := append(parser.Feed([]byte(matrixMockStream(apiType))), parser.Flush()...)
					var framed strings.Builder
					for _, event := range events {
						if event.Name != "" {
							framed.WriteString("event: " + event.Name + ending.value)
						}
						framed.WriteString(": heartbeat" + ending.value)
						data := event.Data
						var pretty bytes.Buffer
						if json.Indent(&pretty, []byte(data), "", "  ") == nil {
							data = pretty.String()
						}
						for _, line := range strings.Split(data, "\n") {
							// data: 后的单个空格在 SSE 中可省略。
							framed.WriteString("data:" + line + ending.value)
						}
						framed.WriteString(ending.value)
					}
					payload := strings.TrimRight(framed.String(), "\r\n") // EOF 收尾兼容。
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, payload)
					}))
					defer upstream.Close()
					matrixSelectUpstream(upstream.URL, apiType)
					body, _ := matrixDoRequest(t, gateway, surface, true)
					if !strings.Contains(string(body), matrixText) {
						t.Fatalf("text lost: %s", body)
					}
					clientParser := sse.NewParser(sse.DefaultMaxEventBytes)
					clientEvents := append(clientParser.Feed(body), clientParser.Flush()...)
					terminal := false
					for _, event := range clientEvents {
						var object map[string]any
						if event.Data == "[DONE]" {
							terminal = true
						} else if json.Unmarshal([]byte(event.Data), &object) == nil {
							if object["error"] != nil || object["type"] == "error" || object["type"] == "response.failed" || object["type"] == "response.incomplete" {
								t.Fatalf("unexpected failure: %s", body)
							}
							if event.Name == "response.completed" || event.Name == "message_stop" {
								terminal = true
							}
						}
					}
					if !terminal {
						t.Fatalf("terminal event missing: %s", body)
					}
				})
			}
		}
	}
}
