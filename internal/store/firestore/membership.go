package firestore

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pitch-ai/internal/auth"
)

func (s *Store) Membership(ctx context.Context, uid string) (auth.Membership, error) {
	snap, err := s.client.Collection("users").Doc(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return auth.Membership{}, auth.ErrNoMembership
	}
	if err != nil {
		return auth.Membership{}, fmt.Errorf("firestore: get membership %q: %w", uid, err)
	}

	var m auth.Membership
	if err := snap.DataTo(&m); err != nil {
		return auth.Membership{}, fmt.Errorf("firestore: decode membership %q: %w", uid, err)
	}
	m.UID = uid
	return m, nil
}
