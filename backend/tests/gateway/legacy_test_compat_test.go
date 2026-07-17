package gateway

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/bridge"
	"llmrelay/backend/internal/bridge/convert"
	bridgestream "llmrelay/backend/internal/bridge/stream"
	"llmrelay/backend/internal/catalog"
	configpkg "llmrelay/backend/internal/config"
	"llmrelay/backend/internal/domain"
	gatewaypkg "llmrelay/backend/internal/gateway"
	adminapi "llmrelay/backend/internal/httpapi/admin"
	"llmrelay/backend/internal/netproxy"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/protocol/responses"
	"llmrelay/backend/internal/stats"
	"llmrelay/backend/internal/upstream"
	"llmrelay/backend/internal/websearch"
)

type OpenAIRequest = chat.Request
type Message = chat.Message
type ToolCall = chat.ToolCall
type FunctionCall = chat.FunctionCall
type Tool = chat.Tool
type ToolFunction = chat.ToolFunction
type ClaudeRequest = anthropic.Request
type ClaudeTool = anthropic.Tool
type ResponsesAPIRequest = responses.Request
type ResponsesTool = responses.Tool
type ResponseToolNameMapping = responses.ToolNameMapping
type UpstreamConfig = domain.UpstreamConfig
type UpstreamType = domain.UpstreamType
type ModelAlias = domain.ModelAlias
type ModelInfo = domain.ModelInfo
type WebSearchConfig = domain.WebSearchConfig
type WireProtocol = bridge.WireProtocol
type ProtocolDecision = bridge.ProtocolDecision
type BridgeWarning = bridge.BridgeWarning
type BridgeMode = domain.BridgeMode
type NativeProxyRequest = gatewaypkg.NativeProxyRequest

const (
	UpstreamOpenAI    = domain.UpstreamOpenAI
	UpstreamAnthropic = domain.UpstreamAnthropic
	UpstreamResponses = domain.UpstreamResponses

	WireChat      = bridge.WireChat
	WireAnthropic = bridge.WireAnthropic
	WireResponses = bridge.WireResponses

	BridgePathPassthrough = bridge.BridgePathPassthrough
	BridgePathPairwise    = bridge.BridgePathPairwise
	BridgePathPivot       = bridge.BridgePathPivot

	BridgeModeCompatible = domain.BridgeModeCompatible
	BridgeModeStrict     = domain.BridgeModeStrict

	internalAnthropicRequestKey = chat.InternalAnthropicRequestKey
)

var (
	ChatCompletionsHandler = gatewaypkg.ChatCompletionsHandler
	ClaudeMessagesHandler  = gatewaypkg.ClaudeMessagesHandler
	ResponsesHandler       = gatewaypkg.ResponsesHandler
	ListModelsHandler      = gatewaypkg.ListModelsHandler
	ServeNativeProtocol    = gatewaypkg.ServeNativeProtocol
)

type Socks5Proxy = domain.Socks5Proxy
type AppConfig = domain.AppConfig
type TokenStatsData = stats.TokenStatsData
type ModelStats = stats.ModelStats
type DailyStats = stats.DailyStats
type webSearchResult = websearch.Result
type SearXNGPublicInstance = websearch.SearXNGPublicInstance
type ClaudeMessage = anthropic.Message
type searxngSearchFailureSummary = websearch.SearxngSearchFailureSummary

type fallbackWebSearchProvider struct {
	primary       websearch.Provider
	fallback      websearch.Provider
	primaryBudget time.Duration
}

func (provider *fallbackWebSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	return websearch.NewFallbackProvider(provider.primary, provider.fallback, provider.primaryBudget).Search(ctx, query, maxResults)
}

type autoSearxngSearchProvider struct {
	directoryURL string
	fallbackURL  string
}

func (provider *autoSearxngSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	return websearch.NewAutoSearxngProvider(provider.directoryURL, provider.fallbackURL).Search(ctx, query, maxResults)
}

var duckDuckGoSearchRate = struct {
	sync.Mutex
	last time.Time
}{}

const (
	webSearchProviderDuckDuckGo   = "duckduckgo"
	searxngModeAuto               = "auto"
	searxngModeSelected           = "selected"
	searxngModeCustom             = "custom"
	internalWebSearchToolName     = "llm2api_web_search"
	defaultDuckDuckGoEndpoint     = "https://lite.duckduckgo.com/lite/"
	webSearchFallbackNone         = "none"
	defaultWebSearchMaxResults    = 6
	defaultWebSearchMaxToolRounds = 2
)

var errWebSearchRequestTimeout = websearch.ErrRequestTimeout

type BridgePath = bridge.BridgePath

type streamDispatchKind string

const (
	streamKindChatPassthrough      streamDispatchKind = "chat_passthrough"
	streamKindAnthropicPassthrough streamDispatchKind = "anthropic_passthrough"
	streamKindResponsesPassthrough streamDispatchKind = "responses_passthrough"
	streamKindAnthropicToChat      streamDispatchKind = "anthropic_to_chat"
	streamKindResponsesToChat      streamDispatchKind = "responses_to_chat"
	streamKindChatToAnthropic      streamDispatchKind = "chat_to_anthropic"
	streamKindResponsesToAnthropic streamDispatchKind = "responses_to_anthropic"
	streamKindChatToResponses      streamDispatchKind = "chat_to_responses"
	streamKindAnthropicToResponses streamDispatchKind = "anthropic_to_responses"
)

var (
	configMu            sync.RWMutex
	modelAlias          = map[string]ModelAlias{}
	upstreamCfg         *UpstreamConfig
	upstreamCfgs        = map[string]*UpstreamConfig{}
	defaultUpstreamName string
	reasoningEffortMap  = map[string]string{}
	webSearchCfg        WebSearchConfig
	apiAccessKey        string

	modelMu                    sync.RWMutex
	modelsCache                []ModelInfo
	upstreamModelCatalogCache  []ModelInfo
	modelsLoaded               bool
	upstreamModelCatalogLoaded bool

	socks5Proxies        []Socks5Proxy
	activeSocks5         string
	socks5Mu             sync.RWMutex
	socks5ClientCache    = map[socks5ClientCacheKey]*http.Client{}
	socks5RRIndex        uint32
	socks5RateLimitIndex uint32

	tokenStatsMu   sync.Mutex
	tokenStats     = &TokenStatsData{Models: map[string]*ModelStats{}}
	tokenStatsPath = "stats.json"
	statsDate      string
)

type socks5ClientCacheKey struct {
	Addr, Username, Password string
	Stream                   bool
}

const (
	socks5RR                      = netproxy.ModeRoundRobin
	socks5RateLimitSwitch         = netproxy.ModeRateLimitSwitch
	socks5RateLimitSwitchNoDirect = netproxy.ModeRateLimitSwitchNoDirect
)

func syncLegacyConfig() {
	configMu.RLock()
	value := AppConfig{
		ModelAlias:         modelAlias,
		ReasoningEffortMap: reasoningEffortMap,
		WebSearch:          webSearchCfg,
		Upstreams:          upstreamCfgs,
		DefaultUpstream:    defaultUpstreamName,
		Socks5Proxies:      socks5Proxies,
		ActiveSocks5:       activeSocks5,
	}
	configMu.RUnlock()
	configpkg.NormalizeConfig(&value)
	configpkg.ApplyConfig(value)
}

func chooseStreamDispatch(client WireProtocol, decision ProtocolDecision, target *UpstreamConfig) streamDispatchKind {
	return streamDispatchKind(bridge.ChooseStreamDispatch(client, decision, target))
}

func clientProtocolFromPath(path string) WireProtocol { return bridge.ClientProtocolFromPath(path) }

func wireProtocolFromUpstream(apiType UpstreamType) WireProtocol {
	return bridge.ProtocolFromUpstream(apiType)
}

func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	syncLegacyStats()
	ChatCompletionsHandler(w, r)
	loadLegacyStats()
}

func claudeMessagesHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	syncLegacyStats()
	ClaudeMessagesHandler(w, r)
	loadLegacyStats()
}

func responsesHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	syncLegacyStats()
	ResponsesHandler(w, r)
	loadLegacyStats()
}

func listModelsHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	ListModelsHandler(w, r)
}

func requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	auth.Configure("", apiAccessKey)
	return auth.RequireAPIAuth(next)
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	syncLegacyCatalog()
	adminapi.ReloadHandler(w, r)
	loadLegacyCatalog()
}

func adminTestModelHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	adminapi.AdminTestModelHandler(w, r)
}

func normalizeWebSearchConfig(value WebSearchConfig) WebSearchConfig {
	return websearch.NormalizeWebSearchConfig(value)
}

func validateWebSearchConfig(value WebSearchConfig) error {
	return websearch.ValidateWebSearchConfig(value)
}

func resetHostedWebSearchCapabilityCache() { websearch.ResetHostedWebSearchCapabilityCache() }

func getToday() string { return stats.GetToday() }

func getUpstreamEndpoint(target *UpstreamConfig) string { return upstream.GetUpstreamEndpoint(target) }

func getUpstreamModelsEndpoint(target *UpstreamConfig) string {
	return upstream.GetUpstreamModelsEndpoint(target)
}

func prepareResponsesPassthroughBody(body []byte, model string) ([]byte, error) {
	return upstream.PrepareResponsesPassthroughBody(body, model)
}

func prepareAnthropicPassthroughBody(body []byte, model string) ([]byte, error) {
	return upstream.PrepareAnthropicPassthroughBody(body, model)
}

type nativeProxyRequest = NativeProxyRequest

func serveNativeProtocol(w http.ResponseWriter, request nativeProxyRequest) {
	ServeNativeProtocol(w, request)
}

func webSearchWithTimeout(ctx context.Context, config WebSearchConfig, query string) (websearch.SearchResponse, error) {
	websearch.ResetDuckDuckGoRateLimit()
	return websearch.WebSearchWithTimeout(ctx, config, query)
}

func listSearXNGPublicInstances(ctx context.Context, directory string, refresh bool) ([]SearXNGPublicInstance, time.Time, error) {
	return websearch.ListSearXNGPublicInstances(ctx, directory, refresh)
}

func searxngInstancesHandler(w http.ResponseWriter, r *http.Request) {
	syncLegacyConfig()
	websearch.SearxngInstancesHandler(w, r)
}

func parseChatToolCalls(body []byte) (Message, []ToolCall, error) {
	return websearch.ParseChatToolCalls(body)
}

func openAIToAnthropicRequest(body []byte) []byte { return convert.OpenAIToAnthropicRequest(body) }

func anthropicContentToResponsesItems(role string, content any, path string) ([]any, []BridgeWarning) {
	return convert.AnthropicContentToResponsesItems(role, content, path)
}

func responsesWebSearchToAnthropicBlocks(item map[string]any) []any {
	return convert.ResponsesWebSearchToAnthropicBlocks(item)
}

func getFloat(values map[string]any, keys ...string) (float64, bool) {
	return convert.GetFloat(values, keys...)
}

func openAIToResponsesRequest(body []byte, target *UpstreamConfig) []byte {
	return convert.OpenAIToResponsesRequest(body, target)
}

func convertResponsesToChat(body []byte, model string) []byte {
	return convert.ConvertResponsesToChat(body, model)
}

func convertChatToResponsesForRequest(chatBody []byte, model string, requestBody []byte, mappings map[string]ResponseToolNameMapping, warnings ...[]BridgeWarning) []byte {
	return convert.ConvertChatToResponsesForRequest(chatBody, model, requestBody, mappings, warnings...)
}

func convertChatToResponsesObject(chatBody []byte, model string, tools, choice, parallel any, mappings map[string]ResponseToolNameMapping, warnings ...[]BridgeWarning) []byte {
	return convert.ConvertChatToResponsesObject(chatBody, model, tools, choice, parallel, mappings, warnings...)
}

func convertStreamChunkWithUsage(line string) (string, map[string]any) {
	return convert.ConvertStreamChunkWithUsage(line)
}

func cleanJsonSchema(value any) any { return convert.CleanJsonSchema(value) }

func validateResponsesBridgeRequest(fields map[string]any) error {
	return convert.ValidateResponsesBridgeRequest(fields)
}

func responsesBridgeRequestWarnings(fields map[string]any) []BridgeWarning {
	return convert.ResponsesBridgeRequestWarnings(fields)
}

func reasoningEffortToAnthropicThinking(effort string) map[string]any {
	return convert.ReasoningEffortToAnthropicThinking(effort)
}

func convertMessagesForUpstream(messages []Message, reasoning bool) []map[string]any {
	return convert.ConvertMessagesForUpstream(messages, reasoning)
}

func responseUpstreamToolName(kind, namespace, name string, mappings map[string]ResponseToolNameMapping) string {
	return convert.ResponseUpstreamToolName(kind, namespace, name, mappings)
}

func responseFunctionCallItem(itemID, status, arguments, callID, name string, mappings map[string]ResponseToolNameMapping) map[string]any {
	return convert.ResponseFunctionCallItem(itemID, status, arguments, callID, name, mappings)
}

func anthropicStreamToResponsesDirectHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, mode BridgeMode, tools, choice any, parallel *bool, mappings map[string]ResponseToolNameMapping, echo map[string]any, warnings ...[]BridgeWarning) {
	bridgestream.AnthropicStreamToResponsesDirectHandler(w, body, model, usageModel, mode, tools, choice, parallel, mappings, echo, warnings...)
}

func responsesStreamToAnthropicDirectHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string) {
	bridgestream.ResponsesStreamToAnthropicDirectHandler(w, body, model, usageModel)
}

func anthropicStreamToChatHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, record bool) {
	bridgestream.AnthropicStreamToChatHandler(w, body, model, usageModel, record)
}

func claudeStreamHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string) {
	bridgestream.ClaudeStreamHandler(w, body, model, usageModel)
}

func responsesStreamToChatHandler(w http.ResponseWriter, body io.ReadCloser, model, usageModel string, record bool) {
	upstream.ResponsesStreamToChatHandler(w, body, model, usageModel, record)
}

func responsesStreamHandler(w http.ResponseWriter, request *http.Request, response *http.Response, model, usageModel string, tools, choice any, parallel *bool, mappings map[string]ResponseToolNameMapping, echo map[string]any, warnings ...[]BridgeWarning) {
	bridgestream.ResponsesStreamHandler(w, request, response, model, usageModel, tools, choice, parallel, mappings, echo, warnings...)
}

func validateConfig(value *AppConfig) error { return configpkg.ValidateConfig(value) }

func normalizeConfig(value *AppConfig) { configpkg.NormalizeConfig(value) }

func loadConfig(path string) (AppConfig, error) { return configpkg.LoadConfig(path) }

func saveConfig(path string, value AppConfig) error { return configpkg.SaveConfig(path, value) }

func fetchModelsFromUpstream(name string, target *UpstreamConfig, custom bool) ([]ModelInfo, error) {
	return catalog.FetchModelsFromUpstream(name, target, custom)
}

func syncLegacyStats() {
	tokenStatsMu.Lock()
	data := *tokenStats
	date := statsDate
	path := tokenStatsPath
	tokenStatsMu.Unlock()
	stats.SetPath(path)
	stats.Restore(data, date)
}

func loadLegacyStats() {
	data := stats.Snapshot()
	tokenStatsMu.Lock()
	tokenStats = &data
	if data.Daily != nil {
		statsDate = data.Daily.Date
	}
	tokenStatsPath = stats.Path()
	tokenStatsMu.Unlock()
}

func saveTokenStats() {
	syncLegacyStats()
	stats.SaveTokenStats()
}

func scheduleTokenStatsSave() {
	syncLegacyStats()
	stats.ScheduleTokenStatsSave()
}

func currentSocks5ExitLabel() string { return netproxy.CurrentExitLabel() }

func socks5RateLimitAttemptCount() int { return netproxy.RateLimitAttemptCount() }

func cloneUpstreamConfig(target *UpstreamConfig) *UpstreamConfig {
	return configpkg.CloneUpstreamConfig(target)
}

func normalizeSingleUpstream(target *UpstreamConfig) bool {
	return configpkg.NormalizeSingleUpstream(target)
}

func closeSocks5ClientsLocked() {
	socks5ClientCache = map[socks5ClientCacheKey]*http.Client{}
}

func syncLegacyCatalog() {
	modelMu.RLock()
	state := catalog.State{
		Models:                append([]ModelInfo(nil), modelsCache...),
		UpstreamCatalog:       append([]ModelInfo(nil), upstreamModelCatalogCache...),
		ModelsLoaded:          modelsLoaded,
		UpstreamCatalogLoaded: upstreamModelCatalogLoaded,
	}
	modelMu.RUnlock()
	catalog.RestoreState(state)
}

func loadLegacyCatalog() {
	state := catalog.SnapshotState()
	modelMu.Lock()
	modelsCache = state.Models
	upstreamModelCatalogCache = state.UpstreamCatalog
	modelsLoaded = state.ModelsLoaded
	upstreamModelCatalogLoaded = state.UpstreamCatalogLoaded
	modelMu.Unlock()
}

func getHTTPClientWithExit(stream bool) (*http.Client, string) {
	return netproxy.ClientWithExit(stream)
}

var _ = catalog.GetModelIDs
