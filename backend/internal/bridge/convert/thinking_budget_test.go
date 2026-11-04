package convert

import (
	"fmt"
	"testing"
)

func TestAnthropicThinkingBudgetAcrossRequestConverters(t *testing.T) {
	for _, limit := range []int{0, 1024, 1025, 1536, 4096, 65536} {
		for _, protocol := range []string{"chat", "responses"} {
			t.Run(fmt.Sprintf("%s/limit=%d", protocol, limit), func(t *testing.T) {
				var body []byte
				var err error
				capField := ""
				if limit > 0 {
					if protocol == "chat" {
						capField = fmt.Sprintf(`,"max_completion_tokens":%d`, limit)
					} else {
						capField = fmt.Sprintf(`,"max_output_tokens":%d`, limit)
					}
				}
				if protocol == "chat" {
					body, err = OpenAIToAnthropicRequestWithError([]byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high","temperature":0` + capField + `}`))
				} else {
					body, _, _, err = ConvertResponsesRequestToAnthropicDirect([]byte(`{"input":"hello","reasoning":{"effort":"high"},"temperature":0`+capField+`}`), "claude-sonnet-4-5", true)
				}
				if limit == 1024 {
					if err == nil {
						t.Fatal("impossible budget was accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				request := decodeConversionTestObject(t, body)
				maxTokens := request["max_tokens"].(float64)
				budget := request["thinking"].(map[string]any)["budget_tokens"].(float64)
				if budget < 1024 || budget >= maxTokens || (limit > 0 && maxTokens != float64(limit)) {
					t.Fatalf("budget=%v max_tokens=%v client limit=%d", budget, maxTokens, limit)
				}
				if request["temperature"] != nil {
					t.Fatal("manual thinking retained incompatible temperature")
				}
			})
		}
	}
}

func TestChatModernTokenLimitTakesPrecedence(t *testing.T) {
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"hello"}],"max_tokens":1000,"max_completion_tokens":2000}`)
	anthropic, err := OpenAIToAnthropicRequestWithError(body)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeConversionTestObject(t, anthropic)["max_tokens"]; got != float64(2000) {
		t.Fatalf("Anthropic max_tokens=%v", got)
	}
	if got := decodeConversionTestObject(t, OpenAIToResponsesRequest(body, nil))["max_output_tokens"]; got != float64(2000) {
		t.Fatalf("Responses max_output_tokens=%v", got)
	}
}

func TestClaude55ModelsUseAdaptiveThinking(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"} {
		body, err := OpenAIToAnthropicRequestWithError([]byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high","max_completion_tokens":100}`, model)))
		if err != nil {
			t.Fatal(err)
		}
		request := decodeConversionTestObject(t, body)
		if request["thinking"].(map[string]any)["type"] != "adaptive" || request["max_tokens"] != float64(100) {
			t.Fatalf("incorrect current-model thinking: %s", body)
		}
	}
}
