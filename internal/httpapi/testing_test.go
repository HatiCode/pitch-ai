package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/squad"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// fakeAuthMiddleware injects a membership directly, so handler tests exercise
// routing and permissions without minting real Firebase tokens.
func fakeAuthMiddleware(member auth.Membership) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), member)))
		})
	}
}

// newTestRouter builds a router wired to a squad service and a fake identity.
func newTestRouter(t *testing.T, member auth.Membership, svc *squad.Service) http.Handler {
	t.Helper()

	return NewRouter(Deps{
		Logger: discardLogger(),
		Squad:  svc,
		Auth:   fakeAuthMiddleware(member),
	})
}

func do(t *testing.T, router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
