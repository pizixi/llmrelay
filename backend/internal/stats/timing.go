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
	upstreamKeyIndex int
	upstreamKeyTotal int
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

	mu               sync.Mutex
	firstWriteAt     time.Time
	finishedAt       time.Time
	finished         bool
	pending          []usageSample
	apiKeyID         string
	apiKeyName       string
	upstreamKeyIndex int
	upstreamKeyTotal int
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

// RecordUpstreamKeySlot remembers which upstream credential was used by the
// request that owns ctx. The last successful attempt wins, so retries that
// rotate to another key report the key that actually answered.
func RecordUpstreamKeySlot(ctx context.Context, index, total int) {
	if index < 0 || total <= 0 {
		return
	}
	timing := requestTimingFromContext(ctx)
	if timing == nil {
		return
	}
	timing.mu.Lock()
	timing.upstreamKeyIndex = index + 1
	timing.upstreamKeyTotal = total
	timing.mu.Unlock()
}

type requestIdentity struct {
	apiKeyID         string
	apiKeyName       string
	upstreamKeyIndex int
	upstreamKeyTotal int
}

func (t *RequestTiming) identityLocked() requestIdentity {
	if t == nil {
		return requestIdentity{}
	}
	return requestIdentity{
		apiKeyID:         t.apiKeyID,
		apiKeyName:       t.apiKeyName,
		upstreamKeyIndex: t.upstreamKeyIndex,
		upstreamKeyTotal: t.upstreamKeyTotal,
	}
}

func (identity requestIdentity) attach(sample *usageSample) {
	if sample == nil {
		return
	}
	if sample.apiKeyID == "" {
		sample.apiKeyID = identity.apiKeyID
	}
	if sample.apiKeyName == "" {
		sample.apiKeyName = identity.apiKeyName
	}
	if sample.upstreamKeyTotal == 0 {
		sample.upstreamKeyIndex = identity.upstreamKeyIndex
		sample.upstreamKeyTotal = identity.upstreamKeyTotal
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
	identity := t.identityLocked()
	startedAt, firstByteMS, durationMS := t.metricsLocked()
	t.mu.Unlock()
	identity.attach(&sample)
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
	identity := t.identityLocked()
	startedAt, firstByteMS, durationMS := t.metricsLocked()
	t.mu.Unlock()
	requestTimingRegistry.Delete(t.id)
	for _, sample := range pending {
		identity.attach(&sample)
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
		sample.upstreamKeyIndex,
		sample.upstreamKeyTotal,
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
