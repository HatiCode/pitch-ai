package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>pitch-ai</title>")},
		"assets/app.js":        {Data: []byte("console.log(1)")},
		"sw.js":                {Data: []byte("self.addEventListener('install', () => {})")},
		"manifest.webmanifest": {Data: []byte(`{"name":"pitch-ai"}`)},
	}
}

// Go's mime table has no .webmanifest entry, so without this the manifest goes
// out as text/plain and the browser has no installable app to offer.
func TestSPAServesTheManifestAsJSON(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/manifest+json")
	}
}

func TestSPAServesRealFile(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("body = %q, want the asset contents", rec.Body.String())
	}
}

func TestSPAFallsBackToIndexForClientRoutes(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/matches/abc123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>pitch-ai</title>") {
		t.Errorf("body = %q, want index.html", rec.Body.String())
	}
}

// A service worker that a CDN or a browser heuristic caches is how a PWA gets
// stuck on last week's bundle. The immutable header on hashed assets is the
// counterpart that makes no-cache on the shell cheap.
func TestSPACacheHeaders(t *testing.T) {
	h := SPAHandler(testAssets())

	cases := map[string]string{
		"/sw.js":         "no-cache",
		"/index.html":    "no-cache",
		"/":              "no-cache",
		"/matches/abc":   "no-cache",
		"/assets/app.js": "public, max-age=31536000, immutable",
	}

	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if got := rec.Header().Get("Cache-Control"); got != want {
				t.Errorf("Cache-Control = %q, want %q", got, want)
			}
		})
	}
}

func TestSPAServesIndexAtRoot(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
