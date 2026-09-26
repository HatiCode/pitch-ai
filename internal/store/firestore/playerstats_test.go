package firestore

import (
	"context"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

func readPlayerStats(t *testing.T, store *Store, clubID, teamID, matchID, playerID string) core.PlayerMatchStats {
	t.Helper()

	snap, err := store.playerStatsCol(clubID, teamID, matchID).Doc(playerID).Get(context.Background())
	if err != nil {
		t.Fatalf("get playerStats %q: %v", playerID, err)
	}
	var stats core.PlayerMatchStats
	if err := snap.DataTo(&stats); err != nil {
		t.Fatalf("decode playerStats %q: %v", playerID, err)
	}
	return stats
}

func TestPutPlayerStatsRoundTripsTheDenormalisedFields(t *testing.T) {
	store := newTestStore(t)
	clubID := uniqueClubID(t)
	kickoff := time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)

	stats := []core.PlayerMatchStats{
		{
			PlayerID:  "p7",
			MatchID:   "m1",
			TeamID:    "team-1",
			SeasonID:  "2026-27",
			MatchDate: kickoff,
			Jersey:    7,
			Position:  core.Openside,
			Group:     core.BackRow,
			MinutesMs: 4800000,
			Counts:    map[string]int{string(core.KindTackleMade): 12, string(core.KindJackalWon): 2},
		},
		{
			PlayerID:  "p10",
			MatchID:   "m1",
			TeamID:    "team-1",
			SeasonID:  "2026-27",
			MatchDate: kickoff,
			Jersey:    10,
			Position:  core.FlyHalf,
			Group:     core.HalfBacks,
			MinutesMs: 4800000,
			Counts:    map[string]int{string(core.KindKick): 9},
		},
	}

	if err := store.PutPlayerStats(context.Background(), clubID, "team-1", "m1", stats); err != nil {
		t.Fatalf("PutPlayerStats() error = %v", err)
	}

	got := readPlayerStats(t, store, clubID, "team-1", "m1", "p7")
	// Denormalised so a season query is a collection-group read rather than a
	// rollup table that can go stale — M4 depends on every one of these.
	if got.TeamID != "team-1" || got.SeasonID != "2026-27" || got.MatchID != "m1" {
		t.Errorf("identity = (%q, %q, %q), want (team-1, 2026-27, m1)", got.TeamID, got.SeasonID, got.MatchID)
	}
	if !got.MatchDate.Equal(kickoff) {
		t.Errorf("MatchDate = %v, want %v", got.MatchDate, kickoff)
	}
	if got.Jersey != 7 || got.Position != core.Openside || got.Group != core.BackRow {
		t.Errorf("jersey/position/group = (%d, %q, %q), want (7, openside, back_row)", got.Jersey, got.Position, got.Group)
	}
	if got.MinutesMs != 4800000 || got.Counts[string(core.KindTackleMade)] != 12 {
		t.Errorf("minutes/counts = (%d, %v), want (4800000, tackle_made 12)", got.MinutesMs, got.Counts)
	}

	if fly := readPlayerStats(t, store, clubID, "team-1", "m1", "p10"); fly.Counts[string(core.KindKick)] != 9 {
		t.Errorf("p10 kicks = %d, want 9 — the whole batch must land", fly.Counts[string(core.KindKick)])
	}
}

// A re-fold after a void has to lower a count. Merging would leave the old,
// higher figure in place and there would be no way to correct a mis-tap.
func TestPutPlayerStatsOverwritesRatherThanMerges(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	clubID := uniqueClubID(t)

	before := core.PlayerMatchStats{
		PlayerID:  "p7",
		MatchID:   "m1",
		TeamID:    "team-1",
		SeasonID:  "2026-27",
		Jersey:    7,
		MinutesMs: 4800000,
		Counts:    map[string]int{string(core.KindTackleMade): 3, string(core.KindCarry): 1},
	}
	if err := store.PutPlayerStats(ctx, clubID, "team-1", "m1", []core.PlayerMatchStats{before}); err != nil {
		t.Fatalf("PutPlayerStats() error = %v", err)
	}

	after := before
	after.MinutesMs = 2400000
	after.Counts = map[string]int{string(core.KindTackleMade): 2}
	if err := store.PutPlayerStats(ctx, clubID, "team-1", "m1", []core.PlayerMatchStats{after}); err != nil {
		t.Fatalf("PutPlayerStats() error = %v", err)
	}

	got := readPlayerStats(t, store, clubID, "team-1", "m1", "p7")
	if got.Counts[string(core.KindTackleMade)] != 2 {
		t.Errorf("tackle_made = %d, want 2", got.Counts[string(core.KindTackleMade)])
	}
	if _, ok := got.Counts[string(core.KindCarry)]; ok {
		t.Errorf("Counts = %v, want the voided carry gone rather than merged", got.Counts)
	}
	if got.MinutesMs != 2400000 {
		t.Errorf("MinutesMs = %d, want 2400000", got.MinutesMs)
	}
}

func TestPutPlayerStatsWithNothingToWriteIsANoOp(t *testing.T) {
	store := newTestStore(t)

	if err := store.PutPlayerStats(context.Background(), uniqueClubID(t), "team-1", "m1", nil); err != nil {
		t.Errorf("PutPlayerStats(nil) error = %v, want nil", err)
	}
}
