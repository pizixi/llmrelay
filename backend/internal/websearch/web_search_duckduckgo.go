package websearch

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const searxngFallbackPrimaryBudget = time.Second

var duckDuckGoUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
}

var duckDuckGoAcceptLanguages = []string{
	"en-US,en;q=0.9",
	"zh-CN,zh;q=0.9,en;q=0.8",
	"en-GB,en;q=0.9,en-US;q=0.8",
}

var duckDuckGoSearchRate = struct {
	sync.Mutex
	last time.Time
}{}

type duckDuckGoSearchProvider struct {
	endpoint string
}

func (p *duckDuckGoSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if err := WaitForDuckDuckGoSearch(ctx); err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(strings.TrimSpace(p.endpoint))
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("q", query)
	endpoint.RawQuery = values.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	SetDuckDuckGoHeaders(req)
	body, err := DoWebSearchRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	return ParseDuckDuckGoLiteResults(body, maxResults)
}

func SetDuckDuckGoHeaders(req *http.Request) {
	req.Header.Set("User-Agent", duckDuckGoUserAgents[rand.IntN(len(duckDuckGoUserAgents))])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", duckDuckGoAcceptLanguages[rand.IntN(len(duckDuckGoAcceptLanguages))])
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("DNT", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func WaitForDuckDuckGoSearch(ctx context.Context) error {
	duckDuckGoSearchRate.Lock()
	defer duckDuckGoSearchRate.Unlock()
	minimumGap := time.Duration(500+rand.IntN(1000)) * time.Millisecond
	wait := minimumGap - time.Since(duckDuckGoSearchRate.last)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	duckDuckGoSearchRate.last = time.Now()
	return nil
}

func ParseDuckDuckGoLiteResults(body []byte, maxResults int) ([]Result, error) {
	if maxResults <= 0 {
		maxResults = defaultWebSearchMaxResults
	}
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("decode DuckDuckGo response: %w", err)
	}
	results := make([]Result, 0, maxResults)
	var currentTitle, currentURL, currentSnippet string
	flush := func() {
		if len(results) >= maxResults || currentURL == "" {
			return
		}
		if result, ok := NormalizeWebSearchResult(currentTitle, currentURL, currentSnippet, 0); ok {
			results = append(results, result)
		}
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if node.Type == html.ElementNode {
			if node.Data == "a" && DuckDuckGoNodeHasClass(node, "result-link") {
				flush()
				currentTitle = DuckDuckGoNodeText(node)
				currentURL = CleanDuckDuckGoResultURL(DuckDuckGoNodeAttribute(node, "href"))
				currentSnippet = ""
			}
			if node.Data == "td" && DuckDuckGoNodeHasClass(node, "result-snippet") && currentURL != "" {
				currentSnippet = DuckDuckGoNodeText(node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
			if len(results) >= maxResults {
				return
			}
		}
	}
	walk(document)
	flush()
	return results, nil
}

func DuckDuckGoNodeHasClass(node *html.Node, className string) bool {
	return slices.Contains(strings.Fields(DuckDuckGoNodeAttribute(node, "class")), className)
}

func DuckDuckGoNodeAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func DuckDuckGoNodeText(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(text.String()), " ")
}

func CleanDuckDuckGoResultURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && strings.EqualFold(parsed.Hostname(), "duckduckgo.com") && parsed.Path == "/l/" {
		if target := parsed.Query().Get("uddg"); target != "" {
			return target
		}
	}
	return rawURL
}

type fallbackWebSearchProvider struct {
	primary       Provider
	fallback      Provider
	primaryBudget time.Duration
}

func (p *fallbackWebSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	budget := p.primaryBudget
	if budget <= 0 {
		budget = searxngFallbackPrimaryBudget
	}
	primaryCtx, cancel := context.WithTimeout(ctx, budget)
	results, primaryErr := p.primary.Search(primaryCtx, query, maxResults)
	cancel()
	if primaryErr == nil && len(results) > 0 {
		return results, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	results, fallbackErr := p.fallback.Search(ctx, query, maxResults)
	if fallbackErr == nil {
		return results, nil
	}
	if primaryErr == nil {
		primaryErr = errors.New("primary search returned no results")
	}
	return nil, fmt.Errorf("SearXNG failed (%v); DuckDuckGo fallback failed (%v)", primaryErr, fallbackErr)
}
