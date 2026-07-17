package websearch

import "time"

var (
	ErrRequestTimeout = errWebSearchRequestTimeout
	ErrRequestFailed  = errWebSearchRequestFailed
)

func NewFallbackProvider(primary, fallback Provider, budget time.Duration) Provider {
	return &fallbackWebSearchProvider{primary: primary, fallback: fallback, primaryBudget: budget}
}

func NewAutoSearxngProvider(directoryURL, fallbackURL string) Provider {
	return &autoSearxngSearchProvider{directoryURL: directoryURL, fallbackURL: fallbackURL}
}

func ResetDuckDuckGoRateLimit() {
	duckDuckGoSearchRate.Lock()
	duckDuckGoSearchRate.last = time.Time{}
	duckDuckGoSearchRate.Unlock()
}
