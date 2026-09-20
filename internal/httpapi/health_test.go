package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(Deps{Logger: slog.New(slog.DiscardHandler)})

	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`status field = %q, want "ok"`, body["status"])
	}
}

// An unknown /api/ path must return JSON, not the SPA's HTML: a client that
// mistypes an endpoint should see a 404 it can parse, not a parse error.
func TestUnknownAPIPathIs404JSON(t *testing.T) {
	router := NewRouter(Deps{
		Logger: slog.New(slog.DiscardHandler),
		Assets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>pitch-ai</title>")}},
	})

	for _, target := range []string{"/api/nope", "/api/teams/t1/nope"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s content type = %q, want JSON", target, ct)
		}
	}
}

// The SPA fallback must still serve client-side routes.
func TestNonAPIPathStillReachesTheSPA(t *testing.T) {
	router := NewRouter(Deps{
		Logger: slog.New(slog.DiscardHandler),
		Assets: fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>pitch-ai</title>")}},
	})

	req := httptest.NewRequest(http.MethodGet, "/squads/team-1/matches", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>pitch-ai</title>") {
		t.Error("client route did not fall back to index.html")
	}
}
