package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func ClaudeMessagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	body, readStatus, err := readAPIRequestBody(w, r)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, readStatus, "invalid_request_error", err.Error())
		return
	}

	cnt := requestCount.Add(1)
	if debugMode && debugLogBodies {
		log.Printf("[请求 #%d] POST /v1/messages\n%s", cnt, string(body))
	}

	envelope, err := DecodeNativeRequestEnvelope(body)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}
	requestedModel := strings.TrimSpace(envelope.Model)
	r, releaseModelRequest := trackModelRequest(r, requestedModel)
	defer releaseModelRequest()
	upstreamContext := withAnthropicProtocolHeaders(r.Context(), r.Header)
	resolvedModel, modelAliasInfo, upstreamName, upstream, aliasMatched, modelMatched := resolveRequestModel(envelope.Model)
	if resolvedModel == "" {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if !modelMatched {
		writeModelNotFoundError(w, r.URL.Path, requestedModel)
		return
	}
	if upstream == nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusInternalServerError, "upstream_configuration_error", "selected upstream is not configured: "+upstreamName)
		return
	}
	decision := decideProtocolBridge(WireAnthropic, upstream, effectiveBridgeMode(r, upstream))
	effortMap := getReasoningEffortMapForAlias(modelAliasInfo)
	forwardReasoning := shouldForwardReasoningParameters(modelAliasInfo, aliasMatched)
	if resolvedModel != requestedModel || forwardReasoning {
		decision.MarkPatched()
	}
	decision.EvaluateCapabilities(upstream, requestCapabilities(body, WireAnthropic)...)
	if decision.Mode == BridgeModeStrict {
		capabilityWarnings := capabilityBridgeWarnings(decision)
		if decision.Path != BridgePathPassthrough {
			capabilityWarnings = explicitCapabilityBridgeWarnings(decision)
		}
		if rejectStrictBridgeWarnings(w, r, capabilityWarnings) {
			return
		}
	}
	webSearchConfig := getWebSearchConfig()
	hasHostedWebSearch := requestContainsAnthropicHostedWebSearch(body)
	requiresHostedWebSearch := hasHostedWebSearch && requestRequiresAnthropicHostedWebSearch(body)
	allowAutomaticWebSearchFallback := webSearchConfig.Enabled && decision.Mode != BridgeModeStrict
	negotiateHostedWebSearch := hasHostedWebSearch && allowAutomaticWebSearchFallback
	forceWebSearchFallback := allowAutomaticWebSearchFallback && shouldUseGatewayWebSearchFallback(upstream, webSearchConfig, resolvedModel) && hasHostedWebSearch
	if forceWebSearchFallback && decision.Path == BridgePathPassthrough {
		decision.UsePivot()
	}
	if decision.Path == BridgePathPassthrough && !forceWebSearchFallback && !negotiateHostedWebSearch {
		applyDecisionHeaders(w.Header(), decision, nil)
		nativeBody, prepareErr := prepareAnthropicPassthroughBodyWithReasoning(body, resolvedModel, forwardReasoning)
		if prepareErr != nil {
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", prepareErr.Error())
			return
		}
		ctx := withNativeProtocolHeaders(upstreamContext, r.Header)
		ServeNativeProtocol(w, NativeProxyRequest{
			Client: WireAnthropic, Body: nativeBody, UpstreamName: upstreamName,
			Model: resolvedModel, UsageModel: requestedModel, Stream: envelope.Stream,
			Upstream: upstream, RequestContext: ctx,
		})
		return
	}

	var claudeReq ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid Anthropic request")
		return
	}
	claudeReq.Model = resolvedModel
	useWebSearchFallback := allowAutomaticWebSearchFallback && shouldUseGatewayWebSearchFallback(upstream, webSearchConfig, claudeReq.Model) && containsAnthropicHostedWebSearch(claudeReq.Tools)
	nativeSearchProbe := negotiateHostedWebSearch && !useWebSearchFallback && decision.Path == BridgePathPassthrough
	useNativeChatWebSearch := upstream.APIType == UpstreamOpenAI && !useWebSearchFallback && containsAnthropicHostedWebSearch(claudeReq.Tools)
	var originalClaudeRequest map[string]any
	_ = json.Unmarshal(body, &originalClaudeRequest)

	// 多模态路由

	messages := claudeToOpenAIMessages(claudeReq.Messages, claudeReq.System)
	messages = fixToolCallGaps(messages)
	var toolArgsErr error
	messages, toolArgsErr = normalizeMessagesToolCallArguments(messages)
	if toolArgsErr != nil {
		log.Printf("[请求无效] path=/v1/messages model=%q 错误=%v", claudeReq.Model, toolArgsErr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": toolArgsErr.Error()}})
		return
	}

	chatReq := OpenAIRequest{
		Model:    claudeReq.Model,
		Messages: messages,
		Stream:   claudeReq.Stream,
		Thinking: claudeReq.Thinking,
		AdditionalFields: map[string]any{
			internalAnthropicRequestKey: originalClaudeRequest,
		},
		ConfiguredReasoningEffortMap: effortMap,
	}
	if forwardReasoning {
		if effort := reasoningEffortFromAnthropic(claudeReq.Thinking, claudeReq.OutputConfig); effort != "" && effort != "none" {
			chatReq.ReasoningEffort = effort
		}
	}
	if claudeReq.MaxTokens > 0 {
		chatReq.MaxTokens = claudeReq.MaxTokens
	}
	if claudeReq.Temperature != nil {
		chatReq.Temperature = claudeReq.Temperature
	}
	if claudeReq.TopP != nil {
		chatReq.TopP = claudeReq.TopP
	}
	if len(claudeReq.StopSequences) > 0 {
		chatReq.AdditionalFields["stop"] = append([]string(nil), claudeReq.StopSequences...)
	}
	if claudeReq.Metadata != nil {
		chatReq.AdditionalFields["metadata"] = claudeReq.Metadata
	}
	var bridgeWarnings []BridgeWarning
	if claudeReq.ServiceTier != nil {
		if mapped, recognized := openAIServiceTierFromAnthropic(claudeReq.ServiceTier); recognized {
			chatReq.AdditionalFields["service_tier"] = mapped
		} else if upstream.APIType != UpstreamAnthropic {
			bridgeWarnings = appendBridgeWarning(bridgeWarnings, BridgeWarning{
				Code: "request_field_ignored", Path: "service_tier",
				Message: fmt.Sprintf("Anthropic service_tier %q has no OpenAI equivalent and was ignored", bridgeString(claudeReq.ServiceTier)),
			})
		}
	}
	if len(claudeReq.Tools) > 0 {
		convertedTools, toolWarnings, convertErr := claudeToOpenAIToolsDetailed(claudeReq.Tools, useWebSearchFallback)
		if convertErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"type":  "error",
				"error": map[string]any{"type": "invalid_request_error", "message": convertErr.Error()},
			})
			return
		}
		if useNativeChatWebSearch {
			toolWarnings = filterAnthropicNativeChatWebSearchWarnings(toolWarnings, claudeReq.Tools)
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, toolWarnings)
		} else if useWebSearchFallback {
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, toolWarnings)
		} else if upstream.APIType != UpstreamAnthropic && decision.Upstream != WireResponses {
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, toolWarnings)
			chatReq.Messages = prependBridgeGuidance(chatReq.Messages, anthropicServerToolGuidance(claudeReq.Tools))
		}
		if len(convertedTools) > 0 {
			chatReq.Tools = convertedTools
			var parallelToolCalls *bool
			chatReq.ToolChoice, parallelToolCalls = claudeToolChoiceToOpenAI(claudeReq.ToolChoice)
			if useWebSearchFallback {
				chatReq.ToolChoice = rewriteAnthropicWebSearchToolChoice(chatReq.ToolChoice)
			}
			if chatReq.ToolChoice == nil {
				chatReq.ToolChoice = "auto"
			}
			if parallelToolCalls != nil {
				chatReq.AdditionalFields["parallel_tool_calls"] = *parallelToolCalls
			}
		}
	}
	if useNativeChatWebSearch {
		if options := anthropicWebSearchOptionsForChat(claudeReq.Tools); options != nil {
			chatReq.AdditionalFields["web_search_options"] = options
		}
	}
	var pairwiseUpstreamBody []byte
	if decision.Upstream == WireResponses && !useWebSearchFallback {
		var pairwiseWarnings []BridgeWarning
		pairwiseUpstreamBody, pairwiseWarnings, err = convertAnthropicRequestToResponsesDirect(body, claudeReq.Model, forwardReasoning, effortMap)
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, pairwiseWarnings)
		if err != nil {
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	strictBridge := upstream.APIType != UpstreamAnthropic && effectiveBridgeMode(r, upstream) == BridgeModeStrict
	if decision.Path != BridgePathPassthrough {
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, conversionCapabilityBridgeWarnings(decision, hasHostedWebSearch))
	}
	if strictBridge && rejectStrictBridgeWarnings(w, r, bridgeWarnings) {
		return
	}

	ensureReasoningEffort(&chatReq, modelAliasInfo)
	chatReq.Messages = ensureReasoningContent(chatReq.Messages, modelAliasInfo.WithReasoning)
	applyDecisionHeaders(w.Header(), decision, bridgeWarnings)
	logBridgeWarnings("anthropic", upstreamName, chatReq.Model, bridgeWarnings)

	upstreamBody := pairwiseUpstreamBody
	if len(upstreamBody) == 0 {
		upstreamBody = buildUpstreamBody(&chatReq, forwardReasoning)
	}
	if nativeSearchProbe {
		upstreamBody, err = prepareAnthropicPassthroughBodyWithReasoning(body, claudeReq.Model, forwardReasoning)
		if err != nil {
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	activateWebSearchFallback := func() {
		if useWebSearchFallback {
			return
		}
		useWebSearchFallback = true
		decision.UsePivot()
		fallbackTools, fallbackWarnings, convertErr := claudeToOpenAIToolsDetailed(claudeReq.Tools, true)
		if convertErr != nil {
			fallbackWarnings = appendBridgeWarning(fallbackWarnings, BridgeWarning{
				Code: "hosted_web_search_fallback_failed", Path: "tools",
				Message: convertErr.Error(),
			})
		} else {
			chatReq.Tools = fallbackTools
			delete(chatReq.AdditionalFields, "web_search_options")
			var parallelToolCalls *bool
			chatReq.ToolChoice, parallelToolCalls = claudeToolChoiceToOpenAI(claudeReq.ToolChoice)
			chatReq.ToolChoice = rewriteAnthropicWebSearchToolChoice(chatReq.ToolChoice)
			if chatReq.ToolChoice == nil {
				chatReq.ToolChoice = "auto"
			}
			if parallelToolCalls != nil {
				chatReq.AdditionalFields["parallel_tool_calls"] = *parallelToolCalls
			}
		}
		delete(chatReq.AdditionalFields, internalAnthropicRequestKey)
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, fallbackWarnings)
		applyDecisionHeaders(w.Header(), decision, bridgeWarnings)
		logBridgeWarnings("anthropic", upstreamName, chatReq.Model, fallbackWarnings)
	}

	serveWebSearchFallback := func() {
		// Anthropic 兼容上游的自动降级必须使用转换后的 Chat 请求，
		// 不能使用保留的原生请求体。
		delete(chatReq.AdditionalFields, internalAnthropicRequestKey)
		loopResult := executeGatewayWebSearchLoop(
			upstreamContext, chatReq, forwardReasoning, upstreamName, upstream, nil, webSearchConfig,
		)
		if loopResult.Err != nil || loopResult.Status < 200 || loopResult.Status >= 300 {
			w.Header().Set("Content-Type", "application/json")
			status := applyUpstreamErrorHeaders(w, loopResult.Header, loopResult.Status)
			w.WriteHeader(status)
			w.Write(mapErrorBodyToClaude(loopResult.Body, "web search fallback upstream error"))
			return
		}
		claudeRespBody, convertErr := openAIToClaudeResponseWithError(loopResult.Body, claudeReq.Model)
		if convertErr == nil {
			claudeRespBody, convertErr = injectAnthropicWebSearchMetadata(claudeRespBody, loopResult.Traces)
		}
		if convertErr != nil {
			log.Printf("[响应转换失败] path=/v1/messages model=%q 错误=%v", claudeReq.Model, convertErr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write(mapErrorBodyToClaude(nil, "web search fallback returned an invalid Chat response"))
			return
		}
		copyFilteredResponseHeaders(w.Header(), loopResult.Header)
		usageStats := newRequestUsageAccumulatorForContext(r.Context(), requestedModel, upstreamName, claudeReq.Model)
		var usageResponse map[string]any
		if json.Unmarshal(loopResult.Body, &usageResponse) == nil {
			if usage, ok := usageResponse["usage"].(map[string]any); ok {
				usageStats.observeMap(usage)
			}
		}
		usageStats.commit()
		if claudeReq.Stream {
			writeBufferedAnthropicStream(w, claudeRespBody)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(claudeRespBody)
	}
	if useWebSearchFallback {
		serveWebSearchFallback()
		return
	}

	if claudeReq.Stream {
		var upResp io.ReadCloser
		var status int
		var upHeader http.Header
		if nativeSearchProbe {
			upResp, status, upHeader, err = callPreparedUpstreamStream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream, true)
		} else if decision.Upstream == WireResponses {
			upResp, status, upHeader, err = callPreparedUpstreamStream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream)
		} else {
			upResp, status, upHeader, err = callUpstreamStream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream)
		}
		if err != nil || status < 200 || status >= 300 {
			var errBody []byte
			if upResp != nil {
				errBody, _ = io.ReadAll(upResp)
				upResp.Close()
			}
			if hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, errBody) {
				markHostedWebSearchUnsupported(upstream, chatReq.Model)
				if webSearchConfig.Enabled && decision.Mode != BridgeModeStrict {
					activateWebSearchFallback()
					serveWebSearchFallback()
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			status = applyUpstreamErrorHeaders(w, upHeader, status)
			w.WriteHeader(status)
			w.Write(mapErrorBodyToClaude(errBody, "upstream error"))
			return
		}
		if requiresHostedWebSearch && allowAutomaticWebSearchFallback && !isHostedWebSearchKnownSupported(upstream, chatReq.Model) {
			var probeBody []byte
			upResp, probeBody, err = bufferHostedWebSearchProbeStream(upResp)
			if err != nil {
				writeClientAPIError(w, WireAnthropic, http.StatusBadGateway, "upstream_protocol_error", "failed to inspect hosted web search stream")
				return
			}
			if responseContainsHostedWebSearchEvidence(probeBody) {
				markHostedWebSearchSupported(upstream, chatReq.Model)
			} else if responseRepresentsSuccessfulCompletion(probeBody) {
				markHostedWebSearchUnsupported(upstream, chatReq.Model)
				log.Printf("[Hosted Web Search 自动回退] 上游=%s 模型=%s 原因=成功流式响应未执行强制搜索", effectiveUpstreamName(upstreamName), chatReq.Model)
				activateWebSearchFallback()
				serveWebSearchFallback()
				return
			}
		}
		defer upResp.Close()
		copyFilteredResponseHeaders(w.Header(), upHeader)
		dispatchClientStream(w, WireAnthropic, decision, upstream, upResp, claudeReq.Model, clientStreamOptions{UsageModel: requestedModel, UpstreamName: upstreamName, UpstreamModel: claudeReq.Model, RequestContext: r.Context()})
		return
	}

	rawAnthropicResponse := upstream != nil && upstream.APIType == UpstreamAnthropic
	pairwiseResponse := decision.Upstream == WireResponses
	var respBody []byte
	var status int
	var upHeader http.Header
	if nativeSearchProbe {
		respBody, status, upHeader, err = callPreparedUpstream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream, true)
	} else if pairwiseResponse {
		respBody, status, upHeader, err = callPreparedUpstream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream, true)
	} else {
		respBody, status, upHeader, err = callUpstream(upstreamContext, upstreamBody, upstreamName, chatReq.Model, upstream, rawAnthropicResponse)
	}
	if (err != nil || status < 200 || status >= 300) && hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, respBody) {
		markHostedWebSearchUnsupported(upstream, chatReq.Model)
		if webSearchConfig.Enabled && decision.Mode != BridgeModeStrict {
			activateWebSearchFallback()
			serveWebSearchFallback()
			return
		}
	}
	if err != nil || status < 200 || status >= 300 {
		w.Header().Set("Content-Type", "application/json")
		status = applyUpstreamErrorHeaders(w, upHeader, status)
		w.WriteHeader(status)
		w.Write(mapErrorBodyToClaude(respBody, "upstream error"))
		return
	}
	if requiresHostedWebSearch && webSearchConfig.Enabled {
		if responseContainsHostedWebSearchEvidence(respBody) {
			markHostedWebSearchSupported(upstream, chatReq.Model)
		} else if responseRepresentsSuccessfulCompletion(respBody) {
			markHostedWebSearchUnsupported(upstream, chatReq.Model)
			if decision.Mode != BridgeModeStrict {
				log.Printf("[Hosted Web Search 自动回退] 上游=%s 模型=%s 原因=成功响应未执行强制搜索", effectiveUpstreamName(upstreamName), chatReq.Model)
				activateWebSearchFallback()
				serveWebSearchFallback()
				return
			}
		}
	}

	claudeRespBody := respBody
	if pairwiseResponse {
		var responseWarnings []BridgeWarning
		claudeRespBody, responseWarnings, err = convertResponsesResponseToAnthropicDirect(respBody, claudeReq.Model, nil)
		if len(responseWarnings) > 0 {
			if decision.Mode == BridgeModeStrict {
				writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", bridgeWarningsError(responseWarnings).Error())
				return
			}
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, responseWarnings)
			writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
			logBridgeWarnings("anthropic", upstreamName, chatReq.Model, responseWarnings)
		}
		if err != nil {
			log.Printf("[响应转换失败] path=/v1/messages model=%q 错误=%v", claudeReq.Model, err)
			writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", "upstream returned an invalid Responses response")
			return
		}
	} else if !rawAnthropicResponse {
		claudeRespBody, err = openAIToClaudeResponseWithError(respBody, claudeReq.Model)
		if err != nil {
			log.Printf("[响应转换失败] path=/v1/messages model=%q 错误=%v", claudeReq.Model, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			w.Write(mapErrorBodyToClaude(nil, "upstream returned an invalid Chat response"))
			return
		}
	}

	usageStats := newRequestUsageAccumulatorForContext(r.Context(), requestedModel, upstreamName, claudeReq.Model)
	var usageResp2 map[string]any
	if json.Unmarshal(respBody, &usageResp2) == nil {
		if u, ok := usageResp2["usage"].(map[string]any); ok {
			usageStats.observeMap(u)
		}
	}
	usageStats.commit()

	copyFilteredResponseHeaders(w.Header(), upHeader)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if debugMode && debugLogBodies {
		log.Printf("[客户端响应]\n%s", string(claudeRespBody))
	}
	w.Write(claudeRespBody)
}
