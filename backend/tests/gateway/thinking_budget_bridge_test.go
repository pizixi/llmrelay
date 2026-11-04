package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestImpossibleThinkingBudgetRejectedBeforeUpstream(t *testing.T) {
	matrixIsolateRuntime(t)
	gateway := matrixGatewayServer(t)
	for _, surface := range []matrixSurface{matrixSurfaces[0], matrixSurfaces[2]} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", surface.name, streaming), func(t *testing.T) {
				upstream, recorder := matrixMockUpstream(t, UpstreamAnthropic)
				matrixSelectUpstream(upstream.URL, UpstreamAnthropic)
				configMu.Lock()
				alias := modelAlias[matrixPublicModel]
				alias.WithReasoning = true
				modelAlias[matrixPublicModel] = alias
				configMu.Unlock()
				request := decodePairwiseTestObject(t, matrixClientRequestBody(surface, streaming))
				if surface.path == "/v1/responses" {
					request["max_output_tokens"] = 1024
					request["reasoning"] = map[string]any{"effort": "high"}
				} else {
					request["max_completion_tokens"] = 1024
					request["reasoning_effort"] = "high"
				}
				body, _ := json.Marshal(request)
				resp, err := gateway.Client().Post(gateway.URL+surface.path, "application/json", strings.NewReader(string(body)))
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				responseBody, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(responseBody), "max_tokens greater than 1024") {
					t.Fatalf("status=%d body=%s", resp.StatusCode, responseBody)
				}
				if calls := recorder.snapshot(); len(calls) != 0 {
					t.Fatalf("invalid request reached upstream: %#v", calls)
				}
			})
		}
	}
}
