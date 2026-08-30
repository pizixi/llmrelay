package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	siteInfoRequestTimeout = 8 * time.Second
	siteInfoMaxBodyBytes   = 1 << 20 // a title is near the beginning of a page
	siteInfoMaxNameRunes   = 128
	siteInfoMaxJSONDepth   = 6
)

var siteInfoKeySeparators = regexp.MustCompile(`[^a-z0-9]+`)

var siteInfoHTTPClient = &http.Client{
	Timeout: siteInfoRequestTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// SiteInfoHandler returns a public site's display name for the admin form.
// Base URLs are API endpoints in most configurations (for example
// https://provider.example/v1), so the request always targets the origin root
// and never forwards the API path to the metadata lookup.
func SiteInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	origin, err := siteInfoOrigin(r.URL.Query().Get("url"))
	if err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), siteInfoRequestTimeout)
	defer cancel()
	// Modern upstream dashboards are often SPAs: their static HTML contains a
	// framework placeholder title and the browser replaces document.title with
	// a public runtime setting after startup. Fetch both sources concurrently
	// so a missing status endpoint does not delay ordinary HTML sites.
	type siteInfoResult struct {
		body []byte
		err  error
	}
	pageResult := make(chan siteInfoResult, 1)
	runtimeResult := make(chan siteInfoResult, 1)
	go func() {
		body, fetchErr := fetchSiteInfoResource(ctx, origin, "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")
		pageResult <- siteInfoResult{body: body, err: fetchErr}
	}()
	go func() {
		body, fetchErr := fetchSiteInfoResource(ctx, origin+"/api/status", "application/json")
		runtimeResult <- siteInfoResult{body: body, err: fetchErr}
	}()
	page := <-pageResult
	runtime := <-runtimeResult
	if page.err != nil && runtime.err != nil {
		writeAdminJSON(w, http.StatusBadGateway, map[string]any{"error": "failed to fetch site information"})
		return
	}

	runtimeName := siteInfoRuntimeName(runtime.body)
	pageName := siteInfoName(page.body)
	name := firstSiteInfoName(runtimeName, pageName)
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"name":   name,
		"origin": origin,
	})
}

func fetchSiteInfoResource(ctx context.Context, target, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, errors.New("invalid site URL")
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "LLMRelay Admin/1.0")
	resp, err := siteInfoHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("site returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, siteInfoMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > siteInfoMaxBodyBytes {
		body = body[:siteInfoMaxBodyBytes]
	}
	return body, nil
}

// siteInfoOrigin normalizes an upstream Base URL to its origin root. It is
// deliberately independent from request routing: /v1, /models and query
// parameters are all omitted from metadata requests.
func siteInfoOrigin(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + strings.TrimLeft(value, "/")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("invalid site URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("site URL must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("site URL must not contain credentials")
	}
	parsed.Path = "/"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	// Keep only the origin in the value returned to the caller. net/http will
	// still issue the request with `/` as its request-target, but no API path
	// (such as `/v1`) is carried as a metadata parameter.
	return parsed.Scheme + "://" + parsed.Host, nil
}

func siteInfoName(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	var title strings.Builder
	var openTitle bool
	var siteName string
	var applicationName string
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				// The title is what a browser displays before any client-side
				// runtime customization. Other metadata is a fallback only.
				return firstSiteInfoName(title.String(), siteName, applicationName)
			}
			return firstSiteInfoName(title.String(), siteName, applicationName)
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			switch strings.ToLower(token.Data) {
			case "title":
				if tokenType == html.StartTagToken {
					openTitle = true
				}
			case "meta":
				property, name, content := "", "", ""
				for _, attr := range token.Attr {
					switch strings.ToLower(attr.Key) {
					case "property":
						property = strings.ToLower(strings.TrimSpace(attr.Val))
					case "name":
						name = strings.ToLower(strings.TrimSpace(attr.Val))
					case "content":
						content = attr.Val
					}
				}
				content = cleanSiteInfoName(content)
				if content == "" {
					continue
				}
				if siteName == "" && (property == "og:site_name" || name == "og:site_name") {
					siteName = content
				}
				if applicationName == "" && name == "application-name" {
					applicationName = content
				}
			}
		case html.TextToken:
			if openTitle {
				title.WriteString(tokenizer.Token().Data)
			}
		case html.EndTagToken:
			if strings.EqualFold(tokenizer.Token().Data, "title") {
				openTitle = false
			}
		}
	}
}

func siteInfoRuntimeName(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	name, _ := findSiteInfoRuntimeName(payload, 0)
	return name
}

func findSiteInfoRuntimeName(value any, depth int) (string, int) {
	if depth > siteInfoMaxJSONDepth {
		return "", 0
	}
	bestName := ""
	bestScore := 0
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			score := siteInfoRuntimeKeyScore(key)
			if score > 0 {
				if text, ok := child.(string); ok {
					if cleaned := cleanSiteInfoName(text); cleaned != "" && score > bestScore {
						bestName, bestScore = cleaned, score
					}
				}
			}
			childName, childScore := findSiteInfoRuntimeName(child, depth+1)
			// Prefer a shallower field when aliases have the same confidence.
			childScore -= depth
			if childScore > bestScore {
				bestName, bestScore = childName, childScore
			}
		}
	case []any:
		for _, child := range typed {
			childName, childScore := findSiteInfoRuntimeName(child, depth+1)
			childScore -= depth
			if childScore > bestScore {
				bestName, bestScore = childName, childScore
			}
		}
	}
	return bestName, bestScore
}

func siteInfoRuntimeKeyScore(key string) int {
	normalized := siteInfoKeySeparators.ReplaceAllString(strings.ToLower(strings.TrimSpace(key)), "")
	switch normalized {
	case "systemname":
		return 100
	case "sitename", "websitename":
		return 95
	case "applicationname", "appname":
		return 90
	case "brandname", "productname":
		return 85
	case "pagetitle", "sitetitle":
		return 80
	default:
		return 0
	}
}

func firstSiteInfoName(values ...string) string {
	for _, value := range values {
		if cleaned := cleanSiteInfoName(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func cleanSiteInfoName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) > siteInfoMaxNameRunes {
		runes := []rune(value)
		value = string(runes[:siteInfoMaxNameRunes])
	}
	return value
}
