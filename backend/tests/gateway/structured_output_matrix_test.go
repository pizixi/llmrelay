package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStructuredOutputAcrossProtocolMatrix(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	// 使用嵌套定义和约束，验证转换不会为适配供应商而删改 Schema。
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"answer": map[string]any{"$ref": "#/$defs/answer"}},
		"required":   []any{"answer"},
		"$defs":      map[string]any{"answer": map[string]any{"type": "string", "enum": []any{"yes", "no"}}},
	}
	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			for _, stream := range []bool{false, true} {
				for _, mode := range []string{"compatible", "strict"} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/%s", surface.name, apiType, stream, mode), func(t *testing.T) {
						upstream, recorder := matrixMockUpstream(t, apiType)
						matrixSelectUpstream(upstream.URL, apiType)
						request := decodePairwiseTestObject(t, matrixClientRequestBody(surface, stream))
						switch surface.path {
						case "/v1/messages":
							request["output_config"] = map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}}
						case "/v1/responses":
							request["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "answer", "strict": true, "schema": schema}}
						default:
							request["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "strict": true, "schema": schema}}
						}
						body, _ := json.Marshal(request)
						req, _ := http.NewRequest(http.MethodPost, gateway.URL+surface.path, strings.NewReader(string(body)))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Llm2api-Bridge-Mode", mode)
						resp, err := gateway.Client().Do(req)
						if err != nil {
							t.Fatal(err)
						}
						defer resp.Body.Close()
						responseBody, _ := io.ReadAll(resp.Body)
						if resp.StatusCode != http.StatusOK {
							t.Fatalf("status=%d body=%s", resp.StatusCode, responseBody)
						}
						if resp.Header.Get("X-Llm2api-Warning-Count") != "" {
							t.Fatalf("supported output format warned: %v", resp.Header)
						}
						if surface.path == "/v1/responses" && apiType != UpstreamResponses {
							var converted map[string]any
							if stream {
								terminal := findTestSSEEvent(t, parseTestSSE(t, string(responseBody)), "response.completed")
								converted = testObject(t, terminal.data["response"], "response")
							} else {
								converted = decodeTestObject(t, responseBody)
							}
							requireTestEqual(t, "response text config", converted["text"], request["text"])
						}
						calls := recorder.snapshot()
						if len(calls) != 1 {
							t.Fatalf("upstream calls=%d", len(calls))
						}
						var format map[string]any
						switch apiType {
						case UpstreamAnthropic:
							format = testObject(t, testObject(t, calls[0].body["output_config"], "output_config")["format"], "format")
						case UpstreamResponses:
							format = testObject(t, testObject(t, calls[0].body["text"], "text")["format"], "format")
						default:
							outer := testObject(t, calls[0].body["response_format"], "response_format")
							requireTestEqual(t, "format type", outer["type"], "json_schema")
							format = testObject(t, outer["json_schema"], "json_schema")
						}
						requireTestEqual(t, "schema", format["schema"], schema)
						if apiType != UpstreamAnthropic {
							requireTestEqual(t, "strict", format["strict"], true)
							if name, _ := format["name"].(string); name == "" {
								t.Fatal("OpenAI output schema requires a name")
							}
						}
					})
				}
			}
		}
	}
}

func TestAnthropicOutputEffortAndFormatRemainIndependent(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, apiType := range matrixUpstreamTypes {
		for _, withReasoning := range []bool{false, true} {
			for _, withFormat := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/reasoning=%t/format=%t", apiType, withReasoning, withFormat), func(t *testing.T) {
					upstream, recorder := matrixMockUpstream(t, apiType)
					matrixSelectUpstream(upstream.URL, apiType)
					configMu.Lock()
					alias := modelAlias[matrixPublicModel]
					alias.WithReasoning = withReasoning
					modelAlias[matrixPublicModel] = alias
					configMu.Unlock()
					request := decodePairwiseTestObject(t, matrixClientRequestBody(matrixSurfaces[1], false))
					config := map[string]any{"effort": "high"}
					if withFormat {
						config["format"] = map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}
					}
					request["output_config"] = config
					body, _ := json.Marshal(request)
					req, _ := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(string(body)))
					req.Header.Set("X-Llm2api-Bridge-Mode", "strict")
					resp, err := gateway.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					responseBody, _ := io.ReadAll(resp.Body)
					if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Llm2api-Warning-Count") != "" {
						t.Fatalf("status=%d headers=%v body=%s", resp.StatusCode, resp.Header, responseBody)
					}
					calls := recorder.snapshot()
					if len(calls) != 1 {
						t.Fatalf("upstream calls=%d", len(calls))
					}
					var effort any
					var format any
					switch apiType {
					case UpstreamAnthropic:
						config, _ := calls[0].body["output_config"].(map[string]any)
						effort, format = config["effort"], config["format"]
					case UpstreamResponses:
						reasoning, _ := calls[0].body["reasoning"].(map[string]any)
						effort, format = reasoning["effort"], calls[0].body["text"]
					default:
						effort, format = calls[0].body["reasoning_effort"], calls[0].body["response_format"]
					}
					if (effort != nil) != withReasoning || (format != nil) != withFormat {
						t.Fatalf("effort=%v format=%v, want reasoning=%t format=%t", effort, format, withReasoning, withFormat)
					}
				})
			}
		}
	}
}

func TestUnsupportedOutputFormatStillWarnsOrRejects(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, surface := range []matrixSurface{matrixSurfaces[0], matrixSurfaces[2]} {
		for _, formatCase := range []struct {
			name   string
			format map[string]any
			code   string
		}{
			{"json object", map[string]any{"type": "json_object"}, "request_field_ignored"},
			{"explicit non-strict", map[string]any{"type": "json_schema", "name": "answer", "strict": false, "schema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}, "structured_output_strictness_changed"},
			{"unknown format", map[string]any{"type": "future_format"}, "request_field_ignored"},
		} {
			for _, mode := range []string{"compatible", "strict"} {
				t.Run(surface.name+"/"+formatCase.name+"/"+mode, func(t *testing.T) {
					upstream, recorder := matrixMockUpstream(t, UpstreamAnthropic)
					matrixSelectUpstream(upstream.URL, UpstreamAnthropic)
					request := decodePairwiseTestObject(t, matrixClientRequestBody(surface, false))
					format := formatCase.format
					if surface.path == "/v1/responses" {
						request["text"] = map[string]any{"format": format}
					} else {
						if format["type"] == "json_schema" {
							format = map[string]any{"type": "json_schema", "json_schema": format}
						}
						request["response_format"] = format
					}
					body, _ := json.Marshal(request)
					req, _ := http.NewRequest(http.MethodPost, gateway.URL+surface.path, strings.NewReader(string(body)))
					req.Header.Set("X-Llm2api-Bridge-Mode", mode)
					resp, err := gateway.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					responseBody, _ := io.ReadAll(resp.Body)
					if mode == "strict" {
						if resp.StatusCode != http.StatusBadRequest || len(recorder.snapshot()) != 0 {
							t.Fatalf("strict status=%d calls=%d body=%s", resp.StatusCode, len(recorder.snapshot()), responseBody)
						}
					} else if resp.StatusCode != http.StatusOK || !strings.Contains(strings.Join(resp.Header.Values("X-Llm2api-Warning"), ","), formatCase.code) {
						t.Fatalf("compatible status=%d warnings=%v body=%s", resp.StatusCode, resp.Header, responseBody)
					}
				})
			}
		}
	}
}
