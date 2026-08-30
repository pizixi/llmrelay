package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiteInfoOriginDropsAPIPathAndQuery(t *testing.T) {
	got, err := siteInfoOrigin("https://provider.example/v1/models?foo=bar")
	if err != nil {
		t.Fatalf("siteInfoOrigin returned error: %v", err)
	}
	if got != "https://provider.example" {
		t.Fatalf("origin = %q, want https://provider.example", got)
	}

	got, err = siteInfoOrigin("provider.example:8443/v1")
	if err != nil {
		t.Fatalf("host-only URL returned error: %v", err)
	}
	if got != "https://provider.example:8443" {
		t.Fatalf("host-only origin = %q", got)
	}
}

func TestSiteInfoHandlerFetchesOriginRootAndParsesName(t *testing.T) {
	requestedPaths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths <- r.URL.RequestURI()
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><meta property="og:site_name" content="Example Site"><title>Browser title</title></head></html>`))
		case "/api/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"system_name":"Runtime Site Name","passkey_display_name":"Framework Name"},"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/site-info?url="+server.URL+"/v1/models?foo=bar", nil)
	SiteInfoHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	seenPaths := map[string]bool{}
	for range 2 {
		seenPaths[<-requestedPaths] = true
	}
	if !seenPaths["/"] || !seenPaths["/api/status"] {
		t.Fatalf("metadata request paths = %#v", seenPaths)
	}
	var response struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "Runtime Site Name" {
		t.Fatalf("name = %q", response.Name)
	}
}

func TestSiteInfoRuntimeNameSupportsCommonBrandFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "snake case", body: `{"data":{"site_name":"Site Name"}}`, want: "Site Name"},
		{name: "camel case", body: `{"settings":{"appName":"App Name"}}`, want: "App Name"},
		{name: "brand", body: `{"public":{"brand-name":"Brand Name"}}`, want: "Brand Name"},
		{name: "priority", body: `{"app_name":"App","system_name":"System"}`, want: "System"},
		{name: "ignore generic fields", body: `{"name":"Health Check","title":"Operational"}`, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := siteInfoRuntimeName([]byte(test.body)); got != test.want {
				t.Fatalf("siteInfoRuntimeName() = %q, want %q", got, test.want)
			}
		})
	}
}
