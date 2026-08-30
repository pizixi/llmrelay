package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	bridgestate "llmrelay/backend/internal/bridge/state"
)

func ResponsesHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("[请求 #%d] POST /v1/responses\n%s", cnt, string(body))
	}

	envelope, err := DecodeNativeRequestEnvelope(body)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}
	requestedModel := strings.TrimSpace(envelope.Model)
	r, releaseModelRequest := trackModelRequest(r, requestedModel)
	defer releaseModelRequest()
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
	decision := decideProtocolBridge(WireResponses, upstream, effectiveBridgeMode(r, upstream))
	strictBridge := decision.Mode == BridgeModeStrict
	effortMap := getReasoningEffortMapForAlias(modelAliasInfo)
	forwardReasoning := shouldForwardReasoningParameters(modelAliasInfo, aliasMatched)
	if resolvedModel != requestedModel || forwardReasoning {
		decision.MarkPatched()
	}
	decision.EvaluateCapabilities(upstream, requestCapabilities(body, WireResponses)...)
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
	hasHostedWebSearch := requestContainsHostedWebSearch(body)
	requiresHostedWebSearch := hasHostedWebSearch && requestRequiresHostedWebSearch(body)
	allowAutomaticWebSearchFallback := webSearchConfig.Enabled && !strictBridge
	negotiateHostedWebSearch := hasHostedWebSearch && allowAutomaticWebSearchFallback
	forceWebSearchFallback := allowAutomaticWebSearchFallback && shouldUseGatewayWebSearchFallback(upstream, webSearchConfig, resolvedModel) && hasHostedWebSearch
	if forceWebSearchFallback && decision.Path == BridgePathPassthrough {
		// 已缓存该上游不支持原生托管搜索；本地执行器使用 Chat 作为中间协议。
		decision.UsePivot()
	}
	applyDecisionHeaders(w.Header(), decision, nil)

	if decision.Path == BridgePathPassthrough && !forceWebSearchFallback && !negotiateHostedWebSearch {
		rawBody, err := prepareResponsesPassthroughBodyWithEffort(body, resolvedModel, effortMap, forwardReasoning)
		if err != nil {
			log.Printf("[请求无效] path=/v1/responses mode=passthrough model=%q 错误=%v", resolvedModel, err)
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		ServeNativeProtocol(w, NativeProxyRequest{
			Client: WireResponses, Body: rawBody, UpstreamName: upstreamName,
			Model: resolvedModel, UsageModel: requestedModel, Stream: envelope.Stream,
			Upstream: upstream, RequestContext: withNativeProtocolHeaders(r.Context(), r.Header),
		})
		return
	}

	var respReq ResponsesAPIRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid Responses request")
		return
	}
	respReq.Model = resolvedModel
	var requestFields map[string]any
	if err := json.Unmarshal(body, &requestFields); err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}
	responseEcho := responsesRequestEchoFields(requestFields)
	storeRequested, _ := requestFields["store"].(bool)
	storeEmulated := storeRequested && upstream.APIType != UpstreamResponses
	bridgeRequestBody := body
	_, localStateItems := resolveLocalResponsesState(requestFields, upstream)
	localStateEmulated := len(localStateItems) > 0
	if localStateEmulated {
		respReq.Input = prependResponsesStateItems(respReq.Input, localStateItems)
		bridgeRequestBody = replaceResponsesInput(body, respReq.Input)
	}
	useWebSearchFallback := allowAutomaticWebSearchFallback && shouldUseGatewayWebSearchFallback(upstream, webSearchConfig, respReq.Model) && containsHostedWebSearch(respReq.Tools)
	nativeSearchProbe := negotiateHostedWebSearch && !useWebSearchFallback && decision.Path == BridgePathPassthrough
	useNativeChatWebSearch := upstream.APIType == UpstreamOpenAI && !useWebSearchFallback && containsHostedWebSearch(respReq.Tools)
	requestBridgeWarnings := responsesBridgeRequestWarningsForUpstream(requestFields, upstream)
	if localStateEmulated {
		requestBridgeWarnings = removeBridgeWarning(requestBridgeWarnings, "stateful_context_ignored", "previous_response_id")
		requestBridgeWarnings = appendBridgeWarning(requestBridgeWarnings, BridgeWarning{
			Code: "stateful_context_emulated", Path: "previous_response_id",
			Message: "previous response items were loaded from the gateway's bounded local compatibility store",
		})
	}
	if storeEmulated {
		requestBridgeWarnings = removeBridgeWarning(requestBridgeWarnings, "storage_ignored", "store")
		requestBridgeWarnings = appendBridgeWarning(requestBridgeWarnings, BridgeWarning{
			Code: "storage_emulated", Path: "store",
			Message: "the response will be retained in the gateway's bounded local compatibility store",
		})
	}
	if nativeSearchProbe {
		requestBridgeWarnings = nil
	}
	if strictBridge && rejectStrictBridgeWarnings(w, r, requestBridgeWarnings) {
		return
	}
	// 转换输入历史前先构建跨协议工具计划。
	// 自定义工具调用项目需要与其定义使用相同的可逆名称映射。
	var bridgeWarnings []BridgeWarning
	bridgeWarnings = appendBridgeWarnings(bridgeWarnings, requestBridgeWarnings)
	bridgeToolDefinitions := append([]ResponsesTool(nil), respReq.Tools...)
	bridgeToolDefinitions = append(bridgeToolDefinitions, responsesLoadedToolDefinitions(respReq.Input)...)
	convertedTools, toolNameMappings, toolWarnings := convertResponsesToolsWithMappingsDetailed(bridgeToolDefinitions, useWebSearchFallback)
	if nativeSearchProbe {
		toolWarnings = nil
	} else if upstream.APIType == UpstreamAnthropic {
		toolWarnings = filterNativeHostedWebSearchWarnings(toolWarnings, respReq.Tools)
	} else if useNativeChatWebSearch {
		toolWarnings = filterNativeHostedWebSearchWarnings(toolWarnings, respReq.Tools)
		toolWarnings = appendBridgeWarnings(toolWarnings, responsesWebSearchToChatWarnings(respReq.Tools))
	}
	bridgeWarnings = appendBridgeWarnings(bridgeWarnings, toolWarnings)

	// 多模态路由
	messages := respReq.Messages
	if len(messages) == 0 {
		var inputWarnings []BridgeWarning
		messages, inputWarnings = responsesInputToMessagesWithWarnings(respReq.Input, respReq.Instructions, toolNameMappings)
		if nativeSearchProbe {
			inputWarnings = nil
		}
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, inputWarnings)
	} else if respReq.Instructions != "" {
		messages = append([]Message{{Role: "system", Content: respReq.Instructions}}, messages...)
	}
	if len(messages) == 0 && len(bridgeWarnings) > 0 {
		messages = append(messages, Message{
			Role: "user",
			Content: "The requested Responses state or tool context is unavailable through this compatibility bridge. " +
				"Explain that the missing context is required and ask the user to resend the relevant input explicitly.",
		})
		bridgeWarnings = appendBridgeWarning(bridgeWarnings, BridgeWarning{
			Code: "compatibility_fallback_prompt", Path: "input",
			Message: "no representable input remained after bridge conversion, so a compatibility fallback prompt was used",
		})
	}
	if !useWebSearchFallback && !useNativeChatWebSearch && upstream.APIType != UpstreamAnthropic && upstream.APIType != UpstreamResponses {
		messages = prependBridgeGuidance(messages, responsesHostedToolGuidance(respReq.Tools))
	}

	chatReq := OpenAIRequest{
		Model:                        respReq.Model,
		Messages:                     messages,
		Stream:                       respReq.Stream,
		AdditionalFields:             map[string]any{},
		ConfiguredReasoningEffortMap: effortMap,
	}
	if respReq.Temperature != nil {
		chatReq.Temperature = respReq.Temperature
	}
	if respReq.MaxTokens != 0 {
		chatReq.MaxTokens = respReq.MaxTokens
	}
	if respReq.TopP != nil {
		chatReq.TopP = respReq.TopP
	}
	if respReq.FrequencyPenalty != nil {
		chatReq.AdditionalFields["frequency_penalty"] = *respReq.FrequencyPenalty
	}
	if respReq.PresencePenalty != nil {
		chatReq.AdditionalFields["presence_penalty"] = *respReq.PresencePenalty
	}
	if respReq.Stop != nil {
		chatReq.AdditionalFields["stop"] = respReq.Stop
	}
	if respReq.User != "" {
		chatReq.AdditionalFields["user"] = respReq.User
	}
	if respReq.Metadata != nil {
		chatReq.AdditionalFields["metadata"] = respReq.Metadata
	}
	if upstream.APIType == UpstreamOpenAI {
		for _, field := range []string{
			"prompt_cache_key", "prompt_cache_options", "prompt_cache_retention",
			"service_tier", "safety_identifier", "moderation", "top_logprobs",
		} {
			if value, exists := requestFields[field]; exists && value != nil {
				chatReq.AdditionalFields[field] = value
			}
		}
		if value, exists := requestFields["top_logprobs"]; exists && value != nil {
			chatReq.AdditionalFields["logprobs"] = true
		}
	}
	if upstream.APIType == UpstreamOpenAI {
		if responseFormat, _ := responsesTextToChatResponseFormat(requestFields["text"]); responseFormat != nil {
			chatReq.AdditionalFields["response_format"] = responseFormat
		}
		if textConfig, ok := requestFields["text"].(map[string]any); ok {
			if verbosity, exists := textConfig["verbosity"]; exists && verbosity != nil {
				chatReq.AdditionalFields["verbosity"] = verbosity
			}
		}
		if useNativeChatWebSearch {
			if options := responsesWebSearchOptionsForChat(respReq.Tools); options != nil {
				chatReq.AdditionalFields["web_search_options"] = options
			}
		}
	}
	if len(convertedTools) > 0 {
		chatReq.Tools = convertedTools
	}
	if respReq.ToolChoice != nil {
		var choiceWarnings []BridgeWarning
		chatReq.ToolChoice, choiceWarnings = convertResponsesToolChoiceDetailed(respReq.ToolChoice, toolNameMappings, len(convertedTools) > 0)
		if nativeSearchProbe {
			choiceWarnings = nil
		}
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, choiceWarnings)
	}
	if respReq.ParallelToolCalls != nil {
		chatReq.AdditionalFields["parallel_tool_calls"] = *respReq.ParallelToolCalls
	}
	if respReq.Reasoning.Effort != "" {
		if respReq.Reasoning.Effort != "none" {
			chatReq.ReasoningEffort = respReq.Reasoning.Effort
		}
	}
	var pairwiseUpstreamBody []byte
	if decision.Upstream == WireAnthropic {
		var directMappings map[string]ResponseToolNameMapping
		var pairwiseWarnings []BridgeWarning
		pairwiseUpstreamBody, directMappings, pairwiseWarnings, err = convertResponsesRequestToAnthropicDirect(bridgeRequestBody, respReq.Model, forwardReasoning, effortMap)
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, pairwiseWarnings)
		if len(directMappings) > 0 {
			toolNameMappings = directMappings
		}
		if err != nil {
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	if decision.Path != BridgePathPassthrough {
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, conversionCapabilityBridgeWarnings(decision, hasHostedWebSearch))
	}
	if strictBridge && rejectStrictBridgeWarnings(w, r, bridgeWarnings) {
		return
	}

	chatReq.Messages = fixToolCallGaps(chatReq.Messages)
	chatReq.Messages, err = normalizeMessagesToolCallArguments(chatReq.Messages)
	if err != nil {
		log.Printf("[请求无效] path=/v1/responses model=%q 错误=%v", chatReq.Model, err)
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ensureReasoningEffort(&chatReq, modelAliasInfo)
	chatReq.Messages = ensureReasoningContent(chatReq.Messages, modelAliasInfo.WithReasoning)
	writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
	logBridgeWarnings("responses", upstreamName, chatReq.Model, bridgeWarnings)

	upstreamBody := pairwiseUpstreamBody
	if len(upstreamBody) == 0 {
		upstreamBody = buildUpstreamBody(&chatReq, forwardReasoning)
	}
	if nativeSearchProbe {
		upstreamBody, err = prepareResponsesPassthroughBodyWithEffort(body, respReq.Model, effortMap, forwardReasoning)
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
		fallbackTools, fallbackMappings, fallbackWarnings := convertResponsesToolsWithMappingsDetailed(bridgeToolDefinitions, true)
		toolNameMappings = fallbackMappings
		chatReq.Tools = fallbackTools
		delete(chatReq.AdditionalFields, "web_search_options")
		if respReq.ToolChoice != nil {
			var choiceWarnings []BridgeWarning
			chatReq.ToolChoice, choiceWarnings = convertResponsesToolChoiceDetailed(respReq.ToolChoice, toolNameMappings, len(fallbackTools) > 0)
			fallbackWarnings = appendBridgeWarnings(fallbackWarnings, choiceWarnings)
		}
		bridgeWarnings = appendBridgeWarnings(bridgeWarnings, fallbackWarnings)
		applyDecisionHeaders(w.Header(), decision, bridgeWarnings)
		logBridgeWarnings("responses", upstreamName, chatReq.Model, fallbackWarnings)
	}

	// 托管 Web Search 是 Responses 的原生能力（由上面的路径透传），默认映射为
	// Anthropic 服务端工具。Chat 上游先自动映射为 web_search_options；原生
	// 搜索明确不受支持时才进入回退执行器。执行器完成模型/搜索循环后，向
	// 现有 Responses 转换器返回最终 Chat 响应。
	serveWebSearchFallback := func() {
		loopResult := executeGatewayWebSearchLoop(
			r.Context(), chatReq, forwardReasoning, upstreamName, upstream,
			toolNameMappings, webSearchConfig,
		)
		if loopResult.Err != nil || loopResult.Status < 200 || loopResult.Status >= 300 {
			w.Header().Set("Content-Type", "application/json")
			status := applyUpstreamErrorHeaders(w, loopResult.Header, loopResult.Status)
			w.WriteHeader(status)
			if len(loopResult.Body) > 0 {
				w.Write(loopResult.Body)
			} else {
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "web search fallback upstream error"}})
			}
			return
		}
		responsesBody, convertErr := convertChatToResponsesForRequestWithError(
			loopResult.Body, chatReq.Model, bridgeRequestBody, toolNameMappings, bridgeWarnings,
		)
		if convertErr != nil {
			log.Printf("[响应转换失败] path=/v1/responses model=%q 错误=%v", chatReq.Model, convertErr)
			writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", "web search fallback returned an invalid Chat response")
			return
		}
		if storeEmulated {
			stored := false
			if respReq.Stream {
				stored = storeResponsesStreamResponse(responsesBody)
			} else {
				_, stored = bridgestate.Default().PutResponseBytes(responsesBody)
			}
			if !stored {
				bridgeWarnings = removeBridgeWarning(bridgeWarnings, "storage_emulated", "store")
				bridgeWarnings = appendBridgeWarning(bridgeWarnings, BridgeWarning{
					Code: "storage_ignored", Path: "store",
					Message: "the converted Responses response had no stable id and could not be retained locally",
				})
				writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
			}
			responsesBody = updateResponsesStoreResult(responsesBody, stored, bridgeWarnings)
		}
		copyFilteredResponseHeaders(w.Header(), loopResult.Header)
		usageStats := newRequestUsageAccumulatorForContext(r.Context(), requestedModel, upstreamName, chatReq.Model)
		var usageResponse map[string]any
		if json.Unmarshal(loopResult.Body, &usageResponse) == nil {
			if usage, ok := usageResponse["usage"].(map[string]any); ok {
				usageStats.observeMap(usage)
			}
		}
		usageStats.commit()
		if respReq.Stream {
			writeBufferedResponsesStream(w, responsesBody)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responsesBody)
	}
	if useWebSearchFallback {
		serveWebSearchFallback()
		return
	}

	// callUpstream/callUpstreamStream 内部会根据当前请求选中的 upstream.APIType 自动转换请求格式
	// 不需要在这里手动转换，避免双重转换导致请求体丢失
	// 流式响应需要特殊处理
	if respReq.Stream {
		var upResp io.ReadCloser
		var status int
		var upHeader http.Header
		if nativeSearchProbe {
			upResp, status, upHeader, err = callPreparedUpstreamStream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream, true)
		} else if decision.Upstream == WireAnthropic {
			upResp, status, upHeader, err = callPreparedUpstreamStream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream)
		} else {
			upResp, status, upHeader, err = callUpstreamStream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream)
		}
		var errBody []byte
		if (err != nil || status < 200 || status >= 300) && upResp != nil {
			errBody, _ = io.ReadAll(upResp)
			upResp.Close()
			upResp = nil
		}
		if hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, errBody) {
			markHostedWebSearchUnsupported(upstream, chatReq.Model)
			if webSearchConfig.Enabled && !strictBridge {
				activateWebSearchFallback()
				serveWebSearchFallback()
				return
			}
		}
		if decision.Upstream == WireChat && !strictBridge {
			if retryBody, retryWarnings, retry := downgradeRejectedChatOptions(upstreamBody, status); retry {
				bridgeWarnings = appendBridgeWarnings(bridgeWarnings, retryWarnings)
				writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
				logBridgeWarnings("responses", upstreamName, chatReq.Model, retryWarnings)
				upstreamBody = retryBody
				upResp, status, upHeader, err = callUpstreamStream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream)
				errBody = nil
			}
		}
		if err != nil || status < 200 || status >= 300 {
			if upResp != nil {
				errBody, _ = io.ReadAll(upResp)
				upResp.Close()
				upResp = nil
			}
			if hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, errBody) {
				markHostedWebSearchUnsupported(upstream, chatReq.Model)
				if webSearchConfig.Enabled && !strictBridge {
					activateWebSearchFallback()
					serveWebSearchFallback()
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			status = applyUpstreamErrorHeaders(w, upHeader, status)
			w.WriteHeader(status)
			if len(errBody) > 0 {
				w.Write(errBody)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "upstream error"}})
			return
		}
		if requiresHostedWebSearch && allowAutomaticWebSearchFallback && !isHostedWebSearchKnownSupported(upstream, chatReq.Model) {
			var probeBody []byte
			upResp, probeBody, err = bufferHostedWebSearchProbeStream(upResp)
			if err != nil {
				writeClientAPIError(w, WireResponses, http.StatusBadGateway, "upstream_protocol_error", "failed to inspect hosted web search stream")
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

		dispatchClientStream(w, WireResponses, decision, upstream, upResp, chatReq.Model, clientStreamOptions{
			UsageModel: requestedModel, UpstreamName: upstreamName, UpstreamModel: chatReq.Model, Request: r, RequestContext: r.Context(), Tools: respReq.Tools, ToolChoice: respReq.ToolChoice,
			ParallelToolCalls: respReq.ParallelToolCalls, ToolNameMappings: toolNameMappings,
			ResponseEcho: responseEcho, BridgeWarnings: bridgeWarnings,
			OnResponseCompleted: func(response map[string]any) {
				if storeEmulated {
					_, _ = bridgestate.Default().PutResponse(response)
				}
			},
		})
		return
	}

	var respBody []byte
	var status int
	var upHeader http.Header
	if nativeSearchProbe {
		respBody, status, upHeader, err = callPreparedUpstream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream, true)
	} else if decision.Upstream == WireAnthropic {
		respBody, status, upHeader, err = callPreparedUpstream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream, true)
	} else {
		respBody, status, upHeader, err = callUpstream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream, upstream != nil && upstream.APIType == UpstreamResponses)
	}
	if (err != nil || status < 200 || status >= 300) && hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, respBody) {
		markHostedWebSearchUnsupported(upstream, chatReq.Model)
		if webSearchConfig.Enabled && !strictBridge {
			activateWebSearchFallback()
			serveWebSearchFallback()
			return
		}
	}
	if decision.Upstream == WireChat && !strictBridge {
		if retryBody, retryWarnings, retry := downgradeRejectedChatOptions(upstreamBody, status); retry {
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, retryWarnings)
			writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
			logBridgeWarnings("responses", upstreamName, chatReq.Model, retryWarnings)
			upstreamBody = retryBody
			respBody, status, upHeader, err = callUpstream(r.Context(), upstreamBody, upstreamName, chatReq.Model, upstream)
		}
	}
	if (err != nil || status < 200 || status >= 300) && hasHostedWebSearch && isHostedWebSearchUnsupportedResponse(status, respBody) {
		markHostedWebSearchUnsupported(upstream, chatReq.Model)
		if webSearchConfig.Enabled && !strictBridge {
			activateWebSearchFallback()
			serveWebSearchFallback()
			return
		}
	}
	if err != nil || status < 200 || status >= 300 {
		w.Header().Set("Content-Type", "application/json")
		status = applyUpstreamErrorHeaders(w, upHeader, status)
		w.WriteHeader(status)
		if len(respBody) > 0 {
			if decision.Upstream == WireAnthropic {
				respBody = mapUpstreamErrorBody(respBody, UpstreamAnthropic)
			}
			w.Write(respBody)
		} else {
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "upstream error"}})
		}
		return
	}
	if requiresHostedWebSearch && webSearchConfig.Enabled {
		if responseContainsHostedWebSearchEvidence(respBody) {
			markHostedWebSearchSupported(upstream, chatReq.Model)
		} else if responseRepresentsSuccessfulCompletion(respBody) {
			markHostedWebSearchUnsupported(upstream, chatReq.Model)
			if !strictBridge {
				log.Printf("[Hosted Web Search 自动回退] 上游=%s 模型=%s 原因=成功响应未执行强制搜索", effectiveUpstreamName(upstreamName), chatReq.Model)
				activateWebSearchFallback()
				serveWebSearchFallback()
				return
			}
		}
	}

	// 上游已是 Responses 格式，直接使用原始响应
	var responsesBody []byte
	if upstream != nil && upstream.APIType == UpstreamResponses {
		responsesBody = respBody
	} else if decision.Upstream == WireAnthropic {
		var responseWarnings []BridgeWarning
		responsesBody, responseWarnings, err = convertAnthropicResponseToResponsesDirect(respBody, chatReq.Model, requestFields, toolNameMappings, bridgeWarnings)
		if err != nil {
			log.Printf("[响应转换失败] path=/v1/responses model=%q 错误=%v", chatReq.Model, err)
			writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", "upstream returned an invalid Anthropic response")
			return
		}
		if len(responseWarnings) > 0 {
			if decision.Mode == BridgeModeStrict {
				writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", bridgeWarningsError(responseWarnings).Error())
				return
			}
			bridgeWarnings = appendBridgeWarnings(bridgeWarnings, responseWarnings)
			writeBridgeWarningHeaders(w.Header(), bridgeWarnings)
			logBridgeWarnings("responses", upstreamName, chatReq.Model, responseWarnings)
		}
	} else {
		// callUpstream 返回的 respBody 已统一为 Chat 格式（Anthropic/OpenAI 上游在内部已转换）
		// 需要转为 Responses API 格式返回给客户端
		responsesBody, err = convertChatToResponsesForRequestWithError(respBody, chatReq.Model, bridgeRequestBody, toolNameMappings, bridgeWarnings)
		if err != nil {
			log.Printf("[响应转换失败] path=/v1/responses model=%q 错误=%v", chatReq.Model, err)
			writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", "upstream returned an invalid Chat response")
			return
		}
	}
	if storeEmulated {
		_, stored := bridgestate.Default().PutResponseBytes(responsesBody)
		if !stored {
			bridgeWarnings = removeBridgeWarning(bridgeWarnings, "storage_emulated", "store")
			bridgeWarnings = appendBridgeWarning(bridgeWarnings, BridgeWarning{
				Code: "storage_ignored", Path: "store",
				Message: "the converted Responses response had no stable id and could not be retained locally",
			})
		}
		responsesBody = updateResponsesStoreResult(responsesBody, stored, bridgeWarnings)
	}

	usageStats := newRequestUsageAccumulatorForContext(r.Context(), requestedModel, upstreamName, chatReq.Model)
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
		log.Printf("[Responses 响应]\n%s", string(responsesBody))
	}
	w.Write(responsesBody)
}

// ======================== Responses 流处理器 ========================
