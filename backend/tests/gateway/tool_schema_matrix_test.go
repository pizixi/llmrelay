package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFunctionToolStrictAcrossProtocolMatrix(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, surface := range matrixSurfaces {
		for _, apiType := range matrixUpstreamTypes {
			for _, strict := range []any{nil, false, true} {
				t.Run(fmt.Sprintf("%s/%s/strict=%v", surface.name, apiType, strict), func(t *testing.T) {
					upstream, recorder := matrixMockUpstream(t, apiType)
					matrixSelectUpstream(upstream.URL, apiType)
					request := decodePairwiseTestObject(t, matrixClientRequestBody(surface, false))
					schema := map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []any{"query"}, "additionalProperties": false}
					tool := map[string]any{"name": "lookup", "description": "lookup", "parameters": schema}
					if strict != nil {
						tool["strict"] = strict
					}
					switch surface.path {
					case "/v1/messages":
						tool["type"] = "custom"
						tool["input_schema"] = tool["parameters"]
						delete(tool, "parameters")
					case "/v1/chat/completions":
						tool = map[string]any{"type": "function", "function": tool}
					default:
						tool["type"] = "function"
					}
					request["tools"] = []any{tool}
					body, _ := json.Marshal(request)
					req, _ := http.NewRequest(http.MethodPost, gateway.URL+surface.path, strings.NewReader(string(body)))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Llm2api-Bridge-Mode", "strict")
					resp, err := gateway.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					responseBody, _ := io.ReadAll(resp.Body)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("status=%d body=%s", resp.StatusCode, responseBody)
					}
					calls := recorder.snapshot()
					if len(calls) != 1 {
						t.Fatalf("calls=%#v", calls)
					}
					tools := testArray(t, calls[0].body["tools"], "tools")
					if len(tools) != 1 {
						t.Fatalf("tools=%#v", tools)
					}
					got := testObject(t, tools[0], "tool")
					if apiType == UpstreamOpenAI {
						got = testObject(t, got["function"], "function")
					}
					wantStrict := strict
					if strict == nil && apiType == UpstreamResponses && surface.path != "/v1/responses" {
						wantStrict = false
					}
					requireTestEqual(t, "strict", got["strict"], wantStrict)
					requireTestEqual(t, "name", got["name"], "lookup")
					schemaKey := "parameters"
					if apiType == UpstreamAnthropic {
						schemaKey = "input_schema"
					}
					requireTestEqual(t, "additionalProperties", testObject(t, got[schemaKey], schemaKey)["additionalProperties"], false)
				})
			}
		}
	}
}
