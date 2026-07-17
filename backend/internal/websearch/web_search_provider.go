package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	errWebSearchRequestTimeout = errors.New("search provider request timed out")
	errWebSearchRequestFailed  = errors.New("search provider request failed")
)

type webSearchHTTPStatusError struct {
	StatusCode int
}

func (e *webSearchHTTPStatusError) Error() string {
	return fmt.Sprintf("search provider returned HTTP %d", e.StatusCode)
}

const (
	defaultWebSearchMaxResults     = 6
	defaultWebSearchTimeoutSeconds = 10
	defaultWebSearchMaxToolRounds  = 2
	defaultWebSearchMaxResultBytes = 64 << 10
	maxWebSearchResponseBytes      = 2 << 20
	defaultSearXNGDirectoryURL     = "https://searx.space/data/instances.json"
	defaultDuckDuckGoEndpoint      = "https://lite.duckduckgo.com/lite/"
	webSearchFallbackNone          = "none"
	webSearchProviderDuckDuckGo    = "duckduckgo"
	searxngModeAuto                = "auto"
	searxngModeSelected            = "selected"
	searxngModeCustom              = "custom"
)

type Result struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score,omitempty"`
}

type SearchResponse struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

type Provider interface {
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

func NormalizeWebSearchConfig(cfg WebSearchConfig) WebSearchConfig {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.FallbackProvider = strings.ToLower(strings.TrimSpace(cfg.FallbackProvider))
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.SearXNGMode = strings.ToLower(strings.TrimSpace(cfg.SearXNGMode))
	cfg.SearXNGDirectoryURL = strings.TrimSpace(cfg.SearXNGDirectoryURL)
	if cfg.SearXNGMode == "" {
		if cfg.BaseURL == "" {
			cfg.SearXNGMode = searxngModeAuto
		} else {
			// 向后兼容：升级后，已有的 SearXNG base_url 仍作为固定的自定义端点使用。
			cfg.SearXNGMode = searxngModeCustom
		}
	}
	if cfg.SearXNGDirectoryURL == "" {
		cfg.SearXNGDirectoryURL = defaultSearXNGDirectoryURL
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = defaultWebSearchMaxResults
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = defaultWebSearchTimeoutSeconds
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = defaultWebSearchMaxToolRounds
	}
	if cfg.MaxResultBytes <= 0 {
		cfg.MaxResultBytes = defaultWebSearchMaxResultBytes
	}
	if cfg.Provider == "tavily" && cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.tavily.com/search"
	}
	if cfg.Provider == webSearchProviderDuckDuckGo && cfg.BaseURL == "" {
		cfg.BaseURL = defaultDuckDuckGoEndpoint
	}
	if cfg.FallbackProvider == "" {
		if cfg.Provider == "searxng" && cfg.SearXNGMode == searxngModeAuto {
			cfg.FallbackProvider = webSearchProviderDuckDuckGo
		} else {
			cfg.FallbackProvider = webSearchFallbackNone
		}
	}
	return cfg
}

func ValidateWebSearchConfig(raw WebSearchConfig) error {
	if !raw.Enabled {
		return nil
	}
	cfg := NormalizeWebSearchConfig(raw)
	switch cfg.Provider {
	case "searxng":
		switch cfg.SearXNGMode {
		case searxngModeAuto:
		case searxngModeSelected, searxngModeCustom:
			if cfg.BaseURL == "" {
				return fmt.Errorf("web_search.base_url is required for SearXNG mode %q", cfg.SearXNGMode)
			}
		default:
			return fmt.Errorf("web_search.searxng_mode must be auto, selected, or custom")
		}
	case "tavily":
		if cfg.APIKey == "" {
			return fmt.Errorf("web_search.api_key is required for provider %q", cfg.Provider)
		}
	case webSearchProviderDuckDuckGo:
	default:
		return fmt.Errorf("web_search.provider must be searxng, duckduckgo, or tavily")
	}
	if cfg.FallbackProvider != webSearchFallbackNone && cfg.FallbackProvider != webSearchProviderDuckDuckGo {
		return fmt.Errorf("web_search.fallback_provider must be none or duckduckgo")
	}
	if cfg.FallbackProvider != webSearchFallbackNone && (cfg.Provider != "searxng" || cfg.SearXNGMode != searxngModeAuto) {
		return fmt.Errorf("web_search.fallback_provider is only supported for automatic SearXNG")
	}
	if cfg.BaseURL != "" {
		if err := ValidateWebSearchURL(cfg.BaseURL); err != nil {
			return fmt.Errorf("web_search.base_url %w", err)
		}
	}
	if cfg.Provider == "searxng" {
		if err := ValidateWebSearchURL(cfg.SearXNGDirectoryURL); err != nil {
			return fmt.Errorf("web_search.searxng_directory_url %w", err)
		}
	}
	if cfg.MaxResults < 1 || cfg.MaxResults > 20 {
		return fmt.Errorf("web_search.max_results must be between 1 and 20")
	}
	if cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > 60 {
		return fmt.Errorf("web_search.timeout_seconds must be between 1 and 60")
	}
	if cfg.MaxToolRounds < 1 || cfg.MaxToolRounds > 4 {
		return fmt.Errorf("web_search.max_tool_rounds must be between 1 and 4")
	}
	if cfg.MaxResultBytes < 1024 || cfg.MaxResultBytes > 1<<20 {
		return fmt.Errorf("web_search.max_result_bytes must be between 1024 and 1048576")
	}
	return nil
}

func ResolveWebSearchSecret(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "env:") {
		return strings.TrimSpace(os.Getenv(strings.TrimSpace(strings.TrimPrefix(value, "env:"))))
	}
	return os.ExpandEnv(value)
}

func NewWebSearchProvider(cfg WebSearchConfig) (Provider, error) {
	cfg = NormalizeWebSearchConfig(cfg)
	if err := ValidateWebSearchConfig(cfg); err != nil {
		return nil, err
	}
	switch cfg.Provider {
	case "searxng":
		if cfg.SearXNGMode == searxngModeAuto {
			primary := &autoSearxngSearchProvider{directoryURL: cfg.SearXNGDirectoryURL, fallbackURL: cfg.BaseURL}
			if cfg.FallbackProvider == webSearchProviderDuckDuckGo {
				return &fallbackWebSearchProvider{
					primary:  primary,
					fallback: &duckDuckGoSearchProvider{endpoint: defaultDuckDuckGoEndpoint},
				}, nil
			}
			return primary, nil
		}
		return &searxngSearchProvider{baseURL: cfg.BaseURL}, nil
	case webSearchProviderDuckDuckGo:
		return &duckDuckGoSearchProvider{endpoint: cfg.BaseURL}, nil
	case "tavily":
		apiKey := ResolveWebSearchSecret(cfg.APIKey)
		if apiKey == "" {
			return nil, fmt.Errorf("web_search Tavily API key resolved to an empty value")
		}
		return &tavilySearchProvider{endpoint: cfg.BaseURL, apiKey: apiKey}, nil
	default:
		return nil, fmt.Errorf("unsupported web search provider %q", cfg.Provider)
	}
}

func ValidateWebSearchURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("must be an http(s) URL without embedded credentials")
	}
	return nil
}

func DoWebSearchRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	req = req.WithContext(ctx)
	resp, err := GetHTTPClient(false).Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errWebSearchRequestTimeout
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return nil, errWebSearchRequestTimeout
		}
		// net/http 错误包含完整请求 URL，因此不能将搜索词复制到日志或工具错误信息中。
		return nil, errWebSearchRequestFailed
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebSearchResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxWebSearchResponseBytes {
		return nil, fmt.Errorf("search provider response exceeded %d bytes", maxWebSearchResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &webSearchHTTPStatusError{StatusCode: resp.StatusCode}
	}
	return body, nil
}

type searxngSearchProvider struct {
	baseURL string
}

func (p *searxngSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(strings.TrimRight(endpoint.Path, "/"), "/search") {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/search"
	}
	values := endpoint.Query()
	values.Set("q", query)
	values.Set("format", "json")
	endpoint.RawQuery = values.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", applicationName+"/web-search")
	body, err := DoWebSearchRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode SearXNG response: %w", err)
	}
	results := make([]Result, 0, MinInt(maxResults, len(decoded.Results)))
	for _, item := range decoded.Results {
		if len(results) >= maxResults {
			break
		}
		if result, ok := NormalizeWebSearchResult(item.Title, item.URL, item.Content, item.Score); ok {
			results = append(results, result)
		}
	}
	return results, nil
}

type tavilySearchProvider struct {
	endpoint string
	apiKey   string
}

func (p *tavilySearchProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	payload, _ := json.Marshal(map[string]any{
		"api_key": p.apiKey, "query": query, "max_results": maxResults,
		"search_depth": "basic", "include_answer": false, "include_raw_content": false,
	})
	req, err := http.NewRequest(http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	body, err := DoWebSearchRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode Tavily response: %w", err)
	}
	results := make([]Result, 0, MinInt(maxResults, len(decoded.Results)))
	for _, item := range decoded.Results {
		if len(results) >= maxResults {
			break
		}
		if result, ok := NormalizeWebSearchResult(item.Title, item.URL, item.Content, item.Score); ok {
			results = append(results, result)
		}
	}
	return results, nil
}

func NormalizeWebSearchResult(title, rawURL, snippet string, score float64) (Result, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return Result{}, false
	}
	clean := func(value string, limit int) string {
		value = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(value, "\x00", ""), "\r", " ")), " ")
		if len(value) > limit {
			value = value[:limit] + "…"
		}
		return value
	}
	return Result{Title: clean(title, 512), URL: parsed.String(), Snippet: clean(snippet, 4096), Score: score}, true
}

func WebSearchWithTimeout(ctx context.Context, cfg WebSearchConfig, query string) (SearchResponse, error) {
	cfg = NormalizeWebSearchConfig(cfg)
	query = strings.TrimSpace(query)
	if query == "" {
		err := fmt.Errorf("search query is empty")
		log.Printf("[内置 Web Search] 搜索失败 provider=%s 阶段=参数校验 错误=%v", cfg.Provider, err)
		return SearchResponse{}, err
	}
	if len(query) > 2048 {
		query = query[:2048]
	}
	startedAt := time.Now()
	provider, err := NewWebSearchProvider(cfg)
	if err != nil {
		log.Printf("[内置 Web Search] 搜索失败 provider=%s 阶段=初始化 耗时=%s 错误=%v", cfg.Provider, time.Since(startedAt).Round(time.Millisecond), err)
		return SearchResponse{}, err
	}
	searchCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	results, err := provider.Search(searchCtx, query, cfg.MaxResults)
	if err != nil {
		log.Printf("[内置 Web Search] 搜索失败 provider=%s 耗时=%s 错误=%v", cfg.Provider, time.Since(startedAt).Round(time.Millisecond), err)
		return SearchResponse{}, err
	}
	response := SearchResponse{Query: query, Results: results}
	response.Results = TruncateWebSearchResults(response.Results, cfg.MaxResultBytes)
	log.Printf("[内置 Web Search] 搜索完成 provider=%s 结果数=%d 耗时=%s", cfg.Provider, len(response.Results), time.Since(startedAt).Round(time.Millisecond))
	return response, nil
}

func TruncateWebSearchResults(results []Result, maxBytes int) []Result {
	kept := make([]Result, 0, len(results))
	for _, result := range results {
		candidate := append(append([]Result(nil), kept...), result)
		encoded, _ := json.Marshal(candidate)
		if len(encoded) > maxBytes {
			break
		}
		kept = candidate
	}
	return kept
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
