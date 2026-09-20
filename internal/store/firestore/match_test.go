package firestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

func seedMatch(t *testing.T, store *Store, clubID, teamID, id, opponent string, kickoff time.Time) core.Match {
	t.Helper()
	m := core.Match{
		ID:          id,
		ClubID:      clubID,
		TeamID:      teamID,
		SeasonID:    "2026-27",
		Opponent:    opponent,
		KickoffAt:   kickoff,
		Competition: "Regional 1",
		Venue:       core.VenueHome,
		Status:      core.MatchScheduled,
	}
	if err := store.PutMatch(context.Background(), clubID, teamID, m); err != nil {
		t.Fatalf("PutMatch() error = %v", err)
	}
	return m
}

func march(day int) time.Time {
	return time.Date(2026, 3, day, 15, 0, 0, 0, time.UTC)
}

func TestMatchRoundTrip(t *testing.T) {
	store := newTestStore(t)
	want := seedMatch(t, store, "club-match-rt", "team-1", "match-1", "Castelnau RC", march(14))

	got, err := store.Match(context.Background(), "club-match-rt", "team-1", "match-1")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.Opponent != want.Opponent || got.ID != "match-1" || got.TeamID != "team-1" {
		t.Errorf("Match() = %+v, want %+v", got, want)
	}
	if !got.KickoffAt.Equal(want.KickoffAt) {
		t.Errorf("KickoffAt = %v, want %v", got.KickoffAt, want.KickoffAt)
	}
	if got.Venue != core.VenueHome || got.Status != core.MatchScheduled {
		t.Errorf("venue/status = (%q, %q), want (home, scheduled)", got.Venue, got.Status)
	}
}

func TestMatchMissingIsErrNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Match(context.Background(), "club-none", "team-1", "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Match() error = %v, want core.ErrNotFound", err)
	}
}

func TestMatchesAreScopedToTheirTeamAndSortedNewestFirst(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-match-scope"

	seedMatch(t, store, clubID, "team-1", "m1", "Castelnau RC", march(14))
	seedMatch(t, store, clubID, "team-1", "m2", "Lavaur", march(21))
	seedMatch(t, store, clubID, "team-2", "m3", "Colts opposition", march(14))

	list, err := store.Matches(context.Background(), clubID, "team-1")
	if err != nil {
		t.Fatalf("Matches() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(Matches()) = %d, want 2", len(list))
	}
	if list[0].Opponent != "Lavaur" {
		t.Errorf("first match = %q, want the most recent kick-off (Lavaur)", list[0].Opponent)
	}
}

func TestMatchesOnEmptyTeamReturnsNoError(t *testing.T) {
	store := newTestStore(t)

	list, err := store.Matches(context.Background(), "club-empty", "team-none")
	if err != nil {
		t.Fatalf("Matches() error = %v, want nil", err)
	}
	if len(list) != 0 {
		t.Errorf("len(Matches()) = %d, want 0", len(list))
	}
}

func TestPutMatchPersistsLineup(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-match-lineup"
	m := seedMatch(t, store, clubID, "team-1", "m-lineup", "Castelnau RC", march(14))

	m.Lineup = core.Lineup{
		Starters: []core.LineupSlot{{Jersey: 7, PlayerID: "player-7"}},
		Bench:    []core.LineupSlot{{Jersey: 16, PlayerID: "player-16"}},
	}
	if err := store.PutMatch(context.Background(), clubID, "team-1", m); err != nil {
		t.Fatalf("PutMatch() error = %v", err)
	}

	got, err := store.Match(context.Background(), clubID, "team-1", "m-lineup")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(got.Lineup.Starters) != 1 || got.Lineup.Starters[0].Jersey != 7 {
		t.Errorf("Lineup.Starters = %+v, want jersey 7", got.Lineup.Starters)
	}
	if len(got.Lineup.Bench) != 1 || got.Lineup.Bench[0].PlayerID != "player-16" {
		t.Errorf("Lineup.Bench = %+v, want player-16", got.Lineup.Bench)
	}
}

// Fixtures live under the team, so disbanding a squad must take them with it.
func TestDeletingATeamRemovesItsMatches(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	clubID := "club-match-cascade"

	seedTeam(t, store, clubID, "team-doomed", "Doomed XV")
	seedMatch(t, store, clubID, "team-doomed", "m-doomed", "Castelnau RC", march(14))

	if err := store.DeleteTeam(ctx, clubID, "team-doomed"); err != nil {
		t.Fatalf("DeleteTeam() error = %v", err)
	}

	if _, err := store.Match(ctx, clubID, "team-doomed", "m-doomed"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Match() after team deletion error = %v, want core.ErrNotFound", err)
	}
}
