package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	searxngDirectoryCacheTTL    = 15 * time.Minute
	searxngMaxDirectoryBytes    = 16 << 20
	searxngMaxAutoAttempts      = 8
	searxngPerInstanceTimeout   = 3 * time.Second
	searxngMinimumSearchSuccess = 80.0
	searxngFailureCooldown      = 10 * time.Minute
)

// SearXNGPublicInstance 是清理后提供给管理界面的稳定视图。
// searx.space 完整文档包含选择时不需要的引擎及证书详情，
// 直接返回会使管理接口响应过大。
type SearXNGPublicInstance struct {
	URL           string  `json:"url"`
	Version       string  `json:"version,omitempty"`
	HTTPGrade     string  `json:"http_grade,omitempty"`
	TLSGrade      string  `json:"tls_grade,omitempty"`
	SearchSuccess float64 `json:"search_success"`
	SearchMedian  float64 `json:"search_median_seconds"`
	UptimeDay     float64 `json:"uptime_day"`
	UptimeMonth   float64 `json:"uptime_month"`
	Analytics     bool    `json:"analytics"`
	QualityScore  float64 `json:"quality_score"`
	AutoEligible  bool    `json:"auto_eligible"`
}

type searxngDirectoryEntry struct {
	FetchedAt time.Time
	Instances []SearXNGPublicInstance
}

var searxngDirectoryState = struct {
	sync.Mutex
	Entries   map[string]searxngDirectoryEntry
	Preferred map[string]string
	Failures  map[string]map[string]time.Time
}{Entries: map[string]searxngDirectoryEntry{}, Preferred: map[string]string{}, Failures: map[string]map[string]time.Time{}}

type searxngDirectoryDocument struct {
	Instances map[string]struct {
		Analytics   bool   `json:"analytics"`
		Main        bool   `json:"main"`
		NetworkType string `json:"network_type"`
		Generator   string `json:"generator"`
		Version     string `json:"version"`
		HTTP        struct {
			StatusCode int    `json:"status_code"`
			Grade      string `json:"grade"`
		} `json:"http"`
		TLS struct {
			Grade string `json:"grade"`
		} `json:"tls"`
		Timing struct {
			Search struct {
				SuccessPercentage float64 `json:"success_percentage"`
				All               struct {
					Median float64 `json:"median"`
					Mean   float64 `json:"mean"`
				} `json:"all"`
			} `json:"search"`
		} `json:"timing"`
		Uptime struct {
			Day   float64 `json:"uptimeDay"`
			Week  float64 `json:"uptimeWeek"`
			Month float64 `json:"uptimeMonth"`
		} `json:"uptime"`
	} `json:"instances"`
}

func FetchSearXNGDirectory(ctx context.Context, directoryURL string) ([]SearXNGPublicInstance, error) {
	req, err := http.NewRequest(http.MethodGet, directoryURL, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", applicationName+"/searxng-directory")
	resp, err := GetHTTPClient(false).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, searxngMaxDirectoryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > searxngMaxDirectoryBytes {
		return nil, fmt.Errorf("SearXNG directory response exceeded %d bytes", searxngMaxDirectoryBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SearXNG directory returned HTTP %d", resp.StatusCode)
	}
	var document searxngDirectoryDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("decode SearXNG directory: %w", err)
	}
	instances := make([]SearXNGPublicInstance, 0, len(document.Instances))
	for rawURL, item := range document.Instances {
		parsed, parseErr := url.Parse(strings.TrimSpace(rawURL))
		median := item.Timing.Search.All.Median
		if median <= 0 {
			median = item.Timing.Search.All.Mean
		}
		loopbackHTTP := parsed.Scheme == "http" && (strings.EqualFold(parsed.Hostname(), "localhost") || net.ParseIP(parsed.Hostname()).IsLoopback())
		if parseErr != nil || (parsed.Scheme != "https" && !loopbackHTTP) || parsed.Hostname() == "" || parsed.User != nil ||
			!item.Main || item.NetworkType != "normal" || item.HTTP.StatusCode != http.StatusOK ||
			!strings.EqualFold(item.Generator, "searxng") {
			continue
		}
		autoEligible := item.Timing.Search.SuccessPercentage >= searxngMinimumSearchSuccess && median > 0 && median <= 10 && item.Uptime.Day >= 90
		quality := 100.0
		if median > 0 {
			quality -= median * 5
		} else {
			quality -= 25
		}
		quality -= (100 - item.Timing.Search.SuccessPercentage) * 0.35
		quality -= (100 - item.Uptime.Day) * 0.15
		if item.Analytics {
			quality -= 8
		}
		if item.HTTP.Grade != "A+" && item.HTTP.Grade != "A" {
			quality -= 5
		}
		if item.TLS.Grade != "A+" && item.TLS.Grade != "A" {
			quality -= 5
		}
		instances = append(instances, SearXNGPublicInstance{
			URL: strings.TrimRight(parsed.String(), "/") + "/", Version: item.Version,
			HTTPGrade: item.HTTP.Grade, TLSGrade: item.TLS.Grade,
			SearchSuccess: item.Timing.Search.SuccessPercentage, SearchMedian: median,
			UptimeDay: item.Uptime.Day, UptimeMonth: item.Uptime.Month,
			Analytics: item.Analytics, QualityScore: quality, AutoEligible: autoEligible,
		})
	}
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].AutoEligible != instances[j].AutoEligible {
			return instances[i].AutoEligible
		}
		if instances[i].QualityScore != instances[j].QualityScore {
			return instances[i].QualityScore > instances[j].QualityScore
		}
		if instances[i].SearchMedian != instances[j].SearchMedian {
			return instances[i].SearchMedian < instances[j].SearchMedian
		}
		return instances[i].URL < instances[j].URL
	})
	if len(instances) == 0 {
		return nil, fmt.Errorf("SearXNG directory contained no available public instances")
	}
	return instances, nil
}

func ListSearXNGPublicInstances(ctx context.Context, directoryURL string, forceRefresh bool) ([]SearXNGPublicInstance, time.Time, error) {
	directoryURL = strings.TrimSpace(directoryURL)
	if directoryURL == "" {
		directoryURL = defaultSearXNGDirectoryURL
	}
	now := time.Now()
	searxngDirectoryState.Lock()
	cached, exists := searxngDirectoryState.Entries[directoryURL]
	searxngDirectoryState.Unlock()
	if !forceRefresh && exists && now.Sub(cached.FetchedAt) < searxngDirectoryCacheTTL {
		return append([]SearXNGPublicInstance(nil), cached.Instances...), cached.FetchedAt, nil
	}
	instances, err := FetchSearXNGDirectory(ctx, directoryURL)
	if err != nil {
		if exists && len(cached.Instances) > 0 {
			return append([]SearXNGPublicInstance(nil), cached.Instances...), cached.FetchedAt, nil
		}
		return nil, time.Time{}, err
	}
	entry := searxngDirectoryEntry{FetchedAt: now, Instances: append([]SearXNGPublicInstance(nil), instances...)}
	searxngDirectoryState.Lock()
	searxngDirectoryState.Entries[directoryURL] = entry
	searxngDirectoryState.Unlock()
	return instances, now, nil
}

type autoSearxngSearchProvider struct {
	directoryURL string
	fallbackURL  string
}

type SearxngSearchFailureSummary struct {
	Attempted        int
	RateLimited      int
	TimedOut         int
	Other            int
	DeadlineExceeded bool
}

func (e *SearxngSearchFailureSummary) Error() string {
	return fmt.Sprintf(
		"SearXNG auto search failed (attempted=%d rate_limited=%d timed_out=%d other=%d deadline_exceeded=%t)",
		e.Attempted, e.RateLimited, e.TimedOut, e.Other, e.DeadlineExceeded,
	)
}

func (p *autoSearxngSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	directoryCtx, directoryCancel := context.WithTimeout(ctx, searxngPerInstanceTimeout)
	instances, _, directoryErr := ListSearXNGPublicInstances(directoryCtx, p.directoryURL, false)
	directoryCancel()
	candidates := make([]string, 0, searxngMaxAutoAttempts+1)
	seen := map[string]bool{}
	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	searxngDirectoryState.Lock()
	preferred := searxngDirectoryState.Preferred[p.directoryURL]
	failureTimes := make(map[string]time.Time, len(searxngDirectoryState.Failures[p.directoryURL]))
	for candidate, failedAt := range searxngDirectoryState.Failures[p.directoryURL] {
		failureTimes[candidate] = failedAt
	}
	searxngDirectoryState.Unlock()
	isCoolingDown := func(candidate string) bool {
		failedAt, failed := failureTimes[candidate]
		return failed && time.Since(failedAt) < searxngFailureCooldown
	}
	if !isCoolingDown(preferred) {
		appendCandidate(preferred)
	}
	for _, instance := range instances {
		if len(candidates) >= searxngMaxAutoAttempts {
			break
		}
		if !instance.AutoEligible {
			continue
		}
		if isCoolingDown(instance.URL) {
			continue
		}
		appendCandidate(instance.URL)
	}
	if !isCoolingDown(p.fallbackURL) {
		appendCandidate(p.fallbackURL)
	}
	if len(candidates) == 0 {
		if directoryErr != nil {
			return nil, directoryErr
		}
		return nil, fmt.Errorf("no healthy SearXNG instance is available")
	}
	summary := &SearxngSearchFailureSummary{}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			summary.DeadlineExceeded = true
			break
		}
		attemptTimeout := searxngPerInstanceTimeout
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				summary.DeadlineExceeded = true
				break
			}
			if remaining < attemptTimeout {
				attemptTimeout = remaining
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		results, err := (&searxngSearchProvider{baseURL: candidate}).Search(attemptCtx, query, maxResults)
		cancel()
		summary.Attempted++
		if err == nil {
			searxngDirectoryState.Lock()
			searxngDirectoryState.Preferred[p.directoryURL] = candidate
			if directoryFailures := searxngDirectoryState.Failures[p.directoryURL]; directoryFailures != nil {
				delete(directoryFailures, candidate)
			}
			searxngDirectoryState.Unlock()
			return results, nil
		}
		var statusError *webSearchHTTPStatusError
		switch {
		case errors.As(err, &statusError) && statusError.StatusCode == http.StatusTooManyRequests:
			summary.RateLimited++
		case errors.Is(err, errWebSearchRequestTimeout), errors.Is(err, context.DeadlineExceeded):
			summary.TimedOut++
		default:
			summary.Other++
		}
		searxngDirectoryState.Lock()
		if searxngDirectoryState.Failures[p.directoryURL] == nil {
			searxngDirectoryState.Failures[p.directoryURL] = map[string]time.Time{}
		}
		searxngDirectoryState.Failures[p.directoryURL][candidate] = time.Now()
		if searxngDirectoryState.Preferred[p.directoryURL] == candidate {
			delete(searxngDirectoryState.Preferred, p.directoryURL)
		}
		searxngDirectoryState.Unlock()
		if ctx.Err() != nil {
			summary.DeadlineExceeded = true
			break
		}
	}
	return nil, summary
}

func SearxngInstancesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := NormalizeWebSearchConfig(GetWebSearchConfig())
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	instances, fetchedAt, err := ListSearXNGPublicInstances(ctx, cfg.SearXNGDirectoryURL, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"directory_url": cfg.SearXNGDirectoryURL,
		"fetched_at":    fetchedAt.UTC().Format(time.RFC3339),
		"instances":     instances,
	})
}
