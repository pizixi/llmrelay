package gateway

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/routing"
	"llmrelay/backend/internal/upstream"
)

type routedRequest struct {
	Body           []byte
	Envelope       NativeRequestEnvelope
	RequestedModel string
	Route          config.ModelRoute
}

type routeOutcomeKey struct{}

type routeOutcome struct{ failure *routing.Failure }

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
	routes, matched, policy := config.ResolveRequestModelCandidates(model, routing.TargetAvailable)
	if !matched {
		writeModelNotFoundError(w, r.URL.Path, model)
		return
	}
	attempts := 0
	var retryDelay time.Duration
	for _, route := range routes {
		if r.Context().Err() != nil {
			return
		}
		if (policy.DisableFailover && attempts > 0) || (policy.MaxAttempts > 0 && attempts >= policy.MaxAttempts) {
			break
		}
		attempt, delay := routing.TryTarget(route, policy)
		if attempt == nil {
			retryDelay = minimumRetryDelay(retryDelay, delay)
			continue
		}
		defer attempt.Complete(nil)
		attempts++
		outcome := &routeOutcome{}
		attemptRequest := r.WithContext(context.WithValue(r.Context(), routeOutcomeKey{}, outcome))
		response := &routeResponseWriter{destination: w, header: w.Header().Clone()}
		if route.Upstream == nil {
			writeExternalAPIError(response, r.URL.Path, http.StatusInternalServerError, "upstream_configuration_error", "selected upstream is not configured")
		} else {
			serve(response, attemptRequest, routedRequest{Body: body, Envelope: envelope, RequestedModel: model, Route: route})
		}
		if r.Context().Err() != nil {
			attempt.Complete(nil)
			return
		}
		failure := routing.ClassifyFailure(response.statusCode(), response.errorBody.Bytes())
		if outcome.failure != nil {
			failure = *outcome.failure
		}
		cooldown := attempt.Complete(&failure)
		retryDelay = minimumRetryDelay(retryDelay, cooldown)
		if failure.Kind == "rate_limit" {
			retryDelay = minimumRetryDelay(retryDelay, upstream.ParseRetryAfterDelay(response.header.Get("Retry-After")))
		}
		if cooldown > 0 {
			log.Printf("[模型冷却] 请求=%d 上游=%s 模型=%s 分类=%s 冷却=%s", count, route.UpstreamName, route.Model, failure.Kind, cooldown)
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
	if writer.statusCode() < 400 {
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
