package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"llmrelay/backend/internal/routing/health"
	"llmrelay/backend/internal/upstream"
)

func observeUpstreamResult(ctx context.Context, body []byte, status int, err error) {
	outcome := observeUpstreamTransport(ctx, status, err)
	if outcome == nil || outcome.failure != nil {
		return
	}
	// A 2xx response with nothing in it cannot be served to the client. Treat it
	// as an upstream failure so routing can move on instead of forwarding an
	// empty 200 that clients report as a broken gateway response.
	if status >= 200 && status < 300 && len(bytes.TrimSpace(body)) == 0 {
		outcome.failure = &health.Failure{Kind: "empty_response", Failover: true, Cooldown: true}
	}
}

// observeUpstreamTransport records connection failures and failed 2xx bodies.
func observeUpstreamTransport(ctx context.Context, status int, err error) *routeOutcome {
	outcome, _ := ctx.Value(routeOutcomeKey{}).(*routeOutcome)
	if outcome == nil {
		return nil
	}
	outcome.noCredential = errors.Is(err, upstream.ErrNoAvailableUpstreamKey)
	outcome.failure = nil
	var responseErr *upstream.ResponseError
	if errors.As(err, &responseErr) {
		outcome.failure = &responseErr.Failure
	} else if err != nil && (status == 0 || (status >= 200 && status < 300)) {
		failure := health.ClassifyFailure(http.StatusBadGateway, nil)
		outcome.failure = &failure
	}
	return outcome
}

// Wait for the first meaningful upstream byte before protocol converters can
// emit their opening events. Empty, whitespace-only or non-SSE/JSON streams are
// treated as failures so model routing can try the next target.
func observeUpstreamStream(ctx context.Context, body io.ReadCloser, status int, headers http.Header, err error) (io.ReadCloser, int, http.Header, error) {
	outcome := observeUpstreamTransport(ctx, status, err)
	if outcome == nil || err != nil || status < 200 || status >= 300 || body == nil {
		return body, status, headers, err
	}
	reader, readErr := upstream.PeekStreamStart(body)
	if readErr == nil {
		return &nativeResponseReadCloser{Reader: reader, closer: body}, status, headers, nil
	}
	_ = body.Close()
	if errors.Is(readErr, upstream.ErrMalformedUpstreamStream) {
		outcome.failure = &health.Failure{Kind: "invalid_stream", Failover: true, Cooldown: true}
		return upstreamStreamFailureBody(readErr, upstream.ErrMalformedUpstreamStream.Error())
	}
	outcome.failure = &health.Failure{Kind: "empty_response", Failover: true, Cooldown: true}
	return upstreamStreamFailureBody(readErr, "upstream stream ended before returning data")
}

func upstreamStreamFailureBody(cause error, message string) (io.ReadCloser, int, http.Header, error) {
	failureBody := []byte(`{"error":{"type":"upstream_error","message":"` + message + `"}}`)
	return io.NopCloser(bytes.NewReader(failureBody)), http.StatusBadGateway, http.Header{"Content-Type": {"application/json"}}, cause
}
