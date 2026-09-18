package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeVerifier struct {
	claims Claims
	err    error
}

func (f fakeVerifier) Verify(context.Context, string) (Claims, error) {
	return f.claims, f.err
}

type fakeMemberships map[string]Membership

func (f fakeMemberships) Membership(_ context.Context, uid string) (Membership, error) {
	m, ok := f[uid]
	if !ok {
		return Membership{}, ErrNoMembership
	}
	return m, nil
}

func protectedHandler(t *testing.T, seen *Membership) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, ok := FromContext(r.Context())
		if !ok {
			t.Error("membership missing from request context")
		}
		*seen = m
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestMiddlewareAttachesMembership(t *testing.T) {
	want := Membership{UID: "uid-1", ClubID: "club-1", TeamIDs: []string{"team-1"}, Role: RoleAdmin}
	var got Membership

	mw := Middleware(
		fakeVerifier{claims: Claims{UID: "uid-1", Email: "coach@example.com"}},
		fakeMemberships{"uid-1": want},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer token-abc")
	rec := httptest.NewRecorder()
	mw(protectedHandler(t, &got)).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got.ClubID != want.ClubID || got.Role != want.Role {
		t.Errorf("membership = %+v, want %+v", got, want)
	}
}

func TestMiddlewareRejectsMissingHeader(t *testing.T) {
	mw := Middleware(fakeVerifier{}, fakeMemberships{})

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run without a token")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejectsInvalidToken(t *testing.T) {
	mw := Middleware(fakeVerifier{err: errors.New("bad signature")}, fakeMemberships{})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer rubbish")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run for an invalid token")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareRejectsAuthenticatedNonMember(t *testing.T) {
	mw := Middleware(fakeVerifier{claims: Claims{UID: "stranger"}}, fakeMemberships{})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer token-abc")
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run for a non-member")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
