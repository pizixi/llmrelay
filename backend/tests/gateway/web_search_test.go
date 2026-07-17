package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSearXNGWebSearchProviderNormalizesAndLimitsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "latest glm" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("unexpected search request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":" Result A ","url":"https://example.com/a","content":"first result","score":0.9},
			{"title":"Bad","url":"javascript:alert(1)","content":"unsafe"},
			{"title":"Result B","url":"https://example.com/b","content":"second result","score":0.8}
		]}`))
	}))
	defer server.Close()

	cfg := normalizeWebSearchConfig(WebSearchConfig{
		Enabled: true, Provider: "searxng", BaseURL: server.URL,
		MaxResults: 2, MaxResultBytes: 8192,
	})
	result, err := webSearchWithTimeout(context.Background(), cfg, "latest glm")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || result.Results[0].Title != "Result A" || result.Results[1].URL != "https://example.com/b" {
		t.Fatalf("unexpected normalized results: %#v", result.Results)
	}
}

func TestTavilyWebSearchProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", r.Method)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["api_key"] != "test-key" || request["query"] != "gateway search" || request["max_results"] != float64(3) {
			t.Fatalf("unexpected Tavily request: %#v", request)
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"Gateway","url":"https://example.com/gateway","content":"search result","score":0.7}]}`))
	}))
	defer server.Close()
	result, err := webSearchWithTimeout(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "tavily", BaseURL: server.URL, APIKey: "test-key",
		MaxResults: 3, TimeoutSeconds: 5, MaxResultBytes: 8192,
	}, "gateway search")
	if err != nil || len(result.Results) != 1 || result.Results[0].URL != "https://example.com/gateway" {
		t.Fatalf("Tavily result=%#v err=%v", result, err)
	}
}

func TestDuckDuckGoLiteWebSearchProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Hefei weather" {
			t.Fatalf("query=%q", r.URL.Query().Get("q"))
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "Mozilla/5.0") || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			t.Fatalf("missing browser headers: %#v", r.Header)
		}
		_, _ = w.Write([]byte(`<html><body><table>
			<tr><td><a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fweather&amp;rut=opaque"> Hefei <b>Weather</b> </a></td></tr>
			<tr><td class="result-snippet">Current weather &amp; forecast</td></tr>
			<tr><td><a class="result-link" href="javascript:alert(1)">Unsafe</a></td></tr>
			<tr><td class="result-snippet">Discard this result</td></tr>
		</table></body></html>`))
	}))
	defer server.Close()

	duckDuckGoSearchRate.Lock()
	duckDuckGoSearchRate.last = time.Time{}
	duckDuckGoSearchRate.Unlock()
	result, err := webSearchWithTimeout(context.Background(), WebSearchConfig{
		Enabled: true, Provider: webSearchProviderDuckDuckGo, BaseURL: server.URL,
		MaxResults: 3, TimeoutSeconds: 5, MaxResultBytes: 8192,
	}, "Hefei weather")
	if err != nil || len(result.Results) != 1 {
		t.Fatalf("DuckDuckGo result=%#v err=%v", result, err)
	}
	if result.Results[0].Title != "Hefei Weather" || result.Results[0].URL != "https://example.com/weather" || result.Results[0].Snippet != "Current weather & forecast" {
		t.Fatalf("unexpected parsed result: %#v", result.Results[0])
	}
}

func TestDuckDuckGoPublicEndpointIntegration(t *testing.T) {
	if os.Getenv("LLMGATEWAYGO_TEST_PUBLIC_SEARCH") != "1" {
		t.Skip("set LLMGATEWAYGO_TEST_PUBLIC_SEARCH=1 to test the public endpoint")
	}
	duckDuckGoSearchRate.Lock()
	duckDuckGoSearchRate.last = time.Time{}
	duckDuckGoSearchRate.Unlock()
	result, err := webSearchWithTimeout(context.Background(), WebSearchConfig{
		Enabled: true, Provider: webSearchProviderDuckDuckGo,
		MaxResults: 3, TimeoutSeconds: 15, MaxResultBytes: 8192,
	}, "Hefei weather")
	if err != nil || len(result.Results) == 0 {
		t.Fatalf("public DuckDuckGo result count=%d err=%v", len(result.Results), err)
	}
}

type testWebSearchProvider func(context.Context, string, int) ([]webSearchResult, error)

func (provider testWebSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	return provider(ctx, query, maxResults)
}

func TestSearXNGFallbackUsesDuckDuckGoWithoutExhaustingDeadline(t *testing.T) {
	primaryCalls := 0
	fallbackCalls := 0
	provider := &fallbackWebSearchProvider{
		primaryBudget: 20 * time.Millisecond,
		primary: testWebSearchProvider(func(ctx context.Context, _ string, _ int) ([]webSearchResult, error) {
			primaryCalls++
			<-ctx.Done()
			return nil, ctx.Err()
		}),
		fallback: testWebSearchProvider(func(_ context.Context, query string, _ int) ([]webSearchResult, error) {
			fallbackCalls++
			return []webSearchResult{{Title: "Fallback", URL: "https://example.com/fallback", Snippet: query}}, nil
		}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results, err := provider.Search(ctx, "fallback query", 3)
	if err != nil || len(results) != 1 || primaryCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("results=%#v primary=%d fallback=%d err=%v", results, primaryCalls, fallbackCalls, err)
	}
}

func searxngDirectoryTestInstance(status int, median, success, uptime float64) map[string]any {
	return map[string]any{
		"analytics": false, "main": true, "network_type": "normal", "generator": "searxng", "version": "test",
		"http": map[string]any{"status_code": status, "grade": "A+"},
		"tls":  map[string]any{"grade": "A+"},
		"timing": map[string]any{"search": map[string]any{
			"success_percentage": success, "all": map[string]any{"median": median, "mean": median},
		}},
		"uptime": map[string]any{"uptimeDay": uptime, "uptimeWeek": uptime, "uptimeMonth": uptime},
	}
}

func TestSearXNGDirectoryFiltersAndSortsHealthyInstances(t *testing.T) {
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{
			"https://slow.example/": searxngDirectoryTestInstance(200, 1.5, 100, 100),
			"https://fast.example/": searxngDirectoryTestInstance(200, 0.3, 100, 99),
			"https://down.example/": searxngDirectoryTestInstance(502, 0.1, 100, 100),
			"https://poor.example/": searxngDirectoryTestInstance(200, 0.2, 50, 100),
		}})
	}))
	defer directory.Close()
	instances, _, err := listSearXNGPublicInstances(context.Background(), directory.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 3 || instances[0].URL != "https://fast.example/" || instances[1].URL != "https://slow.example/" || instances[2].URL != "https://poor.example/" || instances[2].AutoEligible {
		t.Fatalf("unexpected ranked instances: %#v", instances)
	}
}

func TestAutomaticSearXNGSelectionFailsOverAndRemembersSuccess(t *testing.T) {
	badCalls := 0
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badCalls++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	goodCalls := 0
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goodCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"title": "Healthy", "url": "https://example.com/healthy", "content": "ok"},
		}})
	}))
	defer good.Close()
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{
			bad.URL + "/":  searxngDirectoryTestInstance(200, 0.1, 100, 100),
			good.URL + "/": searxngDirectoryTestInstance(200, 0.2, 100, 100),
		}})
	}))
	defer directory.Close()
	provider := &autoSearxngSearchProvider{directoryURL: directory.URL}
	results, err := provider.Search(context.Background(), "health", 3)
	if err != nil || len(results) != 1 || badCalls != 1 || goodCalls != 1 {
		t.Fatalf("first selection results=%#v bad=%d good=%d err=%v", results, badCalls, goodCalls, err)
	}
	results, err = provider.Search(context.Background(), "health again", 3)
	if err != nil || len(results) != 1 || badCalls != 1 || goodCalls != 2 {
		t.Fatalf("preferred selection results=%#v bad=%d good=%d err=%v", results, badCalls, goodCalls, err)
	}
}

func TestSearXNGInstancesAdminHandler(t *testing.T) {
	matrixIsolateRuntime(t)
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{
			"https://admin.example/": searxngDirectoryTestInstance(200, 0.4, 100, 100),
		}})
	}))
	defer directory.Close()
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Provider: "searxng", SearXNGMode: searxngModeAuto, SearXNGDirectoryURL: directory.URL})
	configMu.Unlock()
	response := httptest.NewRecorder()
	searxngInstancesHandler(response, httptest.NewRequest(http.MethodGet, "/api/searxng/instances?refresh=1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	decoded := decodeTestObject(t, response.Body.Bytes())
	instances := testArray(t, decoded["instances"], "instances")
	requireTestEqual(t, "admin instance URL", testObject(t, instances[0], "instance")["url"], "https://admin.example/")
}

func TestGatewayWebSearchLoopExecutesAndAggregatesUsage(t *testing.T) {
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"GLM release","url":"https://example.com/glm","content":"released today","score":1}]}`))
	}))
	defer searchServer.Close()

	var mu sync.Mutex
	var calls []map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		calls = append(calls, request)
		callNumber := len(calls)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if callNumber == 1 {
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-search-1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{
					"id":"call_search","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"latest glm\"}"}
				}]}}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-search-2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"GLM was released today [source](https://example.com/glm)."}}],
			"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}
		}`))
	}))
	defer upstreamServer.Close()

	tools := []ResponsesTool{{Type: "web_search"}}
	converted, mappings, warnings := convertResponsesToolsWithMappingsDetailed(tools, true)
	if len(converted) != 1 || len(warnings) != 1 || warnings[0].Code != "hosted_web_search_fallback" {
		t.Fatalf("fallback conversion tools=%#v warnings=%#v", converted, warnings)
	}
	request := OpenAIRequest{
		Model: "glm", Messages: []Message{{Role: "user", Content: "What is new?"}}, Tools: converted,
		AdditionalFields: map[string]any{},
	}
	result := executeGatewayWebSearchLoop(
		context.Background(), request, true, "search-upstream",
		&UpstreamConfig{BaseURL: upstreamServer.URL, APIType: UpstreamOpenAI}, mappings,
		WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL, MaxResults: 5, MaxToolRounds: 2, MaxResultBytes: 8192},
	)
	if result.Err != nil || result.Status != http.StatusOK {
		t.Fatalf("loop result status=%d err=%v body=%s", result.Status, result.Err, result.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("upstream calls=%d, want 2", len(calls))
	}
	secondMessages := testArray(t, calls[1]["messages"], "second messages")
	last := testObject(t, secondMessages[len(secondMessages)-1], "tool result")
	if last["role"] != "tool" || !strings.Contains(last["content"].(string), "https://example.com/glm") {
		t.Fatalf("tool result=%#v", last)
	}
	var response map[string]any
	if err := json.Unmarshal(result.Body, &response); err != nil {
		t.Fatal(err)
	}
	usage := testObject(t, response["usage"], "usage")
	requireTestEqual(t, "aggregated prompt tokens", usage["prompt_tokens"], float64(13))
	requireTestEqual(t, "aggregated completion tokens", usage["completion_tokens"], float64(6))
	choices := testArray(t, response["choices"], "choices")
	message := testObject(t, testObject(t, choices[0], "choice")["message"], "message")
	providerOutput := testArray(t, message["provider_output"], "provider output")
	webCall := testObject(t, providerOutput[0], "web search call")
	requireTestEqual(t, "web search output type", webCall["type"], "web_search_call")
}

func TestGatewayWebSearchLoopDoesNotExecuteClientTools(t *testing.T) {
	upstreamCalls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_, _ = w.Write([]byte(`{
			"id":"chat-client-tool","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{
				"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{\"id\":1}"}
			}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer upstreamServer.Close()
	tools, mappings, _ := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{
		{Type: "web_search"},
		{Type: "function", Name: "lookup", Parameters: map[string]any{"type": "object"}},
	}, true)
	result := executeGatewayWebSearchLoop(
		context.Background(), OpenAIRequest{Model: "glm", Messages: []Message{{Role: "user", Content: "lookup"}}, Tools: tools},
		true, "client-tool", &UpstreamConfig{BaseURL: upstreamServer.URL, APIType: UpstreamOpenAI}, mappings,
		WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: "https://search.invalid", MaxToolRounds: 2},
	)
	if result.Err != nil || upstreamCalls != 1 || len(result.Traces) != 0 {
		t.Fatalf("client tool was intercepted: calls=%d traces=%#v err=%v", upstreamCalls, result.Traces, result.Err)
	}
	_, calls, err := parseChatToolCalls(result.Body)
	if err != nil || len(calls) != 1 || calls[0].Function.Name != "lookup" {
		t.Fatalf("client tool not preserved: calls=%#v err=%v body=%s", calls, err, result.Body)
	}
}

func TestGatewayWebSearchLoopRemovesSearchAfterFirstFailure(t *testing.T) {
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer searchServer.Close()
	var calls []map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, request)
		if len(calls) == 1 {
			_, _ = w.Write([]byte(`{"id":"chat-f1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"search_failed","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"weather\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chat-f2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Search is unavailable."}}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
	}))
	defer upstreamServer.Close()
	tools, mappings, _ := convertResponsesToolsWithMappingsDetailed([]ResponsesTool{{Type: "web_search"}}, true)
	result := executeGatewayWebSearchLoop(
		context.Background(), OpenAIRequest{Model: "glm", Messages: []Message{{Role: "user", Content: "weather"}}, Tools: tools},
		true, "failure", &UpstreamConfig{BaseURL: upstreamServer.URL, APIType: UpstreamOpenAI}, mappings,
		WebSearchConfig{Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom, BaseURL: searchServer.URL, MaxToolRounds: 2},
	)
	if result.Err != nil || len(calls) != 2 || len(result.Traces) != 1 || result.Traces[0].Error == "" {
		t.Fatalf("result=%#v calls=%d", result, len(calls))
	}
	secondTools, _ := calls[1]["tools"].([]any)
	for _, raw := range secondTools {
		tool := testObject(t, raw, "second tool")
		function, _ := tool["function"].(map[string]any)
		if function != nil && function["name"] == internalWebSearchToolName {
			t.Fatalf("failed search remained available on second model call: %#v", secondTools)
		}
	}
}

func TestResponsesWebSearchHistoryReplaysWithoutID(t *testing.T) {
	input := []any{map[string]any{
		"type": "web_search_call", "status": "completed",
		"action": map[string]any{
			"type": "search", "query": "current weather",
			"sources": []any{map[string]any{"type": "url", "url": "https://example.com/weather", "title": "Weather"}},
		},
	}}
	messages, warnings := responsesInputToMessagesWithWarnings(input, "", map[string]ResponseToolNameMapping{
		internalWebSearchToolName: {Kind: "web_search", Name: "web_search"},
	})
	if len(warnings) != 0 || len(messages) != 2 {
		t.Fatalf("messages=%#v warnings=%#v", messages, warnings)
	}
	if len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Function.Name != internalWebSearchToolName {
		t.Fatalf("search call history=%#v", messages[0])
	}
	if messages[0].ToolCalls[0].ID == "" || messages[1].ToolCallID != messages[0].ToolCalls[0].ID || messages[1].Role != "tool" {
		t.Fatalf("unpaired search history=%#v", messages)
	}
	if !strings.Contains(messages[1].Content.(string), "https://example.com/weather") {
		t.Fatalf("search sources missing from history: %#v", messages[1])
	}
}

func TestWebSearchNetworkErrorRedactsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	secretQuery := "private customer acquisition query"
	_, err := webSearchWithTimeout(context.Background(), WebSearchConfig{
		Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom,
		BaseURL: server.URL, TimeoutSeconds: 1, MaxResults: 3, MaxResultBytes: 8192,
	}, secretQuery)
	if !errors.Is(err, errWebSearchRequestTimeout) {
		t.Fatalf("error=%v, want sanitized timeout", err)
	}
	if strings.Contains(err.Error(), secretQuery) || strings.Contains(err.Error(), "q=") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("search error leaked query or URL: %v", err)
	}
}

func TestWebSearchLogsSuccessAndFailureWithoutQuery(t *testing.T) {
	var captured bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&captured)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "unavailable", status)
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"Result","url":"https://example.com","content":"ok"}]}`))
	}))
	defer server.Close()
	cfg := WebSearchConfig{
		Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom,
		BaseURL: server.URL, TimeoutSeconds: 1, MaxResults: 3, MaxResultBytes: 8192,
	}
	secretQuery := "private success search query"
	if _, err := webSearchWithTimeout(context.Background(), cfg, secretQuery); err != nil {
		t.Fatal(err)
	}
	successLogs := captured.String()
	if !strings.Contains(successLogs, "[内置 Web Search] 搜索完成") {
		t.Fatalf("成功调用日志不完整：%s", successLogs)
	}
	if strings.Count(successLogs, "[内置 Web Search]") != 1 {
		t.Fatalf("成功调用应只记录一条结果日志：%s", successLogs)
	}
	if strings.Contains(successLogs, secretQuery) {
		t.Fatalf("成功调用日志泄露了搜索词：%s", successLogs)
	}

	captured.Reset()
	status = http.StatusServiceUnavailable
	secretQuery = "private failed search query"
	if _, err := webSearchWithTimeout(context.Background(), cfg, secretQuery); err == nil {
		t.Fatal("预期 Web Search 调用失败")
	}
	failureLogs := captured.String()
	if !strings.Contains(failureLogs, "[内置 Web Search] 搜索失败") {
		t.Fatalf("失败调用日志不完整：%s", failureLogs)
	}
	if strings.Count(failureLogs, "[内置 Web Search]") != 1 {
		t.Fatalf("失败调用应只记录一条结果日志：%s", failureLogs)
	}
	if strings.Contains(failureLogs, secretQuery) {
		t.Fatalf("失败调用日志泄露了搜索词：%s", failureLogs)
	}
}

func TestAutomaticSearXNGStopsAtOverallDeadlineAndSummarizes(t *testing.T) {
	rateCalls := 0
	rateLimited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rateCalls++
		http.Error(w, "limited", http.StatusTooManyRequests)
	}))
	defer rateLimited.Close()
	slowCalls := 0
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowCalls++
		<-r.Context().Done()
	}))
	defer slow.Close()
	unreachedCalls := 0
	unreached := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unreachedCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer unreached.Close()
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{
			rateLimited.URL + "/": searxngDirectoryTestInstance(200, 0.1, 100, 100),
			slow.URL + "/":        searxngDirectoryTestInstance(200, 0.2, 100, 100),
			unreached.URL + "/":   searxngDirectoryTestInstance(200, 0.3, 100, 100),
		}})
	}))
	defer directory.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := (&autoSearxngSearchProvider{directoryURL: directory.URL}).Search(ctx, "sensitive deadline query", 3)
	var summary *searxngSearchFailureSummary
	if !errors.As(err, &summary) {
		t.Fatalf("error=%T %v, want summary", err, err)
	}
	if summary.Attempted != 2 || summary.RateLimited != 1 || !summary.DeadlineExceeded || rateCalls != 1 || slowCalls != 1 || unreachedCalls != 0 {
		t.Fatalf("summary=%#v calls rate=%d slow=%d unreached=%d", summary, rateCalls, slowCalls, unreachedCalls)
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), rateLimited.URL) {
		t.Fatalf("summary leaked query or candidate URL: %v", err)
	}
}

func TestResponsesHandlerNegotiatesNativeBeforeFallback(t *testing.T) {
	t.Run("Chat executes gateway fallback", func(t *testing.T) {
		matrixIsolateRuntime(t)
		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"title":"Current source","url":"https://example.com/current","content":"current fact"}]}`))
		}))
		defer searchServer.Close()

		var mu sync.Mutex
		upstreamCalls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			upstreamCalls++
			call := upstreamCalls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"unsupported web_search_options"}}`))
				return
			}
			if call == 2 {
				_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"search_1","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"current fact\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"chatcmpl-2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Current fact: [source](https://example.com/current)."}}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
		configMu.Unlock()

		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"what is current?","tools":[{"type":"web_search"}]
		}`))
		response := httptest.NewRecorder()
		responsesHandler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		mu.Lock()
		defer mu.Unlock()
		if upstreamCalls != 3 {
			t.Fatalf("upstream calls=%d, want 3", upstreamCalls)
		}
		converted := decodeTestObject(t, response.Body.Bytes())
		output := testArray(t, converted["output"], "output")
		foundSearch := false
		for _, raw := range output {
			item := testObject(t, raw, "output item")
			if item["type"] == "web_search_call" {
				foundSearch = true
			}
		}
		if !foundSearch || !strings.Contains(response.Body.String(), "https://example.com/current") {
			t.Fatalf("fallback response did not preserve search source: %s", response.Body.String())
		}
	})

	t.Run("Responses keeps native search", func(t *testing.T) {
		matrixIsolateRuntime(t)
		upstreamServer, recorder := matrixMockUpstream(t, UpstreamResponses)
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: "https://search.invalid"})
		configMu.Unlock()

		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"latest","tools":[{"type":"web_search"}]
		}`))
		response := httptest.NewRecorder()
		responsesHandler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		calls := recorder.snapshot()
		if len(calls) != 1 || calls[0].path != "/v1/responses" {
			t.Fatalf("native calls=%#v", calls)
		}
		tools := testArray(t, calls[0].body["tools"], "native tools")
		requireTestEqual(t, "native hosted tool", testObject(t, tools[0], "native tool")["type"], "web_search")
		if strings.Contains(response.Header().Get("X-Llm2api-Warning"), "hosted_web_search_fallback") {
			t.Fatalf("native request unexpectedly used fallback: %#v", response.Header())
		}
	})

	t.Run("Responses automatically falls back after native rejection", func(t *testing.T) {
		matrixIsolateRuntime(t)
		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"title":"Fallback source","url":"https://example.com/fallback","content":"fallback fact"}]}`))
		}))
		defer searchServer.Close()
		var mu sync.Mutex
		upstreamCalls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			upstreamCalls++
			call := upstreamCalls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"unsupported web_search tool"}}`))
				return
			}
			if call == 2 {
				_, _ = w.Write([]byte(`{
					"id":"resp-first","object":"response","status":"completed","model":"matrix-upstream-model",
					"output":[{"id":"fc_1","type":"function_call","status":"completed","call_id":"search_1","name":"llm2api_web_search","arguments":"{\"query\":\"fallback fact\"}"}],
					"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"id":"resp-final","object":"response","status":"completed","model":"matrix-upstream-model",
				"output":[{"id":"msg_final","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Fallback fact [source](https://example.com/fallback).","annotations":[]}]}],
				"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}
			}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
		configMu.Unlock()

		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"find fallback fact","tools":[{"type":"web_search"}]
		}`))
		response := httptest.NewRecorder()
		responsesHandler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if response.Header().Get("X-Llm2api-Bridge-Path") != string(BridgePathPivot) {
			t.Fatalf("bridge path=%q, want pivot", response.Header().Get("X-Llm2api-Bridge-Path"))
		}
		mu.Lock()
		defer mu.Unlock()
		if upstreamCalls != 3 || !strings.Contains(response.Body.String(), "https://example.com/fallback") {
			t.Fatalf("calls=%d response=%s", upstreamCalls, response.Body.String())
		}
	})
}

func TestResponsesHandlerWebSearchFallbackStream(t *testing.T) {
	matrixIsolateRuntime(t)
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"Stream source","url":"https://example.com/stream","content":"stream fact"}]}`))
	}))
	defer searchServer.Close()
	var calls int
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":{"message":"web_search is not supported"}}`))
			return
		}
		if calls == 2 {
			_, _ = w.Write([]byte(`{"id":"chat-1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"search_stream","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"stream fact\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chat-2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Stream answer."}}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
	}))
	defer upstreamServer.Close()
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
	configMu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"stream it","stream":true,"tools":[{"type":"web_search"}]
	}`))
	response := httptest.NewRecorder()
	responsesHandler(response, request)
	if response.Code != http.StatusOK || calls != 3 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	stream := response.Body.String()
	for _, expected := range []string{"event: response.created", "event: response.output_text.delta", "event: response.completed", `"type":"web_search_call"`} {
		if !strings.Contains(stream, expected) {
			t.Fatalf("stream missing %q:\n%s", expected, stream)
		}
	}
}

func TestForcedHostedWebSearchFallsBackAfterSuccessfulZeroSearch(t *testing.T) {
	t.Run("Responses non-stream", func(t *testing.T) {
		matrixIsolateRuntime(t)
		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"title":"Gateway source","url":"https://example.com/gateway","content":"gateway fact"}]}`))
		}))
		defer searchServer.Close()
		calls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			switch calls {
			case 1:
				_, _ = w.Write([]byte(`{
					"id":"resp-zero","object":"response","status":"completed","model":"matrix-upstream-model",
					"output":[{"id":"msg_zero","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Search was ignored.","annotations":[]}]}],
					"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
				}`))
			case 2, 4:
				_, _ = w.Write([]byte(`{
					"id":"resp-tool","object":"response","status":"completed","model":"matrix-upstream-model",
					"output":[{"id":"fc_1","type":"function_call","status":"completed","call_id":"search_1","name":"llm2api_web_search","arguments":"{\"query\":\"gateway fact\"}"}],
					"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
				}`))
			default:
				_, _ = w.Write([]byte(`{
					"id":"resp-final","object":"response","status":"completed","model":"matrix-upstream-model",
					"output":[{"id":"msg_final","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Gateway fact.","annotations":[]}]}],
					"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}
				}`))
			}
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
		configMu.Unlock()

		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"find gateway fact",
			"tools":[{"type":"web_search"}],"tool_choice":{"type":"web_search"}
		}`))
		response := httptest.NewRecorder()
		responsesHandler(response, request)
		if response.Code != http.StatusOK || calls != 3 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "https://example.com/gateway") || strings.Contains(response.Body.String(), "Search was ignored") {
			t.Fatalf("zero-search response was not replaced by gateway fallback: %s", response.Body.String())
		}
		if response.Header().Get("X-Llm2api-Bridge-Path") != string(BridgePathPivot) {
			t.Fatalf("bridge path=%q, want pivot", response.Header().Get("X-Llm2api-Bridge-Path"))
		}

		cachedResponse := httptest.NewRecorder()
		responsesHandler(cachedResponse, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"find gateway fact again",
			"tools":[{"type":"web_search"}],"tool_choice":{"type":"web_search"}
		}`)))
		if cachedResponse.Code != http.StatusOK || calls != 5 {
			t.Fatalf("cached status=%d calls=%d body=%s", cachedResponse.Code, calls, cachedResponse.Body.String())
		}
		if strings.Contains(cachedResponse.Body.String(), "Search was ignored") {
			t.Fatalf("cached unsupported capability retried native search: %s", cachedResponse.Body.String())
		}
	})

	t.Run("Anthropic stream used by Claude Code", func(t *testing.T) {
		matrixIsolateRuntime(t)
		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"title":"Shanghai weather","url":"https://example.com/weather","content":"Sunny"}]}`))
		}))
		defer searchServer.Close()
		calls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(strings.Join([]string{
					testNamedSSE("response.created", `{"type":"response.created","response":{"id":"resp-zero","status":"in_progress","output":[]}}`),
					testNamedSSE("response.completed", `{"type":"response.completed","response":{"id":"resp-zero","status":"completed","output":[{"id":"msg_zero","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"No search.","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`),
				}, "")))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if calls == 2 {
				_, _ = w.Write([]byte(`{
					"id":"resp-tool","object":"response","status":"completed","model":"matrix-upstream-model",
					"output":[{"id":"fc_1","type":"function_call","status":"completed","call_id":"search_1","name":"llm2api_web_search","arguments":"{\"query\":\"上海天气 2026年7月15日\"}"}],
					"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"id":"resp-final","object":"response","status":"completed","model":"matrix-upstream-model",
				"output":[{"id":"msg_final","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Shanghai is sunny.","annotations":[]}]}],
				"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}
			}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
		configMu.Unlock()

		request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
			"model":"matrix-public-model","max_tokens":128,"stream":true,
			"messages":[{"role":"user","content":"Perform a web search for the query: 上海天气 2026年7月15日"}],
			"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":8}],
			"tool_choice":{"type":"tool","name":"web_search"}
		}`))
		response := httptest.NewRecorder()
		claudeMessagesHandler(response, request)
		stream := response.Body.String()
		if response.Code != http.StatusOK || calls != 3 {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, stream)
		}
		for _, expected := range []string{`"type":"server_tool_use"`, `"type":"web_search_tool_result"`, `"web_search_requests":1`, "https://example.com/weather"} {
			if !strings.Contains(stream, expected) {
				t.Fatalf("fallback stream missing %q:\n%s", expected, stream)
			}
		}
	})
}

func TestAutomaticHostedSearchFallbackDoesNotOverrideOptionalOrStrictRequests(t *testing.T) {
	t.Run("forced native search with evidence stays native", func(t *testing.T) {
		matrixIsolateRuntime(t)
		calls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			_, _ = w.Write([]byte(`{
				"id":"resp-native","object":"response","status":"completed","model":"matrix-upstream-model",
				"output":[
					{"id":"ws_native","type":"web_search_call","status":"completed","action":{"type":"search","query":"native","sources":[{"type":"url","url":"https://example.com/native","title":"Native"}]}},
					{"id":"msg_native","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Native answer.","annotations":[]}]}
				]
			}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: "https://search.invalid"})
		configMu.Unlock()
		response := httptest.NewRecorder()
		responsesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"must search","tools":[{"type":"web_search"}],"tool_choice":{"type":"web_search"}
		}`)))
		if response.Code != http.StatusOK || calls != 1 || !strings.Contains(response.Body.String(), "https://example.com/native") {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
		}
		if response.Header().Get("X-Llm2api-Bridge-Path") != string(BridgePathPassthrough) {
			t.Fatalf("native search bridge path=%q", response.Header().Get("X-Llm2api-Bridge-Path"))
		}
	})

	t.Run("optional search may legitimately be unused", func(t *testing.T) {
		matrixIsolateRuntime(t)
		calls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			_, _ = w.Write([]byte(`{
				"id":"resp-plain","object":"response","status":"completed","model":"matrix-upstream-model",
				"output":[{"id":"msg_plain","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Plain answer.","annotations":[]}]}]
			}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: "https://search.invalid"})
		configMu.Unlock()
		response := httptest.NewRecorder()
		responsesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"say hello","tools":[{"type":"web_search"}]
		}`)))
		if response.Code != http.StatusOK || calls != 1 || !strings.Contains(response.Body.String(), "Plain answer") {
			t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
		}
	})

	t.Run("strict mode never auto-falls back", func(t *testing.T) {
		matrixIsolateRuntime(t)
		searchCalls := 0
		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			searchCalls++
			_, _ = w.Write([]byte(`{"results":[]}`))
		}))
		defer searchServer.Close()
		upstreamCalls := 0
		upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalls++
			_, _ = w.Write([]byte(`{
				"id":"resp-zero","object":"response","status":"completed","model":"matrix-upstream-model",
				"output":[{"id":"msg_zero","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Strict zero search.","annotations":[]}]}]
			}`))
		}))
		defer upstreamServer.Close()
		matrixSelectUpstream(upstreamServer.URL, UpstreamResponses)
		configMu.Lock()
		webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
		upstreamCfgs["matrix"].BridgeMode = BridgeModeStrict
		upstreamCfg = cloneUpstreamConfig(upstreamCfgs["matrix"])
		configMu.Unlock()
		response := httptest.NewRecorder()
		responsesHandler(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
			"model":"matrix-public-model","input":"must search","tools":[{"type":"web_search"}],"tool_choice":{"type":"web_search"}
		}`)))
		if response.Code != http.StatusOK || upstreamCalls != 1 || searchCalls != 0 || !strings.Contains(response.Body.String(), "Strict zero search") {
			t.Fatalf("status=%d upstream_calls=%d search_calls=%d body=%s", response.Code, upstreamCalls, searchCalls, response.Body.String())
		}
	})
}

func TestAnthropicHandlerUsesWebSearchFallbackForChat(t *testing.T) {
	matrixIsolateRuntime(t)
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"title": "Hefei weather", "url": "https://example.com/weather", "content": "sunny"},
		}})
	}))
	defer searchServer.Close()
	var calls []map[string]any
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, request)
		w.Header().Set("Content-Type", "application/json")
		if len(calls) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown web_search_options field"}}`))
			return
		}
		if len(calls) == 2 {
			_, _ = w.Write([]byte(`{"id":"chat-a1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"srv_search_1","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"Hefei weather\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chat-a2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Hefei is sunny."}}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
	}))
	defer upstreamServer.Close()
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{
		Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom, BaseURL: searchServer.URL,
	})
	configMu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"matrix-public-model","max_tokens":128,
		"messages":[{"role":"user","content":"weather?"}],
		"tools":[{"type":"web_search_20250305","name":"web_search"}]
	}`))
	response := httptest.NewRecorder()
	claudeMessagesHandler(response, request)
	if response.Code != http.StatusOK || len(calls) != 3 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, len(calls), response.Body.String())
	}
	if _, ok := calls[0]["web_search_options"].(map[string]any); !ok {
		t.Fatalf("native Chat search was not attempted first: %#v", calls[0])
	}
	firstTools := testArray(t, calls[1]["tools"], "fallback tools")
	firstTool := testObject(t, firstTools[0], "first tool")
	function := testObject(t, firstTool["function"], "function")
	requireTestEqual(t, "fallback tool name", function["name"], internalWebSearchToolName)
	converted := decodeTestObject(t, response.Body.Bytes())
	content := testArray(t, converted["content"], "content")
	usage := testObject(t, converted["usage"], "usage")
	serverToolUse := testObject(t, usage["server_tool_use"], "usage.server_tool_use")
	requireTestEqual(t, "fallback web search request count", serverToolUse["web_search_requests"], float64(1))
	blockTypes := map[string]bool{}
	for _, raw := range content {
		blockTypes[bridgeString(testObject(t, raw, "content block")["type"])] = true
	}
	for _, blockType := range []string{"server_tool_use", "web_search_tool_result", "text"} {
		if !blockTypes[blockType] {
			t.Fatalf("missing Anthropic block %q: %s", blockType, response.Body.String())
		}
	}
	for _, warning := range response.Header().Values("X-Llm2api-Warning") {
		if strings.Contains(warning, "unsupported_anthropic_server_tool") {
			t.Fatalf("fallback still reported unsupported server tool: %#v", response.Header())
		}
	}
}

func TestChatWebSearchEvidenceMapsToAnthropicUsage(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantBlocks bool
	}{
		{
			name:       "provider output",
			body:       `{"id":"chat_web","choices":[{"finish_reason":"stop","message":{"content":"answer","provider_output":[{"id":"ws_1","type":"web_search_call","status":"completed","action":{"type":"search","query":"weather","sources":[{"type":"url","url":"https://example.com/weather","title":"Weather"}]}}]}}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`,
			wantBlocks: true,
		},
		{
			name: "annotations",
			body: `{"id":"chat_web","choices":[{"finish_reason":"stop","message":{"content":"answer","annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/weather"}}]}}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := openAIToClaudeResponseWithError([]byte(tt.body), "client-model")
			if err != nil {
				t.Fatal(err)
			}
			converted := decodeTestObject(t, body)
			usage := testObject(t, converted["usage"], "usage")
			serverToolUse := testObject(t, usage["server_tool_use"], "usage.server_tool_use")
			requireTestEqual(t, "web search request count", serverToolUse["web_search_requests"], float64(1))
			if tt.wantBlocks {
				seen := map[string]bool{}
				for _, raw := range testArray(t, converted["content"], "content") {
					seen[bridgeString(testObject(t, raw, "content block")["type"])] = true
				}
				if !seen["server_tool_use"] || !seen["web_search_tool_result"] {
					t.Fatalf("provider output search blocks missing: %s", body)
				}
			}
		})
	}
}

func TestChatResponseWithoutWebSearchEvidenceDoesNotInventUsage(t *testing.T) {
	body, err := openAIToClaudeResponseWithError([]byte(`{
		"id":"chat_plain","choices":[{"finish_reason":"stop","message":{"content":"plain answer"}}],
		"usage":{"prompt_tokens":2,"completion_tokens":3}
	}`), "client-model")
	if err != nil {
		t.Fatal(err)
	}
	converted := decodeTestObject(t, body)
	usage := testObject(t, converted["usage"], "usage")
	if _, exists := usage["server_tool_use"]; exists {
		t.Fatalf("plain Chat response invented server-tool usage: %s", body)
	}
}

func TestHostedWebSearchRequiredAndEvidenceDetection(t *testing.T) {
	if !requestRequiresAnthropicHostedWebSearch([]byte(`{
		"tools":[{"type":"web_search_20250305","name":"web_search"}],
		"tool_choice":{"type":"tool","name":"web_search"}
	}`)) {
		t.Fatal("Claude Code forced WebSearch request was not recognized")
	}
	if requestRequiresAnthropicHostedWebSearch([]byte(`{
		"tools":[{"type":"web_search_20250305","name":"web_search"}],
		"tool_choice":{"type":"auto"}
	}`)) {
		t.Fatal("optional Anthropic WebSearch request was treated as forced")
	}
	if !requestRequiresHostedWebSearch([]byte(`{
		"tools":[{"type":"web_search"}],"tool_choice":{"type":"web_search"}
	}`)) {
		t.Fatal("forced Responses WebSearch request was not recognized")
	}
	if requestRequiresHostedWebSearch([]byte(`{"tools":[{"type":"web_search"}]}`)) {
		t.Fatal("optional Responses WebSearch request was treated as forced")
	}

	evidence := []string{
		`{"output":[{"type":"web_search_call","id":"ws_1"}]}`,
		`{"content":[{"type":"server_tool_use","name":"web_search"}]}`,
		`{"choices":[{"message":{"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com"}}]}}]}`,
		strings.Join([]string{
			testNamedSSE("response.output_item.done", `{"type":"response.output_item.done","item":{"type":"web_search_call","id":"ws_1"}}`),
		}, ""),
	}
	for index, body := range evidence {
		if !responseContainsHostedWebSearchEvidence([]byte(body)) {
			t.Fatalf("evidence case %d was not recognized: %s", index, body)
		}
	}
	if responseContainsHostedWebSearchEvidence([]byte(`{
		"output":[{"type":"message","content":[{"type":"output_text","text":"plain","annotations":[]}]}]
	}`)) {
		t.Fatal("plain response was misclassified as executed web search")
	}
	if !responseRepresentsSuccessfulCompletion([]byte(`{"status":"completed","output":[]}`)) {
		t.Fatal("completed zero-search response was not recognized as a successful terminal response")
	}
	if responseRepresentsSuccessfulCompletion([]byte(`{"status":"failed","error":{"message":"boom"}}`)) {
		t.Fatal("failed response was treated as a successful zero-search response")
	}
	failedStream := strings.Join([]string{
		testNamedSSE("response.failed", `{"type":"response.failed","response":{"status":"failed","error":{"message":"boom"}}}`),
		`data: [DONE]` + "\n\n",
	}, "")
	if responseRepresentsSuccessfulCompletion([]byte(failedStream)) {
		t.Fatal("failed SSE stream was treated as a successful zero-search response")
	}
	if responseRepresentsSuccessfulCompletion([]byte(testNamedSSE("response.created", `{"type":"response.created","response":{"status":"in_progress"}}`))) {
		t.Fatal("premature SSE stream was treated as a successful zero-search response")
	}
}

func TestHostedWebSearchCapabilityCacheIsModelSpecific(t *testing.T) {
	resetHostedWebSearchCapabilityCache()
	t.Cleanup(resetHostedWebSearchCapabilityCache)
	upstream := &UpstreamConfig{APIType: UpstreamResponses, BaseURL: "https://example.test/v1"}
	cfg := WebSearchConfig{Enabled: true}
	markHostedWebSearchUnsupported(upstream, "model-a")
	if !shouldUseGatewayWebSearchFallback(upstream, cfg, "model-a") {
		t.Fatal("unsupported model did not use cached fallback")
	}
	if shouldUseGatewayWebSearchFallback(upstream, cfg, "model-b") {
		t.Fatal("unsupported capability leaked to a different model")
	}
	markHostedWebSearchSupported(upstream, "model-a")
	if shouldUseGatewayWebSearchFallback(upstream, cfg, "model-a") || !isHostedWebSearchKnownSupported(upstream, "model-a") {
		t.Fatal("supported capability did not replace the stale unsupported state")
	}
}

func TestAnthropicHandlerWebSearchFallbackStream(t *testing.T) {
	matrixIsolateRuntime(t)
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"title": "Stream source", "url": "https://example.com/stream", "content": "current"},
		}})
	}))
	defer searchServer.Close()
	calls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":{"message":"web_search is unsupported"}}`))
			return
		}
		if calls == 2 {
			_, _ = w.Write([]byte(`{"id":"chat-s1","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"srv_stream","type":"function","function":{"name":"llm2api_web_search","arguments":"{\"query\":\"stream source\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chat-s2","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Stream answer."}}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`))
	}))
	defer upstreamServer.Close()
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{
		Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom, BaseURL: searchServer.URL,
	})
	configMu.Unlock()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"matrix-public-model","max_tokens":128,"stream":true,
		"messages":[{"role":"user","content":"search"}],
		"tools":[{"type":"web_search_20250305","name":"web_search"}]
	}`))
	response := httptest.NewRecorder()
	claudeMessagesHandler(response, request)
	stream := response.Body.String()
	if response.Code != http.StatusOK || calls != 3 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, stream)
	}
	for _, expected := range []string{"event: message_start", `"type":"server_tool_use"`, `"type":"web_search_tool_result"`, `"web_search_requests":1`, "event: message_stop"} {
		if !strings.Contains(stream, expected) {
			t.Fatalf("stream missing %q:\n%s", expected, stream)
		}
	}
}

func TestAnthropicHandlerKeepsNativeHostedSearchPriority(t *testing.T) {
	matrixIsolateRuntime(t)
	upstreamServer, recorder := matrixMockUpstream(t, UpstreamAnthropic)
	matrixSelectUpstream(upstreamServer.URL, UpstreamAnthropic)
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{
		Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom, BaseURL: "https://search.invalid",
	})
	configMu.Unlock()
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"matrix-public-model","max_tokens":64,"messages":[{"role":"user","content":"search"}],
		"tools":[{"type":"web_search_20250305","name":"web_search"}]
	}`))
	response := httptest.NewRecorder()
	claudeMessagesHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	calls := recorder.snapshot()
	if len(calls) != 1 || calls[0].path != "/v1/messages" {
		t.Fatalf("native Anthropic calls=%#v", calls)
	}
	tools := testArray(t, calls[0].body["tools"], "native Anthropic tools")
	requireTestEqual(t, "native Anthropic search type", testObject(t, tools[0], "native tool")["type"], "web_search_20250305")
	if response.Header().Get("X-Llm2api-Bridge-Path") != string(BridgePathPassthrough) {
		t.Fatalf("bridge path=%q, want passthrough", response.Header().Get("X-Llm2api-Bridge-Path"))
	}
}

func TestAnthropicWebSearchHistoryReplaysToChat(t *testing.T) {
	messages := claudeToOpenAIMessages([]ClaudeMessage{{Role: "assistant", Content: []any{
		map[string]any{"type": "server_tool_use", "id": "srv_1", "name": "web_search", "input": map[string]any{"query": "weather"}},
		map[string]any{"type": "web_search_tool_result", "tool_use_id": "srv_1", "content": []any{
			map[string]any{"type": "web_search_result", "url": "https://example.com/weather", "title": "Weather"},
		}},
		map[string]any{"type": "text", "text": "Sunny."},
	}}}, nil)
	if len(messages) != 2 || len(messages[0].ToolCalls) != 1 || messages[0].ToolCalls[0].Function.Name != internalWebSearchToolName {
		t.Fatalf("Anthropic search history=%#v", messages)
	}
	if messages[1].Role != "tool" || messages[1].ToolCallID != "srv_1" || !strings.Contains(fmt.Sprint(messages[1].Content), "weather") {
		t.Fatalf("Anthropic search result history=%#v", messages)
	}
}

func TestBufferedAnthropicWebSearchStream(t *testing.T) {
	recorder := httptest.NewRecorder()
	body := []byte(`{
		"id":"msg_test","type":"message","role":"assistant","model":"glm","stop_reason":"end_turn","stop_sequence":null,
		"content":[
			{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"weather"}},
			{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[{"type":"web_search_result","url":"https://example.com","title":"Example"}]},
			{"type":"text","text":"Sunny."}
		],"usage":{"input_tokens":2,"output_tokens":3}
	}`)
	writeBufferedAnthropicStream(recorder, body)
	stream := recorder.Body.String()
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d headers=%#v", recorder.Code, recorder.Header())
	}
	for _, expected := range []string{"event: message_start", "event: content_block_start", `"type":"server_tool_use"`, `"type":"web_search_tool_result"`, "event: message_stop"} {
		if !strings.Contains(stream, expected) {
			t.Fatalf("Anthropic stream missing %q:\n%s", expected, stream)
		}
	}
}

func TestAnthropicBridgeKeepsNativeWebSearch(t *testing.T) {
	body := []byte(`{
		"model":"claude-test","input":"latest news",
		"tools":[{"type":"web_search","filters":{"allowed_domains":["example.com"]}}],
		"tool_choice":{"type":"web_search"}
	}`)
	converted, _, warnings, err := convertResponsesRequestToAnthropicDirect(body, "claude-test", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range warnings {
		if warning.Code == "unsupported_hosted_tool" || warning.Code == "tool_choice_downgraded" {
			t.Fatalf("native web search unexpectedly downgraded: %#v", warnings)
		}
	}
	request := decodeTestObject(t, converted)
	tools := testArray(t, request["tools"], "tools")
	tool := testObject(t, tools[0], "web search tool")
	requireTestEqual(t, "Anthropic native tool", tool["type"], "web_search_20250305")
	choice := testObject(t, request["tool_choice"], "tool choice")
	requireTestEqual(t, "Anthropic native tool choice", choice["name"], "web_search")
}

func TestBufferedResponsesStreamIncludesSearchAndCompletion(t *testing.T) {
	recorder := httptest.NewRecorder()
	body := []byte(`{
		"id":"resp_test","object":"response","status":"completed","model":"glm",
		"output":[
			{"id":"ws_1","type":"web_search_call","status":"completed","action":{"type":"search","query":"q","sources":[]}},
			{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.com"}]}]}
		],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
	}`)
	writeBufferedResponsesStream(recorder, body)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d headers=%#v", recorder.Code, recorder.Header())
	}
	stream := recorder.Body.String()
	for _, expected := range []string{"event: response.created", "event: response.output_item.added", "event: response.output_text.delta", "event: response.output_text.annotation.added", "event: response.completed", `"type":"web_search_call"`} {
		if !strings.Contains(stream, expected) {
			t.Fatalf("stream missing %q:\n%s", expected, stream)
		}
	}
}

func TestWebSearchConfigValidation(t *testing.T) {
	if err := validateWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", SearXNGMode: searxngModeCustom}); err == nil {
		t.Fatal("missing custom SearXNG base URL was accepted")
	}
	if err := validateWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", SearXNGMode: searxngModeAuto}); err != nil {
		t.Fatalf("automatic SearXNG mode without a fixed URL was rejected: %v", err)
	}
	if err := validateWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", SearXNGMode: searxngModeSelected}); err == nil {
		t.Fatal("selected SearXNG mode without a selected URL was accepted")
	}
	legacySearx := normalizeWebSearchConfig(WebSearchConfig{Provider: "searxng", BaseURL: "https://legacy.example"})
	if legacySearx.SearXNGMode != searxngModeCustom {
		t.Fatalf("legacy fixed SearXNG config mode=%q, want custom", legacySearx.SearXNGMode)
	}
	if err := validateWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "tavily", APIKey: "env:TAVILY_API_KEY"}); err != nil {
		t.Fatalf("valid Tavily config rejected: %v", err)
	}
	duckDuckGo := normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: webSearchProviderDuckDuckGo})
	if duckDuckGo.BaseURL != defaultDuckDuckGoEndpoint || duckDuckGo.FallbackProvider != webSearchFallbackNone {
		t.Fatalf("DuckDuckGo defaults not normalized: %#v", duckDuckGo)
	}
	automaticSearXNG := normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", SearXNGMode: searxngModeAuto})
	if automaticSearXNG.FallbackProvider != webSearchProviderDuckDuckGo {
		t.Fatalf("automatic SearXNG fallback=%q, want duckduckgo", automaticSearXNG.FallbackProvider)
	}
	resetHostedWebSearchCapabilityCache()
	probeUpstream := &UpstreamConfig{BaseURL: "https://example.test/v1", APIType: UpstreamResponses}
	if shouldUseGatewayWebSearchFallback(probeUpstream, WebSearchConfig{Enabled: true}) {
		t.Fatal("unknown upstream capability should try native search first")
	}
	markHostedWebSearchUnsupported(probeUpstream)
	if !shouldUseGatewayWebSearchFallback(probeUpstream, WebSearchConfig{Enabled: true}) {
		t.Fatal("detected unsupported search capability was not cached")
	}
	if shouldUseGatewayWebSearchFallback(probeUpstream, WebSearchConfig{}) {
		t.Fatal("disabled global search unexpectedly selected fallback")
	}
	if !isHostedWebSearchUnsupportedResponse(http.StatusBadRequest, []byte(`{"error":{"message":"unsupported web_search tool"}}`)) {
		t.Fatal("unsupported native search response was not detected")
	}
	if isHostedWebSearchUnsupportedResponse(http.StatusTooManyRequests, []byte(`{"error":{"message":"web_search rate limited"}}`)) {
		t.Fatal("rate limiting was misclassified as unsupported search")
	}
	var legacy AppConfig
	if err := json.Unmarshal([]byte(`{"upstreams":{"chat":{"base_url":"https://example.test/v1","api_type":"openai","hosted_web_search":"fallback"}}}`), &legacy); err != nil {
		t.Fatalf("legacy hosted_web_search config no longer loads: %v", err)
	}
	if err := validateConfig(&legacy); err != nil {
		t.Fatalf("legacy hosted_web_search config was rejected: %v", err)
	}
	legacyJSON, _ := json.Marshal(legacy)
	if bytes.Contains(legacyJSON, []byte("hosted_web_search")) {
		t.Fatalf("deprecated hosted_web_search field was retained: %s", legacyJSON)
	}
	valid := AppConfig{
		WebSearch: WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: "https://search.example.test"},
		Upstreams: map[string]*UpstreamConfig{
			"responses": {BaseURL: "https://example.test/v1", APIType: UpstreamResponses},
		},
	}
	if err := validateConfig(&valid); err != nil {
		t.Fatalf("valid automatic fallback configuration rejected: %v", err)
	}
	normalizeConfig(&valid)
	if valid.WebSearch.MaxResults != defaultWebSearchMaxResults || valid.WebSearch.MaxToolRounds != defaultWebSearchMaxToolRounds {
		t.Fatalf("web search defaults not normalized: %#v", valid.WebSearch)
	}
}

func TestStrictModeDoesNotHideUnsupportedNativeWebSearch(t *testing.T) {
	matrixIsolateRuntime(t)
	searchCalls := 0
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		searchCalls++
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer searchServer.Close()
	upstreamCalls := 0
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unsupported web_search_options"}}`))
	}))
	defer upstreamServer.Close()
	matrixSelectUpstream(upstreamServer.URL, UpstreamOpenAI)
	configMu.Lock()
	webSearchCfg = normalizeWebSearchConfig(WebSearchConfig{Enabled: true, Provider: "searxng", BaseURL: searchServer.URL})
	upstreamCfgs["matrix"].BridgeMode = BridgeModeStrict
	upstreamCfg = cloneUpstreamConfig(upstreamCfgs["matrix"])
	configMu.Unlock()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"matrix-public-model","input":"latest","tools":[{"type":"web_search"}]
	}`))
	responsesHandler(response, request)
	if response.Code != http.StatusBadRequest || upstreamCalls != 1 || searchCalls != 0 {
		t.Fatalf("status=%d upstream_calls=%d search_calls=%d body=%s", response.Code, upstreamCalls, searchCalls, response.Body.String())
	}
}
