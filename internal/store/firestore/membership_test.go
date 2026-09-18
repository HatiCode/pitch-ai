package firestore

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/auth"
)

func TestMembershipRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	want := auth.Membership{
		UID:     "uid-round-trip",
		ClubID:  "club-1",
		TeamIDs: []string{"team-1", "team-2"},
		Role:    auth.RoleCoach,
	}
	if _, err := store.client.Collection("users").Doc(want.UID).Set(ctx, want); err != nil {
		t.Fatalf("seeding membership: %v", err)
	}

	got, err := store.Membership(ctx, want.UID)
	if err != nil {
		t.Fatalf("Membership() error = %v", err)
	}
	if got.ClubID != want.ClubID || got.Role != want.Role || len(got.TeamIDs) != 2 {
		t.Errorf("Membership() = %+v, want %+v", got, want)
	}
}

func TestMembershipMissingUserIsErrNoMembership(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Membership(context.Background(), "uid-that-does-not-exist")
	if !errors.Is(err, auth.ErrNoMembership) {
		t.Errorf("error = %v, want auth.ErrNoMembership", err)
	}
}
