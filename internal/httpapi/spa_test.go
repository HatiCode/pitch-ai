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
		"index.html":    {Data: []byte("<!doctype html><title>pitch-ai</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
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

func TestSPAServesIndexAtRoot(t *testing.T) {
	h := SPAHandler(testAssets())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
