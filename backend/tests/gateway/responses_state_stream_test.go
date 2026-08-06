package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	bridgestate "llmrelay/backend/internal/bridge/state"
)

func TestResponsesStreamToChatStoresCompletedResponseForLocalState(t *testing.T) {
	matrixIsolateRuntime(t)
	bridgestate.Default().Reset()
	t.Cleanup(bridgestate.Default().Reset)

	upstream, _ := matrixMockUpstream(t, UpstreamOpenAI)
	matrixSelectUpstream(upstream.URL, UpstreamOpenAI)
	gateway := matrixGatewayServer(t)

	body := `{"model":"matrix-public-model","input":"hello","stream":true,"store":true}`
	request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := gateway.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	streamBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, streamBody)
	}

	var storedID string
	for _, event := range parseTestSSE(t, string(streamBody)) {
		if event.name != "response.completed" {
			continue
		}
		responseObject := testObject(t, event.data["response"], "response.completed.response")
		storedID, _ = responseObject["id"].(string)
		break
	}
	if strings.TrimSpace(storedID) == "" {
		t.Fatalf("completed response id missing from stream: %s", streamBody)
	}
	stored, ok := bridgestate.Default().Get(storedID)
	if !ok {
		t.Fatalf("completed response %q was not stored", storedID)
	}
	encoded, _ := json.Marshal(stored)
	if !strings.Contains(string(encoded), "matrix-ok") {
		t.Fatalf("stored response does not contain converted output: %s", encoded)
	}
}
