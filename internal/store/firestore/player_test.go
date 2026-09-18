package firestore

import (
	"context"
	"errors"
	"testing"

	"pitch-ai/internal/core"
)

func seedPlayer(t *testing.T, store *Store, clubID, id, lastName string) core.Player {
	t.Helper()
	p := core.Player{
		ID:        id,
		ClubID:    clubID,
		FirstName: "Thomas",
		LastName:  lastName,
		DOB:       "1998-04-12",
		Positions: []core.Position{core.Openside},
		TeamIDs:   []string{"team-1"},
		Status:    core.PlayerActive,
	}
	if err := store.PutPlayer(context.Background(), clubID, p); err != nil {
		t.Fatalf("PutPlayer() error = %v", err)
	}
	return p
}

func TestPlayerRoundTrip(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-player-round-trip"

	want := seedPlayer(t, store, clubID, "player-1", "Lefevre")

	got, err := store.Player(context.Background(), clubID, "player-1")
	if err != nil {
		t.Fatalf("Player() error = %v", err)
	}
	if got.LastName != want.LastName || got.ID != "player-1" || got.ClubID != clubID {
		t.Errorf("Player() = %+v, want %+v", got, want)
	}
	if got.DOB != want.DOB {
		t.Errorf("DOB = %q, want %q", got.DOB, want.DOB)
	}
	if len(got.Positions) != 1 || got.Positions[0] != core.Openside {
		t.Errorf("Positions = %v, want [openside]", got.Positions)
	}
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != "team-1" {
		t.Errorf("TeamIDs = %v, want [team-1]", got.TeamIDs)
	}
}

func TestPlayerMissingIsErrNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Player(context.Background(), "club-missing", "no-such-player")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Player() error = %v, want core.ErrNotFound", err)
	}
}

func TestPlayersAreScopedToTheirClub(t *testing.T) {
	store := newTestStore(t)

	seedPlayer(t, store, "club-scope-a", "p1", "Alpha")
	seedPlayer(t, store, "club-scope-a", "p2", "Bravo")
	seedPlayer(t, store, "club-scope-b", "p3", "Charlie")

	list, err := store.Players(context.Background(), "club-scope-a")
	if err != nil {
		t.Fatalf("Players() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(Players()) = %d, want 2", len(list))
	}
	if list[0].LastName != "Alpha" || list[1].LastName != "Bravo" {
		t.Errorf("Players() = [%q %q], want sorted [Alpha Bravo]", list[0].LastName, list[1].LastName)
	}
}

func TestPlayersOnEmptyClubReturnsNoError(t *testing.T) {
	store := newTestStore(t)

	list, err := store.Players(context.Background(), "club-with-no-players")
	if err != nil {
		t.Fatalf("Players() error = %v, want nil for an empty club", err)
	}
	if len(list) != 0 {
		t.Errorf("len(Players()) = %d, want 0", len(list))
	}
}

func TestPutPlayerOverwrites(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-overwrite"

	p := seedPlayer(t, store, clubID, "p-ow", "Before")
	p.LastName = "After"
	p.Positions = []core.Position{core.Blindside, core.NumberEight}
	if err := store.PutPlayer(context.Background(), clubID, p); err != nil {
		t.Fatalf("PutPlayer() error = %v", err)
	}

	got, err := store.Player(context.Background(), clubID, "p-ow")
	if err != nil {
		t.Fatalf("Player() error = %v", err)
	}
	if got.LastName != "After" {
		t.Errorf("LastName = %q, want After", got.LastName)
	}
	if len(got.Positions) != 2 {
		t.Errorf("Positions = %v, want two positions", got.Positions)
	}
}

func TestDeletePlayerRemovesIt(t *testing.T) {
	store := newTestStore(t)
	clubID := "club-delete"
	seedPlayer(t, store, clubID, "p-del", "Delete")

	if err := store.DeletePlayer(context.Background(), clubID, "p-del"); err != nil {
		t.Fatalf("DeletePlayer() error = %v", err)
	}
	if _, err := store.Player(context.Background(), clubID, "p-del"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Player() after delete error = %v, want core.ErrNotFound", err)
	}
}
