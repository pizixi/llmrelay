package stats

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"llmrelay/backend/internal/auth"
)

type requestTimingContextKey struct{}

type usageSample struct {
	model            string
	upstreamName     string
	upstreamModel    string
	apiKeyID         string
	apiKeyName       string
	promptTokens     int64
	cachedTokens     int64
	completionTokens int64
	totalTokens      int64
}

// RequestTiming follows one public API request from handler entry through the
// first response body write and final usage commit.
type RequestTiming struct {
	id        uint64
	startedAt time.Time

	mu           sync.Mutex
	firstWriteAt time.Time
	finishedAt   time.Time
	finished     bool
	pending      []usageSample
	apiKeyID     string
	apiKeyName   string
}

var (
	requestTimingSequence atomic.Uint64
	requestTimingRegistry sync.Map
)

func newRequestTiming(ctx context.Context) *RequestTiming {
	timing := &RequestTiming{
		id:        requestTimingSequence.Add(1),
		startedAt: time.Now(),
	}
	if identity, ok := auth.APIKeyFromContext(ctx); ok {
		timing.apiKeyID = identity.ID
		timing.apiKeyName = identity.Name
	}
	requestTimingRegistry.Store(timing.id, timing)
	return timing
}

func (t *RequestTiming) attachAPIKey(sample *usageSample) {
	if t == nil || sample == nil {
		return
	}
	if sample.apiKeyID == "" {
		sample.apiKeyID = t.apiKeyID
	}
	if sample.apiKeyName == "" {
		sample.apiKeyName = t.apiKeyName
	}
}

func requestTimingFromContext(ctx context.Context) *RequestTiming {
	if ctx == nil {
		return nil
	}
	timing, _ := ctx.Value(requestTimingContextKey{}).(*RequestTiming)
	return timing
}

func requestTimingByID(id uint64) *RequestTiming {
	if id == 0 {
		return nil
	}
	value, ok := requestTimingRegistry.Load(id)
	if !ok {
		return nil
	}
	timing, _ := value.(*RequestTiming)
	return timing
}

func (t *RequestTiming) markFirstWrite() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.firstWriteAt.IsZero() {
		t.firstWriteAt = time.Now()
	}
	t.mu.Unlock()
}

func (t *RequestTiming) addUsage(sample usageSample) {
	if t == nil {
		recordUsageSample(sample, time.Now(), 0, 0)
		return
	}
	t.mu.Lock()
	if !t.finished {
		t.pending = append(t.pending, sample)
		t.mu.Unlock()
		return
	}
	t.attachAPIKey(&sample)
	startedAt, firstByteMS, durationMS := t.metricsLocked()
	t.mu.Unlock()
	recordUsageSample(sample, startedAt, firstByteMS, durationMS)
}

func (t *RequestTiming) metricsLocked() (time.Time, int64, int64) {
	finishedAt := t.finishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	durationMS := max(finishedAt.Sub(t.startedAt).Milliseconds(), 0)
	firstByteMS := int64(0)
	if !t.firstWriteAt.IsZero() {
		firstByteMS = max(t.firstWriteAt.Sub(t.startedAt).Milliseconds(), 0)
		if firstByteMS > durationMS {
			durationMS = firstByteMS
		}
	}
	return t.startedAt, firstByteMS, durationMS
}

func (t *RequestTiming) finish() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	t.finishedAt = time.Now()
	pending := append([]usageSample(nil), t.pending...)
	t.pending = nil
	startedAt, firstByteMS, durationMS := t.metricsLocked()
	t.mu.Unlock()
	requestTimingRegistry.Delete(t.id)
	for _, sample := range pending {
		t.attachAPIKey(&sample)
		recordUsageSample(sample, startedAt, firstByteMS, durationMS)
	}
}

func recordUsageSample(sample usageSample, calledAt time.Time, firstByteMS, durationMS int64) {
	recordUsageWithAPIKey(
		sample.model,
		sample.upstreamName,
		sample.upstreamModel,
		sample.promptTokens,
		sample.cachedTokens,
		sample.completionTokens,
		sample.totalTokens,
		calledAt,
		firstByteMS,
		durationMS,
		sample.apiKeyID,
		sample.apiKeyName,
	)
}

type timingResponseWriter struct {
	http.ResponseWriter
	timing *RequestTiming
}

func (w *timingResponseWriter) Write(body []byte) (int, error) {
	if len(body) > 0 {
		w.timing.markFirstWrite()
	}
	return w.ResponseWriter.Write(body)
}

func (w *timingResponseWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *timingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// TrackRequest measures successful usage records without coupling protocol
// adapters to HTTP response timing details.
func TrackRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timing := newRequestTiming(r.Context())
		defer timing.finish()
		ctx := context.WithValue(r.Context(), requestTimingContextKey{}, timing)
		next(&timingResponseWriter{ResponseWriter: w, timing: timing}, r.WithContext(ctx))
	}
}
