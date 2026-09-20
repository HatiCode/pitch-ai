package firestore

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/core"
)

func seedTeam(t *testing.T, store *Store, clubID, id, name string) core.Team {
	t.Helper()
	team := core.Team{
		ID:        id,
		ClubID:    clubID,
		Name:      name,
		ShortName: "SN",
		Active:    true,
	}
	if err := store.PutTeam(context.Background(), clubID, team); err != nil {
		t.Fatalf("PutTeam() error = %v", err)
	}
	return team
}

func TestTeamRoundTrip(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-team-round-trip"

	want := seedTeam(t, store, clubID, "team-1", "1st XV")

	got, err := store.Team(context.Background(), clubID, "team-1")
	if err != nil {
		t.Fatalf("Team() error = %v", err)
	}
	if got.Name != want.Name || got.ID != "team-1" || got.ClubID != clubID {
		t.Errorf("Team() = %+v, want %+v", got, want)
	}
	if !got.Active {
		t.Error("Active = false, want true to survive the round trip")
	}
}

func TestTeamMissingIsErrNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Team(context.Background(), "club-none", "no-such-team")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Team() error = %v, want core.ErrNotFound", err)
	}
}

func TestTeamsAreScopedToTheirClubAndSorted(t *testing.T) {
	store := newTestStore(t)

	seedTeam(t, store, "club-teams-a", "t1", "Colts")
	seedTeam(t, store, "club-teams-a", "t2", "1st XV")
	seedTeam(t, store, "club-teams-b", "t3", "Someone else")

	list, err := store.Teams(context.Background(), "club-teams-a")
	if err != nil {
		t.Fatalf("Teams() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(Teams()) = %d, want 2", len(list))
	}
	if list[0].Name != "1st XV" || list[1].Name != "Colts" {
		t.Errorf("Teams() = [%q %q], want sorted [1st XV, Colts]", list[0].Name, list[1].Name)
	}
}

func TestTeamsOnEmptyClubReturnsNoError(t *testing.T) {
	store := newTestStore(t)

	list, err := store.Teams(context.Background(), "club-with-no-teams")
	if err != nil {
		t.Fatalf("Teams() error = %v, want nil", err)
	}
	if len(list) != 0 {
		t.Errorf("len(Teams()) = %d, want 0", len(list))
	}
}

// Firestore does not cascade deletes, so a squad's fixtures would survive as
// unreachable orphans unless DeleteTeam walks the descendants. This seeds a
// nested document to prove it does.
func TestDeleteTeamRemovesNestedData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	clubID := "club-team-cascade"

	seedTeam(t, store, clubID, "team-cascade", "Doomed XV")

	matchRef := store.teamsCol(clubID).Doc("team-cascade").Collection("matches").Doc("match-1")
	if _, err := matchRef.Set(ctx, map[string]any{"opponent": "Castelnau RC"}); err != nil {
		t.Fatalf("seeding nested match: %v", err)
	}
	eventRef := matchRef.Collection("events").Doc("event-1")
	if _, err := eventRef.Set(ctx, map[string]any{"kind": "tackle_made"}); err != nil {
		t.Fatalf("seeding nested event: %v", err)
	}

	if err := store.DeleteTeam(ctx, clubID, "team-cascade"); err != nil {
		t.Fatalf("DeleteTeam() error = %v", err)
	}

	if _, err := store.Team(ctx, clubID, "team-cascade"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Team() after delete error = %v, want core.ErrNotFound", err)
	}

	snap, err := matchRef.Get(ctx)
	if err == nil && snap.Exists() {
		t.Error("nested match document survived the team deletion")
	}
	eventSnap, err := eventRef.Get(ctx)
	if err == nil && eventSnap.Exists() {
		t.Error("nested event document survived the team deletion — two levels down was not cascaded")
	}
}

func TestDeleteTeamIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	clubID := "club-team-idempotent"
	seedTeam(t, store, clubID, "team-gone", "Gone")

	if err := store.DeleteTeam(ctx, clubID, "team-gone"); err != nil {
		t.Fatalf("first DeleteTeam() error = %v", err)
	}
	if err := store.DeleteTeam(ctx, clubID, "team-gone"); err != nil {
		t.Errorf("second DeleteTeam() error = %v, want nil — deleting twice must not fail", err)
	}
}
