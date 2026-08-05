package stream

import (
	"context"
	"io"
	"log"
	"net/http"

	"llmrelay/backend/internal/stats"
)

// ClientStreamOptions 携带部分流转换器所需的额外状态。
type ClientStreamOptions struct {
	UsageModel        string
	UpstreamName      string
	UpstreamModel     string
	RequestContext    context.Context
	Request           *http.Request
	Tools             any
	ToolChoice        any
	ParallelToolCalls *bool
	ToolNameMappings  map[string]ResponseToolNameMapping
	ResponseEcho      map[string]any
	BridgeWarnings    []BridgeWarning
}

// DispatchClientStream 将成功的上游流路由到正确的客户端转换器或透传代理。
func DispatchClientStream(
	w http.ResponseWriter,
	client WireProtocol,
	decision ProtocolDecision,
	upstream *UpstreamConfig,
	upResp io.ReadCloser,
	model string,
	opts ClientStreamOptions,
) {
	kind := ChooseStreamDispatch(client, decision, upstream)
	usageModel := opts.UsageModel
	if usageModel == "" {
		usageModel = model
	}
	if opts.UpstreamName != "" || opts.UpstreamModel != "" {
		usageModel = stats.UsageIdentityForContext(opts.RequestContext, usageModel, opts.UpstreamName, opts.UpstreamModel)
	}
	switch kind {
	case streamKindChatPassthrough:
		SetSSEHeaders(w.Header())
		w.WriteHeader(http.StatusOK)
		if err := ProxyChatPassthroughStream(w, upResp, usageModel); err != nil && debugMode {
			log.Printf("[Chat 透传流错误] %v", err)
		}
	case streamKindAnthropicPassthrough:
		SetSSEHeaders(w.Header())
		w.WriteHeader(http.StatusOK)
		if err := ProxyAnthropicPassthroughStream(w, upResp, usageModel); err != nil && debugMode {
			log.Printf("[Anthropic 透传流错误] %v", err)
		}
	case streamKindResponsesPassthrough:
		SetSSEHeaders(w.Header())
		w.WriteHeader(http.StatusOK)
		if err := ProxyResponsesPassthroughStream(w, upResp, usageModel); err != nil && debugMode {
			log.Printf("[Responses 透传流错误] %v", err)
		}
	case streamKindAnthropicToChat:
		AnthropicStreamToChatHandler(w, upResp, model, usageModel, true)
	case streamKindResponsesToChat:
		ResponsesStreamToChatHandler(w, upResp, model, usageModel, true)
	case streamKindChatToAnthropic:
		ClaudeStreamHandler(w, upResp, model, usageModel)
	case streamKindResponsesToAnthropic:
		ResponsesStreamToAnthropicDirectHandler(w, upResp, model, usageModel)
	case streamKindAnthropicToResponses:
		AnthropicStreamToResponsesDirectHandler(
			w, upResp, model, usageModel, decision.Mode,
			opts.Tools, opts.ToolChoice, opts.ParallelToolCalls,
			opts.ToolNameMappings, opts.ResponseEcho, opts.BridgeWarnings,
		)
	case streamKindChatToResponses:
		resp := &http.Response{Body: upResp, Header: make(http.Header)}
		ResponsesStreamHandler(
			w, opts.Request, resp, model, usageModel,
			opts.Tools, opts.ToolChoice, opts.ParallelToolCalls,
			opts.ToolNameMappings, opts.ResponseEcho, opts.BridgeWarnings,
		)
	default:
		upResp.Close()
		WriteClientAPIError(w, client, http.StatusBadGateway, "upstream_protocol_error", "unsupported stream bridge path")
	}
}
