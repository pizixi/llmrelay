package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"llmrelay/backend/internal/routing"
	"llmrelay/backend/internal/sse"
	"llmrelay/backend/internal/upstream"
)

func observeUpstreamResult(ctx context.Context, body []byte, status int, err error) {
	outcome, _ := ctx.Value(routeOutcomeKey{}).(*routeOutcome)
	if outcome == nil {
		return
	}
	outcome.failure = nil
	if err != nil && (status == 0 || (status >= 200 && status < 300)) {
		failure := routing.ClassifyFailure(http.StatusBadGateway, body)
		outcome.failure = &failure
	}
}

// Wait for the first upstream byte before protocol converters can emit their
// opening events. An empty or failed stream can then move to the next target.
func observeUpstreamStream(ctx context.Context, body io.ReadCloser, status int, headers http.Header, err error, target *UpstreamConfig) (io.ReadCloser, int, http.Header, error) {
	observeUpstreamResult(ctx, nil, status, err)
	outcome, _ := ctx.Value(routeOutcomeKey{}).(*routeOutcome)
	if outcome == nil || err != nil || status < 200 || status >= 300 || body == nil {
		return body, status, headers, err
	}
	observed := &routeStreamReader{ReadCloser: body, outcome: outcome, ctx: ctx, parser: sse.NewParser(sse.DefaultMaxEventBytes), protocol: target.APIType}
	if observed.protocol == "" {
		observed.protocol = UpstreamOpenAI
	}
	reader := bufio.NewReader(observed)
	if _, readErr := reader.Peek(1); readErr != nil {
		_ = body.Close()
		failure := routing.ClassifyFailure(http.StatusBadGateway, nil)
		outcome.failure = &failure
		failureBody := []byte(`{"error":{"type":"upstream_error","message":"upstream stream ended before returning data"}}`)
		return io.NopCloser(bytes.NewReader(failureBody)), http.StatusBadGateway, http.Header{"Content-Type": {"application/json"}}, readErr
	}
	return &nativeResponseReadCloser{Reader: reader, closer: body}, status, headers, nil
}

type routeStreamReader struct {
	io.ReadCloser
	outcome  *routeOutcome
	ctx      context.Context
	parser   *sse.Parser
	protocol UpstreamType
	terminal bool
}

func (reader *routeStreamReader) Read(body []byte) (int, error) {
	n, err := reader.ReadCloser.Read(body)
	reader.observe(reader.parser.Feed(body[:n]))
	if err != nil {
		reader.observe(reader.parser.Flush())
	}
	if err != nil && !errors.Is(err, io.EOF) && !(reader.terminal && errors.Is(err, io.ErrUnexpectedEOF)) && reader.ctx.Err() == nil {
		failure := routing.ClassifyFailure(http.StatusBadGateway, nil)
		reader.outcome.failure = &failure
	}
	return n, err
}

func (reader *routeStreamReader) observe(events []sse.Event) {
	for _, event := range events {
		reader.terminal = upstream.IsNativeStreamTerminal(event, reader.protocol) || reader.terminal
		if reader.protocol == UpstreamOpenAI {
			var chunk struct {
				Choices []struct {
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(event.Data), &chunk) == nil && len(chunk.Choices) > 0 {
				for _, choice := range chunk.Choices {
					reader.terminal = choice.FinishReason != "" || reader.terminal
				}
			}
		}
	}
}
