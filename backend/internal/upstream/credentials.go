package upstream

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const max429RetriesPerExit = 3

var (
	upstreamKeyCursorMu sync.Mutex
	upstreamKeyCursor   = map[string]int{}
)

func SplitUpstreamAPIKeys(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, strings.Count(raw, "\n")+1)
	for _, line := range strings.Split(raw, "\n") {
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func GetUpstreamAPIKeys(upstream *UpstreamConfig) []string {
	if upstream == nil {
		return nil
	}
	return SplitUpstreamAPIKeys(upstream.APIKey)
}

func NextUpstreamAPIKeyIndex(name string, total int) int {
	if total <= 1 {
		return 0
	}
	resolvedName := EffectiveUpstreamName(name)
	upstreamKeyCursorMu.Lock()
	defer upstreamKeyCursorMu.Unlock()
	idx := upstreamKeyCursor[resolvedName] % total
	upstreamKeyCursor[resolvedName] = (idx + 1) % total
	return idx
}

func SelectUpstreamAPIKey(name string, upstream *UpstreamConfig) (string, int, []string) {
	keys := GetUpstreamAPIKeys(upstream)
	if len(keys) == 0 {
		return "", -1, nil
	}
	idx := NextUpstreamAPIKeyIndex(name, len(keys))
	return keys[idx], idx, keys
}

func RotateUpstreamAPIKey(keys []string, current int) (string, int) {
	if len(keys) == 0 {
		return "", -1
	}
	if current < 0 {
		current = 0
	}
	next := (current + 1) % len(keys)
	return keys[next], next
}

func FormatUpstreamAPIKeySlot(index int, total int) string {
	if index < 0 || total <= 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", index+1, total)
}

func ParseRetryAfterDelay(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(raw)
	if err != nil {
		return 0
	}
	delay := time.Until(when)
	if delay < 0 {
		return 0
	}
	return delay
}

func WaitForRetry(ctx context.Context, baseDelay time.Duration, retryAfter string) error {
	delay := baseDelay
	if headerDelay := ParseRetryAfterDelay(retryAfter); headerDelay > delay {
		delay = headerDelay
	}
	if delay <= 0 {
		delay = time.Second
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func NextRetryDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return time.Second
	}
	if delay >= 30*time.Second {
		return 30 * time.Second
	}
	delay *= 2
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func ShouldRetryUpstreamStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func UpstreamAttemptLimit(upstream *UpstreamConfig, keyCount int) int {
	const defaultRetries = 3
	retries := defaultRetries
	explicit := upstream != nil && upstream.MaxRetries != nil
	if explicit {
		retries = *upstream.MaxRetries
	}
	if retries < 0 {
		retries = 0
	}
	if retries > 10 {
		retries = 10
	}
	attempts := retries + 1
	// 默认让每个已配置密钥都有一次机会。显式 max_retries 始终优先，
	// 包括用零禁用重试的情况。
	if !explicit && keyCount > attempts {
		attempts = keyCount
	}
	return attempts
}
