package gateway

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/routing/health"
	"llmrelay/backend/internal/upstream"
)

type routedRequest struct {
	Body              []byte
	Envelope          NativeRequestEnvelope
	RequestedModel    string
	Route             config.ModelRoute
	CaptureChatStream func(context.Context, []byte)
}

type routeOutcomeKey struct{}

type routeOutcome struct {
	failure      *health.Failure
	noCredential bool
	chatFallback upstream.ChatStreamFallback
}

func serveModelRequest(w http.ResponseWriter, r *http.Request, serve func(http.ResponseWriter, *http.Request, routedRequest)) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	body, status, err := readAPIRequestBody(w, r)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, status, "invalid_request_error", err.Error())
		return
	}
	count := requestCount.Add(1)
	if debugMode && debugLogBodies {
		log.Printf("[请求 #%d] POST %s\n%s", count, r.URL.Path, body)
	}
	envelope, err := DecodeNativeRequestEnvelope(body)
	if err != nil {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "Invalid JSON")
		return
	}
	model := strings.TrimSpace(envelope.Model)
	if model == "" {
		writeExternalAPIError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	r, release := trackModelRequest(r, model)
	defer release()
	routes, matched, policy := config.ResolveRequestModelCandidates(model, func(route config.ModelRoute) bool {
		available, _ := upstream.ModelTargetAvailability(route.UpstreamName, route.Model, route.Upstream)
		return available
	})
	if !matched {
		writeModelNotFoundError(w, r.URL.Path, model)
		return
	}
	ctx := r.Context()
	if len(routes) == 1 {
		ctx = upstream.WithSingleTargetRoutingPolicy(ctx, policy)
		policy.DisableFailover = true
		policy.DisableCooldown = true
	} else {
		ctx = upstream.WithRoutingPolicy(ctx, policy)
	}
	r = r.WithContext(ctx)
	attempts := 0
	var retryDelay time.Duration
	for index, route := range routes {
		if r.Context().Err() != nil {
			return
		}
		if (policy.DisableFailover && attempts > 0) || (policy.MaxAttempts > 0 && attempts >= policy.MaxAttempts) {
			break
		}
		if !policy.DisableCooldown {
			if available, delay := upstream.ModelTargetAvailability(route.UpstreamName, route.Model, route.Upstream); !available {
				retryDelay = minimumRetryDelay(retryDelay, delay)
				continue
			}
		}
		attempts++
		outcome := &routeOutcome{}
		attemptRequest := r.WithContext(context.WithValue(r.Context(), routeOutcomeKey{}, outcome))
		response := &routeResponseWriter{destination: w, header: w.Header().Clone()}
		request := routedRequest{Body: body, Envelope: envelope, RequestedModel: model, Route: route}
		remaining := routes[index+1:]
		if envelope.Stream && isChatModelRoute(route) && !policy.DisableFailover &&
			(policy.MaxAttempts == 0 || attempts < policy.MaxAttempts) && slices.ContainsFunc(remaining, isChatModelRoute) {
			outcome.chatFallback = chatModelFallback(r, request, remaining, policy, &attempts, serve)
		}
		if route.Upstream == nil {
			writeExternalAPIError(response, r.URL.Path, http.StatusInternalServerError, "upstream_configuration_error", "selected upstream is not configured")
		} else {
			serve(response, attemptRequest, request)
		}
		if r.Context().Err() != nil {
			return
		}
		if policy.DisableFailover && policy.DisableCooldown {
			response.finish()
			return
		}
		failure := health.ClassifyFailure(response.statusCode(), response.errorBody.Bytes())
		if outcome.failure != nil {
			failure = *outcome.failure
		}
		if !policy.DisableCooldown && (outcome.noCredential || failure.Failover) {
			_, delay := upstream.ModelTargetAvailability(route.UpstreamName, route.Model, route.Upstream)
			retryDelay = minimumRetryDelay(retryDelay, delay)
		}
		if outcome.noCredential && !response.committed {
			attempts--
			continue
		}
		if failure.Kind == "rate_limit" {
			retryDelay = minimumRetryDelay(retryDelay, upstream.ParseRetryAfterDelay(response.header.Get("Retry-After")))
		}
		if response.committed || !failure.Failover || policy.DisableFailover {
			response.finish()
			return
		}
		log.Printf("[模型故障切换] 请求=%d 别名=%s 上游=%s 模型=%s 状态码=%d 分类=%s 尝试=%d", count, model, route.UpstreamName, route.Model, response.statusCode(), failure.Kind, attempts)
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	w.Header().Set("Retry-After", strconv.Itoa(int((retryDelay+time.Second-1)/time.Second)))
	writeExternalAPIError(w, r.URL.Path, http.StatusServiceUnavailable, "upstream_unavailable", "all configured model targets are temporarily unavailable")
}

func isChatModelRoute(route config.ModelRoute) bool {
	return route.Upstream != nil && route.Upstream.APIType == UpstreamOpenAI
}

func chatModelFallback(r *http.Request, request routedRequest, remaining []config.ModelRoute,
	policy config.ModelRoutingConfig, attempts *int, serve func(http.ResponseWriter, *http.Request, routedRequest)) upstream.ChatStreamFallback {
	return func(ctx context.Context, cause error) (*upstream.ChatStreamBackup, error) {
		// The existing converter verifies the delivered output against the backup
		// stream before forwarding new deltas.
		for len(remaining) > 0 && (policy.MaxAttempts == 0 || *attempts < policy.MaxAttempts) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			route := remaining[0]
			remaining = remaining[1:]
			if !isChatModelRoute(route) {
				continue
			}
			if !policy.DisableCooldown {
				if available, _ := upstream.ModelTargetAvailability(route.UpstreamName, route.Model, route.Upstream); !available {
					continue
				}
			}
			var backupCtx context.Context
			var chatBody []byte
			request.Route = route
			request.CaptureChatStream = func(ctx context.Context, body []byte) {
				backupCtx, chatBody = ctx, body
			}
			serve(&routeResponseWriter{header: make(http.Header)}, r.WithContext(ctx), request)
			if chatBody == nil {
				continue
			}
			(*attempts)++
			backup, status, err := upstream.OpenChatStreamBackup(backupCtx, chatBody, route.UpstreamName, route.Model, route.Upstream)
			if errors.Is(err, upstream.ErrNoAvailableUpstreamKey) {
				(*attempts)--
				continue
			}
			log.Printf("[模型流故障切换] 别名=%s 上游=%s 模型=%s 状态码=%d 尝试=%d 原因=%v", request.RequestedModel, route.UpstreamName, route.Model, status, *attempts, cause)
			if err == nil {
				return backup, nil
			}
		}
		return nil, cause
	}
}

func minimumRetryDelay(current, next time.Duration) time.Duration {
	if next > 0 && (current <= 0 || next < current) {
		return next
	}
	return current
}

// Error responses are held until routing decides whether to try another target.
// Successful output is forwarded immediately, including SSE flushes.
type routeResponseWriter struct {
	destination http.ResponseWriter
	header      http.Header
	status      int
	committed   bool
	errorBody   bytes.Buffer
}

func (writer *routeResponseWriter) Header() http.Header { return writer.header }

func (writer *routeResponseWriter) statusCode() int {
	if writer.status == 0 {
		return http.StatusOK
	}
	return writer.status
}

func (writer *routeResponseWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
}

func (writer *routeResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.status >= 400 {
		return writer.errorBody.Write(body)
	}
	if writer.destination == nil {
		return len(body), nil
	}
	writer.commit()
	return writer.destination.Write(body)
}

func (writer *routeResponseWriter) commit() {
	if writer.committed {
		return
	}
	for name := range writer.destination.Header() {
		delete(writer.destination.Header(), name)
	}
	for name, values := range writer.header {
		writer.destination.Header()[name] = append([]string(nil), values...)
	}
	writer.destination.WriteHeader(writer.statusCode())
	writer.committed = true
}

func (writer *routeResponseWriter) Flush() {
	if writer.destination != nil && writer.statusCode() < 400 {
		writer.commit()
		_ = http.NewResponseController(writer.destination).Flush()
	}
}

func (writer *routeResponseWriter) Unwrap() http.ResponseWriter { return writer.destination }

func (writer *routeResponseWriter) finish() {
	if !writer.committed {
		writer.commit()
		_, _ = writer.destination.Write(writer.errorBody.Bytes())
	}
}
