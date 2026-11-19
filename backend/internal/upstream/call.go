package upstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/routing/health"
	"llmrelay/backend/internal/stats"
)

var ErrNoAvailableUpstreamKey = errors.New("all keys for this upstream model are cooling down or being probed")

// ErrEmptyUpstreamResponse marks a successful HTTP status paired with an empty
// payload. Routing treats it as a target failure instead of forwarding it.
var ErrEmptyUpstreamResponse = errors.New("upstream returned an empty response")

type upstreamCall struct {
	ctx                    context.Context
	body                   []byte
	name, model, endpoint  string
	upstream               *UpstreamConfig
	policy                 domain.ModelRoutingConfig
	target                 health.Target
	keys                   []string
	keyIndex               int
	rejected               map[int]bool
	attempts, limit        int
	retryDelay             time.Duration
	rateLimitRetriesByExit map[string]int
	raw                    bool
	requestID              string
	idleTimeout            time.Duration
	backup                 bool
}

type upstreamReply struct {
	body    io.ReadCloser
	data    []byte
	status  int
	headers http.Header
}

// ResponseError retains the classification of an upstream HTTP error before
// its body is translated into the client's protocol.
type ResponseError struct {
	Failure health.Failure
	cause   error
}

func (err *ResponseError) Error() string { return err.cause.Error() }

func (err *ResponseError) Unwrap() error { return err.cause }

func newUpstreamCall(ctx context.Context, body []byte, name, model string, upstream *UpstreamConfig, raw []bool) *upstreamCall {
	keys := GetUpstreamAPIKeys(upstream)
	policy, tracked := ctx.Value(routingPolicyContextKey{}).(domain.ModelRoutingConfig)
	policy.DisableCooldown = !tracked || policy.DisableCooldown
	call := &upstreamCall{
		ctx: ctx, body: body, name: name, model: model, upstream: upstream,
		endpoint: GetUpstreamEndpoint(upstream), policy: policy, keys: keys,
		keyIndex: NextUpstreamAPIKeyIndex(name, len(keys)),
		limit:    UpstreamAttemptLimit(upstream, len(keys)), retryDelay: time.Second,
		raw: len(raw) > 0 && raw[0],
	}
	if !policy.DisableCooldown {
		call.target = health.NewTarget(name, model, upstream)
	}
	return call
}

func (call *upstreamCall) chooseKey(acquire bool) (string, int, *health.KeyAttempt, bool) {
	count := len(call.keys)
	for offset := range max(count, 1) {
		index, key := -1, ""
		if count > 0 {
			index = (call.keyIndex + offset) % count
			key = call.keys[index]
		}
		if call.rejected[index] {
			continue
		}
		if call.policy.DisableCooldown {
			return key, index, nil, true
		}
		if acquire {
			attempt, _ := call.target.TryKey(key, call.policy)
			if attempt != nil {
				return key, index, attempt, true
			}
		} else if available, _ := call.target.Available([]string{key}); available {
			return key, index, nil, true
		}
	}
	return "", -1, nil, false
}

func (call *upstreamCall) completeKey(attempt *health.KeyAttempt, slot string, outcome *health.Failure) {
	if call.ctx.Err() != nil {
		outcome = nil
	}
	if cooldown := attempt.Complete(outcome); cooldown > 0 {
		log.Printf("[模型 Key 冷却] 上游=%s 模型=%s 密钥=%s 分类=%s 冷却=%s", EffectiveUpstreamName(call.name), call.model, slot, outcome.Kind, cooldown)
	}
}

func (call *upstreamCall) rotateKey() {
	if len(call.keys) > 1 {
		call.keyIndex = (call.keyIndex + 1) % len(call.keys)
	}
}

func (call *upstreamCall) waitToRetry(retryAfter string) (bool, error) {
	if call.attempts >= call.limit {
		return false, nil
	}
	previous := call.keyIndex
	call.rotateKey()
	_, next, _, found := call.chooseKey(false)
	if !found {
		return false, nil
	}
	delay := call.retryDelay
	if next != previous && strings.TrimSpace(retryAfter) == "" {
		delay = min(delay, 300*time.Millisecond)
	}
	if err := WaitForRetry(call.ctx, delay, retryAfter); err != nil {
		return false, err
	}
	call.retryDelay = NextRetryDelay(call.retryDelay)
	return true, nil
}

func (call *upstreamCall) open() (upstreamReply, error) {
	var lastReply upstreamReply
	var lastErr error
	stream := call.requestID != ""
	for call.attempts < call.limit {
		if err := call.ctx.Err(); err != nil {
			return upstreamReply{}, err
		}
		apiKey, keyIndex, keyAttempt, found := call.chooseKey(true)
		if !found {
			if lastErr != nil {
				return lastReply, lastErr
			}
			return upstreamReply{status: http.StatusServiceUnavailable}, ErrNoAvailableUpstreamKey
		}
		call.keyIndex = keyIndex
		call.attempts++
		slot := FormatUpstreamAPIKeySlot(keyIndex, len(call.keys))
		up, err := BuildUpstreamRequest(call.endpoint, apiKey, call.body, call.upstream)
		if err != nil {
			keyAttempt.Complete(nil)
			return upstreamReply{status: http.StatusInternalServerError}, fmt.Errorf("build upstream request: %w", err)
		}
		up = up.WithContext(call.ctx)
		ApplyNativeProtocolHeadersFromContext(up)
		if call.upstream.APIType == UpstreamAnthropic {
			ApplyAnthropicProtocolHeadersFromContext(up)
		}
		start := time.Now()
		client, exit := GetHTTPClientForUpstream(stream, call.upstream)
		var resp *http.Response
		if stream {
			resp, err = call.openStreamResponse(client, up, start, slot, exit)
		} else {
			resp, err = client.Do(up)
		}
		if err != nil {
			lastReply = upstreamReply{status: http.StatusBadGateway}
			lastErr = fmt.Errorf("upstream request failed after %d attempt(s): %w", call.attempts, err)
			failure := health.Failure{Kind: "connection", Cooldown: true}
			call.completeKey(keyAttempt, slot, &failure)
			if err := call.ctx.Err(); err != nil {
				return upstreamReply{}, err
			}
			if stream {
				log.Printf("[上游流连接失败] 请求=%s 上游=%s 模型=%s 密钥=%s 出口=%s 尝试=%d/%d 错误=%v",
					call.requestID, EffectiveUpstreamName(call.name), call.model, slot, exit, call.attempts, call.limit, err)
			}
			if retry, waitErr := call.waitToRetry(""); retry {
				continue
			} else if waitErr != nil {
				return upstreamReply{}, waitErr
			}
			return lastReply, lastErr
		}
		reply := upstreamReply{status: resp.StatusCode, headers: resp.Header}
		success := resp.StatusCode >= 200 && resp.StatusCode < 300
		if success && stream {
			reply.body = resp.Body
			if keyAttempt != nil {
				reply.body = newKeyStreamBody(resp.Body, call.upstream.APIType, call.backup || chatStreamReplayEnabled(call.ctx), func(outcome *health.Failure) {
					call.completeKey(keyAttempt, slot, outcome)
				})
			}
			return reply, nil
		}
		if success {
			log.Printf("[首字节耗时] 上游=%s 模型=%s 密钥=%s 出口=%s 耗时=%s", EffectiveUpstreamName(call.name), call.model, slot, exit, time.Since(start).Round(time.Millisecond))
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		failure := health.ClassifyFailure(resp.StatusCode, data)
		if success {
			if readErr != nil {
				failure = health.Failure{Kind: "response_read", Cooldown: true}
				call.completeKey(keyAttempt, slot, &failure)
				return upstreamReply{status: http.StatusBadGateway}, readErr
			}
			// An empty 2xx body is not a usable model response and usually means
			// a gateway or proxy swallowed the payload. Cool the credential and
			// let the caller fail over to another target.
			if len(bytes.TrimSpace(data)) == 0 {
				failure = health.Failure{Kind: "empty_response", Cooldown: true}
				call.completeKey(keyAttempt, slot, &failure)
				return upstreamReply{status: http.StatusBadGateway, headers: resp.Header}, ErrEmptyUpstreamResponse
			}
			call.completeKey(keyAttempt, slot, &failure)
			stats.RecordUpstreamKeySlot(call.ctx, keyIndex, len(call.keys))
			reply.data = data
			return reply, nil
		}
		call.completeKey(keyAttempt, slot, &failure)
		if !call.raw {
			data = MapUpstreamErrorBody(data, call.upstream.APIType)
		}
		reply.data = data
		if readErr != nil {
			return reply, &ResponseError{Failure: failure, cause: fmt.Errorf("read upstream error response: %w", readErr)}
		}
		lastReply, lastErr = reply, &ResponseError{Failure: failure, cause: fmt.Errorf("upstream returned HTTP %d", reply.status)}
		if ShouldRetryUpstreamStatus(reply.status) {
			retryAfter := reply.headers.Get("Retry-After")
			log.Printf("[上游重试] 上游=%s 模型=%s 密钥=%s 出口=%s 状态码=%d Retry-After=%q", EffectiveUpstreamName(call.name), call.model, slot, exit, reply.status, retryAfter)
			if debugMode && debugLogBodies {
				log.Printf("[上游重试响应体] %s", TruncatePreview(string(data), 1024))
			}
			if reply.status == http.StatusTooManyRequests {
				if call.rateLimitRetriesByExit[exit] >= max429RetriesPerExit {
					return reply, &ResponseError{Failure: failure, cause: fmt.Errorf("upstream remained rate limited on exit %s after %d retries", exit, max429RetriesPerExit)}
				}
				if call.rateLimitRetriesByExit == nil {
					call.rateLimitRetriesByExit = make(map[string]int)
				}
				call.rateLimitRetriesByExit[exit]++
			}
			if retry, waitErr := call.waitToRetry(retryAfter); retry {
				continue
			} else if waitErr != nil {
				return upstreamReply{}, waitErr
			}
			return reply, lastErr
		}
		// These failures can vary by key, even for the same upstream model.
		if len(call.keys) > 1 && (failure.Kind == "authentication_or_quota" || failure.Kind == "permission" ||
			(!call.policy.DisableCooldown && failure.Cooldown && failure.Failover)) {
			if call.rejected == nil {
				call.rejected = make(map[int]bool)
			}
			call.rejected[keyIndex] = true
			call.rotateKey()
			continue
		}
		log.Printf("[上游错误] 上游=%s 模型=%s 密钥=%s 状态码=%d 端点=%s", EffectiveUpstreamName(call.name), call.model, slot, reply.status, call.endpoint)
		return reply, lastErr
	}
	return lastReply, lastErr
}

func (call *upstreamCall) openStreamResponse(client *http.Client, request *http.Request, start time.Time, slot, exit string) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(request.Context())
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		cancel(nil)
		return nil, err
	}
	response.Body = &ttfbReadCloser{
		inner: response.Body, ctx: ctx, cancel: cancel, idleTimeout: call.idleTimeout,
		start: start, protocol: response.Proto, contentLength: response.ContentLength, uncompressed: response.Uncompressed,
		requestID: fmt.Sprintf("%s/%d", call.requestID, call.attempts),
		upstream:  EffectiveUpstreamName(call.name), model: call.model, keySlot: slot, exitLabel: exit,
	}
	return response, nil
}

func CallPreparedUpstream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawResponse ...bool) ([]byte, int, http.Header, error) {
	if upstream == nil || upstream.BaseURL == "" {
		return nil, http.StatusInternalServerError, nil, errors.New("upstream not configured")
	}
	call := newUpstreamCall(ctx, preparedBody, upstreamName, modelID, upstream, rawResponse)
	reply, err := call.open()
	data := reply.data
	if err == nil && !call.raw {
		if upstream.APIType == UpstreamResponses {
			data = ConvertResponsesToChat(data, modelID)
		} else if IsAnthropicFormat(data) {
			data = ConvertAnthropicToOpenAI(data, modelID)
		}
	}
	return data, reply.status, reply.headers, err
}

func CallUpstream(ctx context.Context, reqBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawResponse ...bool) ([]byte, int, http.Header, error) {
	tryBody, err := PrepareOpenAIUpstreamBody(reqBody, modelID, upstream)
	if err != nil {
		return nil, http.StatusBadRequest, nil, err
	}
	return CallPreparedUpstream(ctx, tryBody, upstreamName, modelID, upstream, rawResponse...)
}

func CallPreparedUpstreamStream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, rawErrors ...bool) (io.ReadCloser, int, http.Header, error) {
	return callPreparedUpstreamStream(ctx, preparedBody, upstreamName, modelID, upstream, upstreamStreamIdleTimeout, rawErrors...)
}

func callPreparedUpstreamStream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, idleTimeout time.Duration, rawErrors ...bool) (io.ReadCloser, int, http.Header, error) {
	body, reply, err := openRetryStream(ctx, preparedBody, upstreamName, modelID, upstream, idleTimeout, false, rawErrors)
	if err != nil {
		return reply.body, reply.status, reply.headers, err
	}
	fallback := chatStreamFallbackFromContext(ctx)
	if upstream.APIType == UpstreamOpenAI && chatStreamReplayEnabled(ctx) {
		replay := newChatReplayStream(body)
		replay.canReplay = body.attemptLimit > 1 || fallback != nil
		replay.fallback = fallback
		return replay, reply.status, reply.headers, nil
	}
	return body, reply.status, reply.headers, nil
}

func openRetryStream(ctx context.Context, preparedBody []byte, upstreamName, modelID string, upstream *UpstreamConfig, idleTimeout time.Duration, backup bool, rawErrors []bool) (*retryStreamBody, upstreamReply, error) {
	if upstream == nil || upstream.BaseURL == "" {
		return nil, upstreamReply{status: http.StatusInternalServerError}, errors.New("upstream not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	call := newUpstreamCall(ctx, preparedBody, upstreamName, modelID, upstream, rawErrors)
	call.requestID, call.idleTimeout, call.backup = RandomString(12), idleTimeout, backup
	reply, err := call.open()
	if err != nil {
		cancel()
		if reply.data != nil {
			reply.body = io.NopCloser(bytes.NewReader(reply.data))
		}
		return nil, reply, err
	}
	retrying := &retryStreamBody{ctx: ctx, cancel: cancel, current: reply.body, requestID: call.requestID, attemptLimit: call.limit}
	retrying.reopen = func(cause error) (io.ReadCloser, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		retry, err := call.waitToRetry("")
		if err != nil {
			return nil, err
		}
		if !retry {
			return nil, cause
		}
		log.Printf("[上游流重试] 请求=%s 上游=%s 模型=%s 尝试=%d/%d 原因=%v",
			call.requestID, EffectiveUpstreamName(upstreamName), modelID, call.attempts+1, call.limit, cause)
		next, err := call.open()
		return next.body, err
	}
	if backup {
		retrying.onVerified = func() { stats.RecordRecoveredStreamRoute(ctx, upstreamName, modelID, call.keyIndex, len(call.keys)) }
	} else {
		retrying.onVerified = func() { stats.RecordUpstreamKeySlot(ctx, call.keyIndex, len(call.keys)) }
		retrying.onVerified()
	}
	return retrying, reply, nil
}

func CallUpstreamStream(ctx context.Context, reqBody []byte, upstreamName, modelID string, upstream *UpstreamConfig) (io.ReadCloser, int, http.Header, error) {
	tryBody, err := PrepareOpenAIUpstreamBody(reqBody, modelID, upstream)
	if err != nil {
		return nil, http.StatusBadRequest, nil, err
	}
	return CallPreparedUpstreamStream(ctx, tryBody, upstreamName, modelID, upstream)
}

// OpenChatStreamBackup uses the backup's retry budget and defers usage attribution
// until the original converter has verified its prefix.
func OpenChatStreamBackup(ctx context.Context, chatBody []byte, name, model string, target *UpstreamConfig) (*ChatStreamBackup, int, error) {
	body, err := PrepareOpenAIUpstreamBody(chatBody, model, target)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	stream, reply, err := openRetryStream(ctx, body, name, model, target, upstreamStreamIdleTimeout, true, nil)
	if err != nil {
		if reply.body != nil {
			_ = reply.body.Close()
		}
		return nil, reply.status, err
	}
	return &ChatStreamBackup{stream: stream}, reply.status, nil
}
