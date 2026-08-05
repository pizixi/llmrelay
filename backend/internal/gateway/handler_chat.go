package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

func ChatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("[请求 #%d] POST /v1/chat/completions\n%s", cnt, string(body))
	}

	envelope, err := DecodeNativeRequestEnvelope(body)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}
	requestedModel := strings.TrimSpace(envelope.Model)
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
	bridgeMode := effectiveBridgeMode(r, upstream)
	decision := decideProtocolBridge(WireChat, upstream, bridgeMode)
	effortMap := getReasoningEffortMapForAlias(modelAliasInfo)
	forwardReasoning := shouldForwardReasoningParameters(modelAliasInfo, aliasMatched)
	if decision.Path == BridgePathPassthrough {
		applyDecisionHeaders(w.Header(), decision, nil)
		nativeRequest := OpenAIRequest{ReasoningEffort: envelope.ReasoningEffort}
		ensureReasoningEffort(&nativeRequest, modelAliasInfo)
		nativeBody, prepareErr := prepareChatPassthroughBody(
			body, resolvedModel, mapConfiguredReasoningEffort(nativeRequest.ReasoningEffort, effortMap), forwardReasoning,
		)
		if prepareErr != nil {
			writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", prepareErr.Error())
			return
		}
		ServeNativeProtocol(w, NativeProxyRequest{
			Client: WireChat, Body: nativeBody, UpstreamName: upstreamName,
			Model: resolvedModel, UsageModel: requestedModel, Stream: envelope.Stream,
			Upstream: upstream, RequestContext: withNativeProtocolHeaders(r.Context(), r.Header),
		})
		return
	}

	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid Chat request")
		return
	}
	req.Model = resolvedModel
	req.ConfiguredReasoningEffortMap = effortMap
	ensureReasoningEffort(&req, modelAliasInfo)

	// 多模态路由：检测到图片时转发到配置的上游

	req.Messages = fixToolCallGaps(req.Messages)
	var toolArgsErr error
	req.Messages, toolArgsErr = normalizeMessagesToolCallArguments(req.Messages)
	if toolArgsErr != nil {
		log.Printf("[请求无效] path=/v1/chat/completions model=%q 错误=%v", req.Model, toolArgsErr)
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", toolArgsErr.Error())
		return
	}
	req.Messages = ensureReasoningContent(req.Messages, modelAliasInfo.WithReasoning)
	bridgeWarnings := chatBridgeWarnings(&req, upstream)
	if decision.Mode == BridgeModeStrict && rejectStrictBridgeWarnings(w, r, bridgeWarnings) {
		return
	}
	applyDecisionHeaders(w.Header(), decision, bridgeWarnings)
	logBridgeWarnings("chat", upstreamName, req.Model, bridgeWarnings)
	upstreamBody := buildUpstreamBody(&req, forwardReasoning)

	if req.Stream {
		upResp, status, upHeader, err := callUpstreamStream(r.Context(), upstreamBody, upstreamName, req.Model, upstream)
		if err != nil || status < 200 || status >= 300 {
			w.Header().Set("Content-Type", "application/json")
			status = applyUpstreamErrorHeaders(w, upHeader, status)
			w.WriteHeader(status)
			if upResp != nil {
				errBody, _ := io.ReadAll(upResp)
				if len(errBody) > 0 {
					w.Write(errBody)
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "upstream error", "type": "upstream_error"}})
			return
		}
		defer upResp.Close()
		copyFilteredResponseHeaders(w.Header(), upHeader)
		dispatchClientStream(w, WireChat, decision, upstream, upResp, req.Model, clientStreamOptions{UsageModel: requestedModel, UpstreamName: upstreamName, UpstreamModel: req.Model, RequestContext: r.Context()})
		return
	}

	respBody, status, upHeader, err := callUpstream(r.Context(), upstreamBody, upstreamName, req.Model, upstream)
	if err != nil || status < 200 || status >= 300 {
		w.Header().Set("Content-Type", "application/json")
		status = applyUpstreamErrorHeaders(w, upHeader, status)
		w.WriteHeader(status)
		if len(respBody) > 0 {
			w.Write(respBody)
		} else {
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "upstream error", "type": "upstream_error"}})
		}
		return
	}
	if err := validateChatResponseForBridge(respBody); err != nil {
		log.Printf("[响应转换失败] path=/v1/chat/completions model=%q 错误=%v", req.Model, err)
		writeExternalAPIError(w, r.URL.Path, http.StatusBadGateway, "upstream_protocol_error", "upstream returned an invalid bridged response")
		return
	}
	outBody := respBody
	if upstream == nil || upstream.APIType != UpstreamOpenAI {
		convertedResp, convertErr := convertResponse(respBody)
		if convertErr == nil {
			outBody = convertedResp
		}
	}
	usageStats := newRequestUsageAccumulatorForContext(r.Context(), requestedModel, upstreamName, req.Model)
	var usageResp2 map[string]any
	if json.Unmarshal(respBody, &usageResp2) == nil {
		if u, ok := usageResp2["usage"].(map[string]any); ok {
			usageStats.observeMap(u)
		}
	}
	usageStats.commit()
	copyFilteredResponseHeaders(w.Header(), upHeader)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(outBody)
}

// ======================== 模型处理器 ========================

func ListModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	models := getRoutableModelInfos()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   models,
	})
}
