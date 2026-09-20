package fixture

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"pitch-ai/internal/core"
)

type memStore struct {
	matches map[string]core.Match // clubID/teamID/matchID -> match
	squad   []core.Player
}

func newMemStore() *memStore {
	return &memStore{matches: map[string]core.Match{}}
}

func key(clubID, teamID, matchID string) string { return clubID + "/" + teamID + "/" + matchID }

func (m *memStore) Match(_ context.Context, clubID, teamID, matchID string) (core.Match, error) {
	match, ok := m.matches[key(clubID, teamID, matchID)]
	if !ok {
		return core.Match{}, core.ErrNotFound
	}
	return match, nil
}

func (m *memStore) Matches(_ context.Context, clubID, teamID string) ([]core.Match, error) {
	var out []core.Match
	prefix := clubID + "/" + teamID + "/"
	for k, match := range m.matches {
		if strings.HasPrefix(k, prefix) {
			out = append(out, match)
		}
	}
	return out, nil
}

func (m *memStore) PutMatch(_ context.Context, clubID, teamID string, match core.Match) error {
	m.matches[key(clubID, teamID, match.ID)] = match
	return nil
}

func (m *memStore) Players(_ context.Context, _ string) ([]core.Player, error) {
	return m.squad, nil
}

// squadOf builds n players, all active members of team-1.
func squadOf(n int) []core.Player {
	players := make([]core.Player, 0, n)
	for i := 1; i <= n; i++ {
		players = append(players, core.Player{
			ID:        "player-" + strconv.Itoa(i),
			FirstName: "First",
			LastName:  "Last" + strconv.Itoa(i),
			Positions: []core.Position{core.Openside},
			TeamIDs:   []string{"team-1"},
			Status:    core.PlayerActive,
		})
	}
	return players
}

func fullLineup() core.Lineup {
	starters := make([]core.LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		starters = append(starters, core.LineupSlot{
			Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey),
		})
	}
	return core.Lineup{Starters: starters}
}

func newMatch() core.Match {
	return core.Match{
		SeasonID:    "2026-27",
		Opponent:    "Castelnau RC",
		KickoffAt:   time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Competition: "Regional 1",
		Venue:       core.VenueHome,
	}
}

func newService() (*Service, *memStore) {
	store := newMemStore()
	return NewService(store, store, store, func() string { return "match-generated" }), store
}

func scheduled(t *testing.T, svc *Service) core.Match {
	t.Helper()
	m, err := svc.Schedule(context.Background(), "club-1", "team-1", newMatch())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	return m
}

func TestScheduleDefaultsStatusAndAssignsIdentity(t *testing.T) {
	svc, _ := newService()

	got := scheduled(t, svc)
	if got.ID != "match-generated" || got.ClubID != "club-1" || got.TeamID != "team-1" {
		t.Errorf("identity = (%q, %q, %q), want (match-generated, club-1, team-1)",
			got.ID, got.ClubID, got.TeamID)
	}
	if got.Status != core.MatchScheduled {
		t.Errorf("Status = %q, want %q", got.Status, core.MatchScheduled)
	}
}

func TestScheduleIgnoresCallerSuppliedIdentity(t *testing.T) {
	svc, store := newService()

	m := newMatch()
	m.ID = "hijack"
	m.ClubID = "club-2"
	m.TeamID = "team-9"

	got, err := svc.Schedule(context.Background(), "club-1", "team-1", m)
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if got.ClubID != "club-1" || got.TeamID != "team-1" {
		t.Errorf("identity = (%q, %q), want (club-1, team-1)", got.ClubID, got.TeamID)
	}
	if _, leaked := store.matches[key("club-2", "team-9", "hijack")]; leaked {
		t.Error("match was written to the club and team named in the body")
	}
}

func TestScheduleRejectsInvalidMatch(t *testing.T) {
	svc, store := newService()

	m := newMatch()
	m.Opponent = ""

	if _, err := svc.Schedule(context.Background(), "club-1", "team-1", m); err == nil {
		t.Fatal("Schedule() error = nil, want a validation error")
	}
	if len(store.matches) != 0 {
		t.Error("an invalid match was persisted")
	}
}

func TestSetLineupAcceptsAValidSelection(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)

	match := scheduled(t, svc)

	got, err := svc.SetLineup(context.Background(), "club-1", "team-1", match.ID, fullLineup())
	if err != nil {
		t.Fatalf("SetLineup() error = %v", err)
	}
	if len(got.Lineup.Starters) != 15 {
		t.Errorf("len(Starters) = %d, want 15", len(got.Lineup.Starters))
	}
}

func TestSetLineupRejectsPlayerFromAnotherSquad(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)
	store.squad[0].TeamIDs = []string{"team-2"} // player-1 is a colt, not in team-1

	match := scheduled(t, svc)

	_, err := svc.SetLineup(context.Background(), "club-1", "team-1", match.ID, fullLineup())
	if err == nil {
		t.Fatal("SetLineup() error = nil, want an eligibility error")
	}
	var invalid core.ValidationError
	if !errors.As(err, &invalid) {
		t.Errorf("error = %v, want a core.ValidationError", err)
	}
}

func TestSetLineupRejectsInactivePlayer(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)
	store.squad[4].Status = core.PlayerInactive

	match := scheduled(t, svc)

	if _, err := svc.SetLineup(context.Background(), "club-1", "team-1", match.ID, fullLineup()); err == nil {
		t.Error("SetLineup() error = nil, want an error for an inactive player")
	}
}

func TestSetLineupOnUnknownMatchIsNotFound(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)

	_, err := svc.SetLineup(context.Background(), "club-1", "team-1", "no-such-match", fullLineup())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}

func TestSetLineupAcrossClubsIsNotFound(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)

	match := scheduled(t, svc)

	_, err := svc.SetLineup(context.Background(), "club-2", "team-1", match.ID, fullLineup())
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}

func TestSetLineupDoesNotPersistAnInvalidSelection(t *testing.T) {
	svc, store := newService()
	store.squad = squadOf(15)

	match := scheduled(t, svc)
	short := fullLineup()
	short.Starters = short.Starters[:14]

	if _, err := svc.SetLineup(context.Background(), "club-1", "team-1", match.ID, short); err == nil {
		t.Fatal("SetLineup() error = nil, want an error")
	}

	stored := store.matches[key("club-1", "team-1", match.ID)]
	if !stored.Lineup.IsEmpty() {
		t.Error("a rejected lineup was written to the match")
	}
}

func TestListIsScopedToTheTeam(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()

	if _, err := svc.Schedule(ctx, "club-1", "team-1", newMatch()); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if _, err := svc.Schedule(ctx, "club-1", "team-2", newMatch()); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	list, err := svc.List(ctx, "club-1", "team-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List()) = %d, want 1", len(list))
	}
}
