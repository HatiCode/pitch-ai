package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type contextKey struct{}

// FromContext returns the membership attached by Middleware.
func FromContext(ctx context.Context) (Membership, bool) {
	m, ok := ctx.Value(contextKey{}).(Membership)
	return m, ok
}

// NewContext attaches a membership to a context. Middleware uses it in
// production; handler tests use it to build an authenticated request.
func NewContext(ctx context.Context, m Membership) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}

func Middleware(v TokenVerifier, s MembershipStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			claims, err := v.Verify(r.Context(), token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			member, err := s.Membership(r.Context(), claims.UID)
			if err != nil {
				if errors.Is(err, ErrNoMembership) {
					http.Error(w, "account is not a member of any club", http.StatusForbidden)
					return
				}
				http.Error(w, "membership lookup failed", http.StatusInternalServerError)
				return
			}

			next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), member)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(header[len(prefix):]), true
}
