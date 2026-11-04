package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llmrelay/backend/internal/sse"
)

// 同一份上下文：7 个普通输入、100 个缓存命中、10 个缓存写入、5 个输出。
// 同时检查客户端看到的用量与网关统计，防止某一路径再次漏算或重复计算缓存。
func TestCacheUsageAcrossProtocolMatrix(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", surface.name, apiType, streaming), func(t *testing.T) {
					usageResetStats(t)
					anthropicUsage := `{"input_tokens":7,"cache_read_input_tokens":100,"cache_creation_input_tokens":10,"output_tokens":5}`
					chatUsage := `{"prompt_tokens":117,"completion_tokens":5,"total_tokens":122,"prompt_tokens_details":{"cached_tokens":100,"cache_write_tokens":10}}`
					responsesUsage := `{"input_tokens":117,"output_tokens":5,"total_tokens":122,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":10}}`
					payload := usageMatrixMockJSON(apiType)
					if streaming {
						payload = usageMatrixMockStream(apiType)
						if apiType == UpstreamOpenAI {
							// Chat 的最终用量分块位于 finish_reason 之后。
							finish := `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n"
							payload = strings.Replace(payload, `data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[],`, finish+`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[],`, 1)
						}
					}
					switch apiType {
					case UpstreamAnthropic:
						payload = strings.ReplaceAll(payload, `{"input_tokens":7,"output_tokens":5}`, anthropicUsage)
						// 起始与终态用量不同，验证累积快照的合并而非加总。
						payload = strings.ReplaceAll(payload, `{"input_tokens":7,"output_tokens":0}`, `{"input_tokens":1,"cache_read_input_tokens":50,"cache_creation_input_tokens":2,"output_tokens":0}`)
						payload = strings.ReplaceAll(payload, `{"output_tokens":5}`, anthropicUsage)
					case UpstreamResponses:
						payload = strings.ReplaceAll(payload, `{"input_tokens":7,"output_tokens":5,"total_tokens":12}`, responsesUsage)
					default:
						payload = strings.ReplaceAll(payload, `{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}`, chatUsage)
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						if streaming {
							w.Header().Set("Content-Type", "text/event-stream")
						} else {
							w.Header().Set("Content-Type", "application/json")
						}
						_, _ = io.WriteString(w, payload)
					}))
					defer upstream.Close()
					matrixSelectUpstream(upstream.URL, apiType)
					body, _ := matrixDoRequest(t, gateway, surface, streaming)
					usage := map[string]any{}
					observe := func(object map[string]any) {
						if response, ok := object["response"].(map[string]any); ok {
							object = response
						}
						if message, ok := object["message"].(map[string]any); ok {
							object = message
						}
						if values, ok := object["usage"].(map[string]any); ok {
							for key, value := range values {
								usage[key] = value
							}
						}
					}
					if streaming {
						parser := sse.NewParser(sse.DefaultMaxEventBytes)
						events := append(parser.Feed(body), parser.Flush()...)
						for _, event := range events {
							var object map[string]any
							if json.Unmarshal([]byte(event.Data), &object) == nil {
								if object["error"] != nil || object["type"] == "error" {
									t.Fatalf("stream error: %s", body)
								}
								observe(object)
							}
						}
					} else {
						observe(decodePairwiseTestObject(t, body))
					}
					switch surface.path {
					case "/v1/messages":
						requireTestEqual(t, "uncached input", usage["input_tokens"], float64(7))
						requireTestEqual(t, "cache read", usage["cache_read_input_tokens"], float64(100))
						requireTestEqual(t, "cache write", usage["cache_creation_input_tokens"], float64(10))
						requireTestEqual(t, "output", usage["output_tokens"], float64(5))
					default:
						inputKey, outputKey, detailsKey := "input_tokens", "output_tokens", "input_tokens_details"
						if surface.path == "/v1/chat/completions" {
							inputKey, outputKey, detailsKey = "prompt_tokens", "completion_tokens", "prompt_tokens_details"
						}
						requireTestEqual(t, "total input", usage[inputKey], float64(117))
						requireTestEqual(t, "output", usage[outputKey], float64(5))
						requireTestEqual(t, "total", usage["total_tokens"], float64(122))
						details := testObject(t, usage[detailsKey], detailsKey)
						requireTestEqual(t, "cache read", details["cached_tokens"], float64(100))
						requireTestEqual(t, "cache write", details["cache_write_tokens"], float64(10))
					}
					snapshot := usageSnapshot()
					model := snapshot.Models[matrixPublicModel]
					if snapshot.TotalRequests != 1 || model == nil || model.PromptTokens != 117 || model.CachedTokens != 100 || model.CompletionTokens != 5 || model.TotalTokens != 122 {
						t.Fatalf("incorrect statistics: requests=%d model=%+v", snapshot.TotalRequests, model)
					}
				})
			}
		}
	}
}
