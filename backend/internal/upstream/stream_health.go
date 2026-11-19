package upstream

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"llmrelay/backend/internal/routing/health"
	"llmrelay/backend/internal/sse"
)

var ErrMalformedUpstreamStream = errors.New("upstream returned a malformed stream")

const maxStreamPrefixBytes = 64

type streamPrefixKind uint8

const (
	streamPrefixPending streamPrefixKind = iota
	streamPrefixEmpty
	streamPrefixMalformed
	streamPrefixJSON
	streamPrefixSSE
)

type streamPrefix struct {
	kind    streamPrefixKind
	skipped int
}

func (prefix *streamPrefix) scan(data []byte) {
	for _, value := range data {
		switch value {
		case ' ', '\t', '\r', '\n', 0xef, 0xbb, 0xbf:
			prefix.skipped++
			if prefix.skipped < maxStreamPrefixBytes {
				continue
			}
			prefix.kind = streamPrefixEmpty
		case '{', '[':
			prefix.kind = streamPrefixJSON
		case 'd', 'e', 'i', 'r', ':':
			prefix.kind = streamPrefixSSE
		default:
			prefix.kind = streamPrefixMalformed
		}
		return
	}
}

// PeekStreamStart checks for a possible SSE or JSON response. The returned
// reader retains all bytes for the protocol handler.
func PeekStreamStart(body io.Reader) (*bufio.Reader, error) {
	reader := bufio.NewReader(body)
	var prefix streamPrefix
	for prefix.kind == streamPrefixPending {
		data, err := reader.Peek(prefix.skipped + 1)
		if err != nil {
			return nil, err
		}
		prefix.scan(data[len(data)-1:])
	}
	switch prefix.kind {
	case streamPrefixEmpty:
		return nil, ErrEmptyUpstreamResponse
	case streamPrefixMalformed:
		return nil, ErrMalformedUpstreamStream
	default:
		return reader, nil
	}
}

type keyStreamBody struct {
	io.ReadCloser
	complete      func(*health.Failure)
	parser        *sse.Parser
	protocol      UpstreamType
	requireFinish bool
	prefix        streamPrefix
	finished      bool
}

func newKeyStreamBody(reader io.ReadCloser, protocol UpstreamType, requireFinish bool, complete func(*health.Failure)) *keyStreamBody {
	if protocol == "" {
		protocol = UpstreamOpenAI
	}
	body := &keyStreamBody{
		ReadCloser: reader, complete: complete, protocol: protocol, requireFinish: requireFinish,
		parser: sse.NewParser(sse.DefaultMaxEventBytes),
	}
	if requireFinish && protocol == UpstreamOpenAI {
		// The Chat replay reader forwards parsed events, so framing prefixes
		// and blank lines are handled by the SSE parser.
		body.prefix.kind = streamPrefixSSE
	}
	return body
}

func (body *keyStreamBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if body.finished {
		return n, err
	}
	if body.prefix.kind == streamPrefixPending {
		body.prefix.scan(p[:n])
		switch body.prefix.kind {
		case streamPrefixEmpty:
			body.finish(&health.Failure{Kind: "empty_response", Cooldown: true})
		case streamPrefixMalformed:
			body.finish(&health.Failure{Kind: "invalid_stream", Cooldown: true})
		}
	}
	if body.prefix.kind == streamPrefixSSE {
		body.observe(body.parser.Feed(p[:n]))
	}
	if err != nil && !body.finished {
		if body.prefix.kind == streamPrefixSSE {
			body.observe(body.parser.Flush())
		}
		if !body.finished {
			var failure health.Failure
			if !errors.Is(err, io.EOF) || body.prefix.kind == streamPrefixPending || (body.prefix.kind == streamPrefixSSE && !body.parser.Dropped()) {
				failure = health.Failure{Kind: "stream_read", Cooldown: true}
			}
			body.finish(&failure)
		}
	}
	return n, err
}

func (body *keyStreamBody) Close() error {
	body.complete(nil)
	return body.ReadCloser.Close()
}

func (body *keyStreamBody) finish(outcome *health.Failure) {
	body.finished = true
	body.complete(outcome)
}

func (body *keyStreamBody) observe(events []sse.Event) {
	for _, event := range events {
		if strings.TrimSpace(event.Data) == "[DONE]" {
			failure := health.Failure{}
			if body.protocol == UpstreamOpenAI && body.requireFinish {
				failure = health.Failure{Kind: "stream_missing_finish", Cooldown: true}
			}
			body.finish(&failure)
			return
		}
		var chunk struct {
			Type    string          `json:"type"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(event.Data), &chunk) != nil {
			continue
		}
		kind := chunk.Type
		if kind == "" {
			kind = event.Name
		}
		if (len(chunk.Error) > 0 && string(chunk.Error) != "null") || kind == "error" || kind == "response.failed" || kind == "response.error" {
			failure := streamFailure(event.Data)
			body.finish(&failure)
			return
		}
		switch body.protocol {
		case UpstreamOpenAI:
			for _, choice := range chunk.Choices {
				if choice.FinishReason != "" && (!body.requireFinish || isChatFinishReason(choice.FinishReason)) {
					body.finish(&health.Failure{})
					return
				}
			}
		case UpstreamAnthropic:
			if kind == "message_stop" {
				body.finish(&health.Failure{})
				return
			}
		case UpstreamResponses:
			if kind == "response.completed" || kind == "response.incomplete" {
				body.finish(&health.Failure{})
				return
			}
		}
	}
}

func streamFailure(data string) health.Failure {
	var payload map[string]any
	_ = json.Unmarshal([]byte(data), &payload)
	info := StreamErrorObject(payload, "upstream stream failed")
	status := http.StatusBadGateway
	for _, code := range []any{info["code"], payload["code"], info["type"], payload["type"]} {
		switch code {
		case "rate_limit_error", "rate_limit_exceeded":
			status = http.StatusTooManyRequests
		case "authentication_error", "invalid_api_key":
			status = http.StatusUnauthorized
		case "permission_error", "content_policy_violation":
			status = http.StatusForbidden
		case "invalid_request_error", "context_length_exceeded":
			status = http.StatusBadRequest
		default:
			continue
		}
		break
	}
	return health.ClassifyFailure(status, []byte(data))
}
