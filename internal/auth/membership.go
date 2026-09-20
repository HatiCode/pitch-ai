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

// Membership is stored at users/{uid}. The document ID carries the UID, so it
// is not duplicated in the body; the remaining tags pin the Firestore field
// names rather than leaving them to default to the Go field names.
type Membership struct {
	UID     string   `json:"uid" firestore:"-"`
	ClubID  string   `json:"clubId" firestore:"clubId"`
	TeamIDs []string `json:"teamIds" firestore:"teamIds"`
	Role    Role     `json:"role" firestore:"role"`
}

// TokenVerifier turns a bearer token into verified claims.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (Claims, error)
}

// MembershipStore resolves a verified user to their club and role.
type MembershipStore interface {
	Membership(ctx context.Context, uid string) (Membership, error)
}
