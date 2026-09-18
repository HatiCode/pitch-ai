package auth

import (
	"context"
	"errors"
)

// ErrNoMembership means the token is valid but the account belongs to no club.
var ErrNoMembership = errors.New("auth: no membership for user")

type Claims struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
}

type Membership struct {
	UID     string   `json:"uid"`
	ClubID  string   `json:"clubId"`
	TeamIDs []string `json:"teamIds"`
	Role    Role     `json:"role"`
}

// TokenVerifier turns a bearer token into verified claims.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (Claims, error)
}

// MembershipStore resolves a verified user to their club and role.
type MembershipStore interface {
	Membership(ctx context.Context, uid string) (Membership, error)
}
