package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
	"pitch-ai/internal/fixture"
)

type fakeMatchStore struct {
	matches map[string]core.Match
	squad   []core.Player
}

func newFakeMatchStore() *fakeMatchStore {
	return &fakeMatchStore{matches: map[string]core.Match{}}
}

func matchKey(clubID, teamID, matchID string) string {
	return clubID + "/" + teamID + "/" + matchID
}

func (f *fakeMatchStore) Match(_ context.Context, clubID, teamID, matchID string) (core.Match, error) {
	m, ok := f.matches[matchKey(clubID, teamID, matchID)]
	if !ok {
		return core.Match{}, core.ErrNotFound
	}
	return m, nil
}

func (f *fakeMatchStore) Matches(_ context.Context, clubID, teamID string) ([]core.Match, error) {
	var out []core.Match
	for _, m := range f.matches {
		if m.ClubID == clubID && m.TeamID == teamID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMatchStore) PutMatch(_ context.Context, clubID, teamID string, m core.Match) error {
	f.matches[matchKey(clubID, teamID, m.ID)] = m
	return nil
}

func (f *fakeMatchStore) Players(_ context.Context, _ string) ([]core.Player, error) {
	return f.squad, nil
}

const validMatchJSON = `{"seasonId":"2026-27","opponent":"Castelnau RC","kickoffAt":"2026-03-14T15:00:00Z","competition":"Regional 1","venue":"home","status":"scheduled","lineup":{"starters":null,"bench":null}}`

func matchRouter(t *testing.T, member auth.Membership) (http.Handler, *fakeMatchStore) {
	t.Helper()
	store := newFakeMatchStore()
	svc := fixture.NewService(store, store, store, func() string { return "match-generated" })

	return NewRouter(Deps{
		Logger:  discardLogger(),
		Fixture: svc,
		Auth:    fakeAuthMiddleware(member),
	}), store
}

func squadFixture(n int) []core.Player {
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

func fullLineupJSON() string {
	slots := make([]core.LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		slots = append(slots, core.LineupSlot{Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey)})
	}
	body, err := json.Marshal(core.Lineup{Starters: slots})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func TestCreateMatch(t *testing.T) {
	router, store := matchRouter(t, admin("team-1"))

	rec := do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var got core.Match
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.TeamID != "team-1" || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (club-1, team-1)", got.ClubID, got.TeamID)
	}
	if _, ok := store.matches[matchKey("club-1", "team-1", "match-generated")]; !ok {
		t.Error("match was not persisted under the caller's club and team")
	}
}

func TestCreateMatchRejectsSquadOutsideMembership(t *testing.T) {
	router, _ := matchRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodPost, "/api/teams/team-99/matches", validMatchJSON)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a squad the caller does not belong to", rec.Code)
	}
}

func TestListMatchesEmptyIsJSONArray(t *testing.T) {
	router, _ := matchRouter(t, admin("team-1"))

	rec := do(t, router, http.MethodGet, "/api/teams/team-1/matches", "")
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want %q", got, "[]\n")
	}
}

func TestGetUnknownMatchIs404(t *testing.T) {
	router, _ := matchRouter(t, admin("team-1"))

	rec := do(t, router, http.MethodGet, "/api/teams/team-1/matches/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestSetLineupAcceptsAFullXV(t *testing.T) {
	router, store := matchRouter(t, admin("team-1"))
	store.squad = squadFixture(15)
	_ = do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)

	rec := do(t, router, http.MethodPut,
		"/api/teams/team-1/matches/match-generated/lineup", fullLineupJSON())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var got core.Match
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Lineup.Starters) != 15 {
		t.Errorf("len(Starters) = %d, want 15", len(got.Lineup.Starters))
	}
}

func TestSetLineupRejectsIncompleteXV(t *testing.T) {
	router, store := matchRouter(t, admin("team-1"))
	store.squad = squadFixture(15)
	_ = do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)

	body := `{"starters":[{"jersey":1,"playerId":"player-1"}],"bench":null}`
	rec := do(t, router, http.MethodPut, "/api/teams/team-1/matches/match-generated/lineup", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a one-man lineup", rec.Code)
	}

	var errBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Error("error response carries no message for the coach to act on")
	}
}

func TestCoachCanManageMatches(t *testing.T) {
	router, _ := matchRouter(t, coach("team-1"))

	if rec := do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON); rec.Code != http.StatusCreated {
		t.Errorf("POST status = %d, want 201 — a coach may create fixtures", rec.Code)
	}
	if rec := do(t, router, http.MethodGet, "/api/teams/team-1/matches", ""); rec.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200", rec.Code)
	}
}

func TestAnalystCanSetLineups(t *testing.T) {
	member := auth.Membership{UID: "uid-3", ClubID: "club-1", TeamIDs: []string{"team-1"}, Role: auth.RoleAnalyst}
	router, store := matchRouter(t, member)
	store.squad = squadFixture(15)
	_ = do(t, router, http.MethodPost, "/api/teams/team-1/matches", validMatchJSON)

	rec := do(t, router, http.MethodPut,
		"/api/teams/team-1/matches/match-generated/lineup", fullLineupJSON())
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — an analyst holds ActionEditMatch", rec.Code)
	}
}
