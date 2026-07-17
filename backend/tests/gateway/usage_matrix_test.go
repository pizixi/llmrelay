package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// usageMatrixMockStream 与 matrixMockStream 类似，但输出非零 Token 用量，
// 使 recordTokenUsage 的保护条件（tt > 0）真正触发。默认矩阵模拟返回全零用量，
// 因而现有测试从未覆盖生产环境的记录路径。
func usageMatrixMockStream(apiType UpstreamType) string {
	switch apiType {
	case UpstreamAnthropic:
		return strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_u","type":"message","role":"assistant","model":"matrix-upstream-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":7,"output_tokens":0}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"matrix-ok"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
	case UpstreamResponses:
		resp := `{"id":"resp_u","object":"response","created_at":1710000000,"status":"completed","background":false,"error":null,"incomplete_details":null,"model":"matrix-upstream-model","output":[{"id":"msg_u","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"matrix-ok","annotations":[],"logprobs":[]}]}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}}`
		return strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_u","object":"response","created_at":1710000000,"status":"in_progress","output":[]}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_u","output_index":0,"content_index":0,"delta":"matrix-ok","logprobs":[]}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","sequence_number":4,"response":` + resp + `}`,
			``,
		}, "\n")
	default:
		return strings.Join([]string{
			`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"content":"matrix-ok"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
	}
}

func usageMatrixMockJSON(apiType UpstreamType) string {
	switch apiType {
	case UpstreamAnthropic:
		return `{"id":"msg_u","type":"message","role":"assistant","model":"matrix-upstream-model","content":[{"type":"text","text":"matrix-ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":7,"output_tokens":5}}`
	case UpstreamResponses:
		return `{"id":"resp_u","object":"response","created_at":1710000000,"status":"completed","background":false,"error":null,"incomplete_details":null,"model":"matrix-upstream-model","output":[{"id":"msg_u","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"matrix-ok","annotations":[],"logprobs":[]}]}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}}`
	default:
		return `{"id":"chatcmpl-u","object":"chat.completion","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"matrix-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}}`
	}
}

func usageMockUpstream(t *testing.T, apiType UpstreamType) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		stream, _ := decoded["stream"].(bool)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, usageMatrixMockStream(apiType))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, usageMatrixMockJSON(apiType))
	}))
	t.Cleanup(server.Close)
	return server
}

func usageSnapshot() TokenStatsData {
	tokenStatsMu.Lock()
	defer tokenStatsMu.Unlock()
	cp := TokenStatsData{
		TotalRequests: tokenStats.TotalRequests,
		Models:        map[string]*ModelStats{},
	}
	for k, v := range tokenStats.Models {
		cp.Models[k] = &ModelStats{
			RequestCount:     v.RequestCount,
			PromptTokens:     v.PromptTokens,
			CompletionTokens: v.CompletionTokens,
			TotalTokens:      v.TotalTokens,
		}
	}
	if tokenStats.Daily != nil {
		cp.Daily = &DailyStats{
			Date:          tokenStats.Daily.Date,
			TotalRequests: tokenStats.Daily.TotalRequests,
			Models:        map[string]*ModelStats{},
		}
		for k, v := range tokenStats.Daily.Models {
			cp.Daily.Models[k] = &ModelStats{
				RequestCount:     v.RequestCount,
				PromptTokens:     v.PromptTokens,
				CompletionTokens: v.CompletionTokens,
				TotalTokens:      v.TotalTokens,
			}
		}
	}
	return cp
}

func usageResetStats(t *testing.T) {
	zeroRetries := 0
	_ = zeroRetries
	tokenStatsMu.Lock()
	tokenStats = &TokenStatsData{Models: map[string]*ModelStats{}, Daily: &DailyStats{Date: getToday(), Models: map[string]*ModelStats{}}}
	statsDate = getToday()
	tokenStatsMu.Unlock()
}

// usageMockUpstreamNoTotalStream 创建兼容 OpenAI 的上游，其流式用量数据块有意省略
// total_tokens。许多真实供应商（以及部分 OpenAI 模式）只在最终数据块发送
// prompt_tokens 和 completion_tokens。记录路径必须用 total_tokens = pt + ct
// 作为降级计算，不能静默丢弃用量。
func usageMockUpstreamNoTotalStream(t *testing.T) *httptest.Server {
	t.Helper()
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"delta":{"content":"matrix-ok"},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1710000000,"model":"matrix-upstream-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":5}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		if s, _ := decoded["stream"].(bool); s {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, stream)
			return
		}
		// 非流式响应也省略 total_tokens。
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"chatcmpl-u","object":"chat.completion","created":1710000000,"model":"matrix-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"matrix-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5}}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func usageMockUpstreamWithoutUsage(t *testing.T, apiType UpstreamType) *httptest.Server {
	t.Helper()
	var stream, response string
	switch apiType {
	case UpstreamAnthropic:
		stream = strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_u","type":"message","role":"assistant","model":"matrix-upstream-model","content":[],"stop_reason":null}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"matrix-ok"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}, "\n")
		response = `{"id":"msg_u","type":"message","role":"assistant","model":"matrix-upstream-model","content":[{"type":"text","text":"matrix-ok"}],"stop_reason":"end_turn"}`
	case UpstreamResponses:
		completed := `{"id":"resp_u","object":"response","status":"completed","model":"matrix-upstream-model","output":[{"id":"msg_u","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"matrix-ok","annotations":[]}]}]}`
		stream = strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","response":{"id":"resp_u","object":"response","status":"in_progress","output":[]}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","item_id":"msg_u","output_index":0,"content_index":0,"delta":"matrix-ok"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":` + completed + `}`,
			``,
		}, "\n")
		response = completed
	default:
		stream = strings.Join([]string{
			`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","model":"matrix-upstream-model","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			``,
			`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","model":"matrix-upstream-model","choices":[{"index":0,"delta":{"content":"matrix-ok"},"finish_reason":"stop"}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		response = `{"id":"chatcmpl-u","object":"chat.completion","model":"matrix-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"matrix-ok"},"finish_reason":"stop"}]}`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		if isStream, _ := decoded["stream"].(bool); isStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, stream)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSuccessfulMappedRequestRecordedWithoutUsage(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)

	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			for _, stream := range []bool{false, true} {
				name := fmt.Sprintf("%s_to_%s_nonstream", surface.name, apiType)
				if stream {
					name = fmt.Sprintf("%s_to_%s_stream", surface.name, apiType)
				}
				t.Run(name, func(t *testing.T) {
					usageResetStats(t)
					upstream := usageMockUpstreamWithoutUsage(t, apiType)
					matrixSelectUpstream(upstream.URL, apiType)
					matrixDoRequest(t, gateway, surface, stream)

					snap := usageSnapshot()
					if snap.TotalRequests != 1 || snap.Daily == nil || snap.Daily.TotalRequests != 1 {
						t.Fatalf("successful request without usage was not counted: %+v", snap)
					}
					modelStats := snap.Models[matrixPublicModel]
					if modelStats == nil || modelStats.RequestCount != 1 {
						t.Fatalf("request was not recorded under alias %q: %+v", matrixPublicModel, snap.Models)
					}
					if modelStats.PromptTokens != 0 || modelStats.CompletionTokens != 0 || modelStats.TotalTokens != 0 {
						t.Fatalf("missing usage must produce zero token counts: %+v", modelStats)
					}
				})
			}
		}
	}
}

// TestTokenUsageRecordedWithoutTotalTokens 验证即使上游 usage 对象省略 total_tokens，
// 仍会记录 Token 用量；这在许多兼容 OpenAI 供应商的流式模式中很常见。
func TestTokenUsageRecordedWithoutTotalTokens(t *testing.T) {
	matrixIsolateRuntime(t)
	usageResetStats(t)
	gateway := matrixGatewayServer(t)
	upstream := usageMockUpstreamNoTotalStream(t)
	matrixSelectUpstream(upstream.URL, UpstreamOpenAI)

	// 流式路径。
	t.Run("stream", func(t *testing.T) {
		usageResetStats(t)
		matrixDoRequest(t, gateway, matrixSurface{name: "chat", path: "/v1/chat/completions"}, true)

		snap := usageSnapshot()
		if snap.TotalRequests != 1 {
			t.Fatalf("stream: cumulative TotalRequests = %d, want 1 (usage not recorded)", snap.TotalRequests)
		}
		var ms *ModelStats
		for _, v := range snap.Models {
			ms = v
		}
		if ms == nil {
			t.Fatalf("stream: no model stats recorded: %+v", snap.Models)
		}
		if ms.PromptTokens != 7 || ms.CompletionTokens != 5 {
			t.Fatalf("stream: recorded pt=%d ct=%d, want pt=7 ct=5", ms.PromptTokens, ms.CompletionTokens)
		}
		if ms.TotalTokens != 12 {
			t.Fatalf("stream: recorded TotalTokens = %d, want 12 (pt+ct fallback)", ms.TotalTokens)
		}
	})

	// 非流式路径（已有降级逻辑，此处补充完整性验证）。
	t.Run("nonstream", func(t *testing.T) {
		usageResetStats(t)
		matrixDoRequest(t, gateway, matrixSurface{name: "chat", path: "/v1/chat/completions"}, false)

		snap := usageSnapshot()
		if snap.TotalRequests != 1 {
			t.Fatalf("nonstream: cumulative TotalRequests = %d, want 1 (usage not recorded)", snap.TotalRequests)
		}
		var ms *ModelStats
		for _, v := range snap.Models {
			ms = v
		}
		if ms == nil {
			t.Fatalf("nonstream: no model stats recorded: %+v", snap.Models)
		}
		if ms.TotalTokens != 12 {
			t.Fatalf("nonstream: recorded TotalTokens = %d, want 12 (pt+ct fallback)", ms.TotalTokens)
		}
	})
}

// TestTokenUsageRecordedOnClientDisconnect 验证客户端在流中途断开时仍会记录 Token 用量。
// Claude Code 经常在收到足够数据后立即关闭连接，这是其主要失败模式。
// 修复前，proxyAnthropicPassthroughStream 遇到写入错误会提前返回，
// 无法执行用量记录代码。
func TestTokenUsageRecordedOnClientDisconnect(t *testing.T) {
	matrixIsolateRuntime(t)
	usageResetStats(t)

	// 在 message_start 和 message_delta 中发送用量的 Anthropic SSE 流。
	anthropicStream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_u","type":"message","role":"assistant","model":"matrix-upstream-model","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, anthropicStream)
	}))
	t.Cleanup(upstream.Close)

	matrixSelectUpstream(upstream.URL, UpstreamAnthropic)
	gateway := matrixGatewayServer(t)

	// 通过 /v1/messages（Claude Messages API）发送流式请求，
	// 并在流中途关闭响应体以模拟客户端断开。
	reqBody := `{"model":"matrix-public-model","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := gateway.Client().Do(req)
	if err != nil {
		t.Fatalf("call gateway: %v", err)
	}
	// 读取少量字节后关闭，模拟 Claude Code 断开连接。
	buf := make([]byte, 64)
	_, _ = resp.Body.Read(buf)
	resp.Body.Close()

	// 给服务器少量时间完成上游流处理。
	time.Sleep(100 * time.Millisecond)

	snap := usageSnapshot()
	if snap.TotalRequests != 1 {
		t.Fatalf("cumulative TotalRequests = %d, want 1 (usage lost on client disconnect)", snap.TotalRequests)
	}
	var ms *ModelStats
	for _, v := range snap.Models {
		ms = v
	}
	if ms == nil {
		t.Fatalf("no model stats recorded: %+v", snap.Models)
	}
	if ms.PromptTokens != 10 {
		t.Fatalf("recorded PromptTokens = %d, want 10", ms.PromptTokens)
	}
	if ms.CompletionTokens != 8 {
		t.Fatalf("recorded CompletionTokens = %d, want 8", ms.CompletionTokens)
	}
	if ms.TotalTokens != 18 {
		t.Fatalf("recorded TotalTokens = %d, want 18", ms.TotalTokens)
	}
}

// TestTokenUsageRecordedAcrossMatrix 使用输出非零用量的模拟上游，驱动每个
// （接口 x 上游 x 流模式）组合经过真实处理器，并断言累计统计和每日统计均有增长。
// 现有矩阵测试因模拟用量为零而跳过了这条路径。
func TestTokenUsageRecordedAcrossMatrix(t *testing.T) {
	matrixIsolateRuntime(t)
	usageResetStats(t)
	gateway := matrixGatewayServer(t) // reuse: registers the three handlers

	surfaces := []matrixSurface{
		{name: "chat", path: "/v1/chat/completions"},
		{name: "claude", path: "/v1/messages"},
		{name: "responses", path: "/v1/responses"},
	}
	upstreamTypes := []UpstreamType{UpstreamOpenAI, UpstreamAnthropic, UpstreamResponses}

	for _, surface := range surfaces {
		for _, apiType := range upstreamTypes {
			t.Run(fmt.Sprintf("%s_to_%s_stream", surface.name, apiType), func(t *testing.T) {
				usageResetStats(t)
				upstream := usageMockUpstream(t, apiType)
				matrixSelectUpstream(upstream.URL, apiType)

				body, _ := matrixDoRequest(t, gateway, surface, true)
				_ = body

				snap := usageSnapshot()
				if snap.TotalRequests != 1 {
					t.Fatalf("cumulative TotalRequests = %d, want 1 (usage not recorded)", snap.TotalRequests)
				}
				if snap.Daily == nil || snap.Daily.TotalRequests != 1 {
					t.Fatalf("daily TotalRequests not recorded: %+v", snap.Daily)
				}
				ms := snap.Models[matrixPublicModel]
				if ms == nil {
					t.Fatalf("usage was not recorded under requested alias %q: %+v", matrixPublicModel, snap.Models)
				}
				if _, exists := snap.Models[matrixUpstreamModel]; exists {
					t.Fatalf("usage was recorded under resolved upstream model %q: %+v", matrixUpstreamModel, snap.Models)
				}
				if ms.TotalTokens <= 0 {
					t.Fatalf("recorded TotalTokens = %d, want > 0", ms.TotalTokens)
				}
				if snap.Daily.Models[matrixPublicModel] == nil {
					t.Fatalf("daily usage was not recorded under requested alias %q: %+v", matrixPublicModel, snap.Daily.Models)
				}
			})
			t.Run(fmt.Sprintf("%s_to_%s_nonstream", surface.name, apiType), func(t *testing.T) {
				usageResetStats(t)
				upstream := usageMockUpstream(t, apiType)
				matrixSelectUpstream(upstream.URL, apiType)

				matrixDoRequest(t, gateway, surface, false)

				snap := usageSnapshot()
				if snap.TotalRequests != 1 {
					t.Fatalf("cumulative TotalRequests = %d, want 1 (usage not recorded)", snap.TotalRequests)
				}
				if snap.Daily == nil || snap.Daily.TotalRequests != 1 {
					t.Fatalf("daily TotalRequests not recorded: %+v", snap.Daily)
				}
				ms := snap.Models[matrixPublicModel]
				if ms == nil || ms.TotalTokens <= 0 {
					t.Fatalf("usage was not recorded under requested alias %q: %+v", matrixPublicModel, snap.Models)
				}
				if _, exists := snap.Models[matrixUpstreamModel]; exists {
					t.Fatalf("usage was recorded under resolved upstream model %q: %+v", matrixUpstreamModel, snap.Models)
				}
				if snap.Daily.Models[matrixPublicModel] == nil {
					t.Fatalf("daily usage was not recorded under requested alias %q: %+v", matrixPublicModel, snap.Daily.Models)
				}
			})
		}
	}
}
