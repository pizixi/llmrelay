package gateway

import (
	"context"
	"io"
	"net/http"

	"llmrelay/backend/internal/bridge"
	"llmrelay/backend/internal/bridge/convert"
	bridgestream "llmrelay/backend/internal/bridge/stream"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/upstream"
	"llmrelay/backend/internal/websearch"
)

func withAnthropicProtocolHeaders(ctx context.Context, headers http.Header) context.Context {
	return upstream.WithAnthropicProtocolHeaders(ctx, headers)
}

func withNativeProtocolHeaders(ctx context.Context, headers http.Header) context.Context {
	return upstream.WithNativeProtocolHeaders(ctx, headers)
}

func prepareAnthropicPassthroughBodyWithReasoning(body []byte, model string, forward bool) ([]byte, error) {
	return upstream.PrepareAnthropicPassthroughBodyWithReasoning(body, model, forward)
}

func prepareResponsesPassthroughBodyWithEffort(body []byte, model string, effortMap map[string]string, forward bool) ([]byte, error) {
	return upstream.PrepareResponsesPassthroughBodyWithEffort(body, model, effortMap, forward)
}

func prepareChatPassthroughBody(body []byte, model, effort string, forward bool) ([]byte, error) {
	return upstream.PrepareChatPassthroughBody(body, model, effort, forward)
}

func callPreparedUpstream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig, raw ...bool) ([]byte, int, http.Header, error) {
	return upstream.CallPreparedUpstream(ctx, body, upstreamName, model, target, raw...)
}

func callUpstream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig, raw ...bool) ([]byte, int, http.Header, error) {
	return upstream.CallUpstream(ctx, body, upstreamName, model, target, raw...)
}

func callPreparedUpstreamStream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig, raw ...bool) (io.ReadCloser, int, http.Header, error) {
	return upstream.CallPreparedUpstreamStream(ctx, body, upstreamName, model, target, raw...)
}

func callUpstreamStream(ctx context.Context, body []byte, upstreamName, model string, target *UpstreamConfig) (io.ReadCloser, int, http.Header, error) {
	return upstream.CallUpstreamStream(ctx, body, upstreamName, model, target)
}

func copyFilteredResponseHeaders(dst, src http.Header) {
	upstream.CopyFilteredResponseHeaders(dst, src)
}

func applyUpstreamErrorHeaders(w http.ResponseWriter, headers http.Header, status int) int {
	return upstream.ApplyUpstreamErrorHeaders(w, headers, status)
}

func mapUpstreamErrorBody(body []byte, upstreamType UpstreamType) []byte {
	return upstream.MapUpstreamErrorBody(body, upstreamType)
}

func mapErrorBodyToClaude(body []byte, fallback string) []byte {
	return upstream.MapErrorBodyToClaude(body, fallback)
}

func normalizeUpstreamStatus(status int) int { return upstream.NormalizeUpstreamStatus(status) }

func proxyChatPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string, record ...bool) error {
	return upstream.ProxyChatPassthroughStream(w, body, model, record...)
}

func proxyAnthropicPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	return upstream.ProxyAnthropicPassthroughStream(w, body, model)
}

func proxyResponsesPassthroughStream(w http.ResponseWriter, body io.ReadCloser, model string) error {
	return upstream.ProxyResponsesPassthroughStream(w, body, model)
}

func setSSEHeaders(header http.Header) { sse.SetHeaders(header) }

func claudeToOpenAIMessages(messages []anthropic.Message, system any) []Message {
	return convert.ClaudeToOpenAIMessages(messages, system)
}

func claudeToOpenAIToolsDetailed(tools []ClaudeTool, fallback ...bool) ([]Tool, []BridgeWarning, error) {
	return convert.ClaudeToOpenAIToolsDetailed(tools, fallback...)
}

func claudeToolChoiceToOpenAI(choice any) (any, *bool) {
	return convert.ClaudeToolChoiceToOpenAI(choice)
}

func reasoningEffortFromAnthropic(thinking, outputConfig any) string {
	return convert.ReasoningEffortFromAnthropic(thinking, outputConfig)
}

func fixToolCallGaps(messages []Message) []Message { return convert.FixToolCallGaps(messages) }

func normalizeMessagesToolCallArguments(messages []Message) ([]Message, error) {
	return convert.NormalizeMessagesToolCallArguments(messages)
}

func ensureReasoningEffort(request *OpenAIRequest, alias ModelAlias) {
	convert.EnsureReasoningEffort(request, alias)
}

func ensureReasoningContent(messages []Message, enabled bool) []Message {
	return convert.EnsureReasoningContent(messages, enabled)
}

func mapConfiguredReasoningEffort(effort string, maps ...map[string]string) string {
	return convert.MapConfiguredReasoningEffort(effort, maps...)
}

func buildUpstreamBody(request *OpenAIRequest, reasoning ...bool) []byte {
	return convert.BuildUpstreamBody(request, reasoning...)
}

func validateChatResponseForBridge(body []byte) error {
	return convert.ValidateChatResponseForBridge(body)
}

func convertResponse(body []byte) ([]byte, error) { return convert.ConvertResponse(body) }

func convertAnthropicRequestToResponsesDirect(body []byte, model string, reasoning bool, maps ...map[string]string) ([]byte, []BridgeWarning, error) {
	return convert.ConvertAnthropicRequestToResponsesDirect(body, model, reasoning, maps...)
}

func convertResponsesRequestToAnthropicDirect(body []byte, model string, reasoning bool, maps ...map[string]string) ([]byte, map[string]ResponseToolNameMapping, []BridgeWarning, error) {
	return convert.ConvertResponsesRequestToAnthropicDirect(body, model, reasoning, maps...)
}

func convertAnthropicResponseToResponsesDirect(body []byte, model string, request map[string]any, mappings map[string]ResponseToolNameMapping, warningGroups ...[]BridgeWarning) ([]byte, []BridgeWarning, error) {
	return convert.ConvertAnthropicResponseToResponsesDirect(body, model, request, mappings, warningGroups...)
}

func convertResponsesResponseToAnthropicDirect(body []byte, model string, mappings map[string]ResponseToolNameMapping) ([]byte, []BridgeWarning, error) {
	return convert.ConvertResponsesResponseToAnthropicDirect(body, model, mappings)
}

func openAIToClaudeResponseWithError(body []byte, model string) ([]byte, error) {
	return convert.OpenAIToClaudeResponseWithError(body, model)
}

func responsesInputToMessagesWithWarnings(input any, instructions string, mappings map[string]ResponseToolNameMapping) ([]Message, []BridgeWarning) {
	return convert.ResponsesInputToMessagesWithWarnings(input, instructions, mappings)
}

func responsesLoadedToolDefinitions(input any) []ResponsesTool {
	return convert.ResponsesLoadedToolDefinitions(input)
}

func convertResponsesToolsWithMappingsDetailed(tools []ResponsesTool, fallback ...bool) ([]Tool, map[string]ResponseToolNameMapping, []BridgeWarning) {
	return convert.ConvertResponsesToolsWithMappingsDetailed(tools, fallback...)
}

func convertResponsesToolChoiceDetailed(choice any, mappings map[string]ResponseToolNameMapping, hasTools bool) (any, []BridgeWarning) {
	return convert.ConvertResponsesToolChoiceDetailed(choice, mappings, hasTools)
}

func responsesBridgeRequestWarningsForUpstream(fields map[string]any, target *UpstreamConfig) []BridgeWarning {
	return convert.ResponsesBridgeRequestWarningsForUpstream(fields, target)
}

func responsesTextToChatResponseFormat(value any) (map[string]any, []BridgeWarning) {
	return convert.ResponsesTextToChatResponseFormat(value)
}

func downgradeRejectedChatOptions(body []byte, status int) ([]byte, []BridgeWarning, bool) {
	return convert.DowngradeRejectedChatOptions(body, status)
}

func responsesRequestEchoFields(fields map[string]any) map[string]any {
	return convert.ResponsesRequestEchoFields(fields)
}

func convertChatToResponsesForRequestWithError(chatBody []byte, model string, requestBody []byte, mappings map[string]ResponseToolNameMapping, warningGroups ...[]BridgeWarning) ([]byte, error) {
	return convert.ConvertChatToResponsesForRequestWithError(chatBody, model, requestBody, mappings, warningGroups...)
}

func openAIServiceTierFromAnthropic(value any) (string, bool) {
	return bridge.OpenAIServiceTierFromAnthropic(value)
}

func prependBridgeGuidance(messages []Message, guidance []string) []Message {
	return bridge.PrependGuidance(messages, guidance)
}

func anthropicServerToolGuidance(tools []ClaudeTool) []string {
	return bridge.AnthropicServerToolGuidance(tools)
}

func responsesHostedToolGuidance(tools []ResponsesTool) []string {
	return bridge.ResponsesHostedToolGuidance(tools)
}

type clientStreamOptions = bridgestream.ClientStreamOptions

func dispatchClientStream(w http.ResponseWriter, client WireProtocol, decision ProtocolDecision, target *UpstreamConfig, body io.ReadCloser, model string, options clientStreamOptions) {
	bridgestream.DispatchClientStream(w, client, decision, target, body, model, options)
}

func requestContainsAnthropicHostedWebSearch(body []byte) bool {
	return websearch.RequestContainsAnthropicHostedWebSearch(body)
}

func requestRequiresAnthropicHostedWebSearch(body []byte) bool {
	return websearch.RequestRequiresAnthropicHostedWebSearch(body)
}

func containsAnthropicHostedWebSearch(tools []ClaudeTool) bool {
	return websearch.ContainsAnthropicHostedWebSearch(tools)
}

func requestContainsHostedWebSearch(body []byte) bool {
	return websearch.RequestContainsHostedWebSearch(body)
}

func requestRequiresHostedWebSearch(body []byte) bool {
	return websearch.RequestRequiresHostedWebSearch(body)
}

func containsHostedWebSearch(tools []ResponsesTool) bool {
	return websearch.ContainsHostedWebSearch(tools)
}

func shouldUseGatewayWebSearchFallback(target *UpstreamConfig, config WebSearchConfig, model ...string) bool {
	return websearch.ShouldUseGatewayWebSearchFallback(target, config, model...)
}

func markHostedWebSearchUnsupported(target *UpstreamConfig, model ...string) {
	websearch.MarkHostedWebSearchUnsupported(target, model...)
}

func markHostedWebSearchSupported(target *UpstreamConfig, model ...string) {
	websearch.MarkHostedWebSearchSupported(target, model...)
}

func isHostedWebSearchKnownSupported(target *UpstreamConfig, model ...string) bool {
	return websearch.IsHostedWebSearchKnownSupported(target, model...)
}

func isHostedWebSearchUnsupportedResponse(status int, body []byte) bool {
	return websearch.IsHostedWebSearchUnsupportedResponse(status, body)
}

func responseContainsHostedWebSearchEvidence(body []byte) bool {
	return websearch.ResponseContainsHostedWebSearchEvidence(body)
}

func responseRepresentsSuccessfulCompletion(body []byte) bool {
	return websearch.ResponseRepresentsSuccessfulCompletion(body)
}

func bufferHostedWebSearchProbeStream(body io.ReadCloser) (io.ReadCloser, []byte, error) {
	return websearch.BufferHostedWebSearchProbeStream(body)
}

func executeGatewayWebSearchLoop(ctx context.Context, request OpenAIRequest, reasoning bool, upstreamName string, target *UpstreamConfig, mappings map[string]ResponseToolNameMapping, config WebSearchConfig) websearch.LoopResult {
	return websearch.ExecuteGatewayWebSearchLoop(ctx, request, reasoning, upstreamName, target, mappings, config)
}

func injectAnthropicWebSearchMetadata(body []byte, traces []websearch.Trace) ([]byte, error) {
	return websearch.InjectAnthropicWebSearchMetadata(body, traces)
}

func writeBufferedAnthropicStream(w http.ResponseWriter, body []byte) {
	websearch.WriteBufferedAnthropicStream(w, body)
}

func writeBufferedResponsesStream(w http.ResponseWriter, body []byte) {
	websearch.WriteBufferedResponsesStream(w, body)
}

func filterAnthropicNativeChatWebSearchWarnings(warnings []BridgeWarning, tools []ClaudeTool) []BridgeWarning {
	return websearch.FilterAnthropicNativeChatWebSearchWarnings(warnings, tools)
}

func anthropicWebSearchOptionsForChat(tools []ClaudeTool) map[string]any {
	return websearch.AnthropicWebSearchOptionsForChat(tools)
}

func rewriteAnthropicWebSearchToolChoice(choice any) any {
	return websearch.RewriteAnthropicWebSearchToolChoice(choice)
}

func filterNativeHostedWebSearchWarnings(warnings []BridgeWarning, tools []ResponsesTool) []BridgeWarning {
	return websearch.FilterNativeHostedWebSearchWarnings(warnings, tools)
}

func responsesWebSearchToChatWarnings(tools []ResponsesTool) []BridgeWarning {
	return websearch.ResponsesWebSearchToChatWarnings(tools)
}

func responsesWebSearchOptionsForChat(tools []ResponsesTool) map[string]any {
	return websearch.ResponsesWebSearchOptionsForChat(tools)
}

var _ = chat.InternalAnthropicRequestKey
