package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
	"pitch-ai/internal/fixture"
	"pitch-ai/internal/match"
)

// fakeEventStore adds an event log and derived documents to the match store the
// fixture tests already use, which is the whole surface match.Service consumes.
type fakeEventStore struct {
	*fakeMatchStore
	events map[string][]core.Event
	stats  map[string][]core.PlayerMatchStats

	// lastSince records the cursor the service was asked for, so a test can pin
	// that the query parameter reaches the store rather than being swallowed.
	lastSince string
}

func newFakeEventStore() *fakeEventStore {
	return &fakeEventStore{
		fakeMatchStore: newFakeMatchStore(),
		events:         map[string][]core.Event{},
		stats:          map[string][]core.PlayerMatchStats{},
	}
}

func (f *fakeEventStore) AppendEvents(_ context.Context, clubID, teamID, matchID string, events []core.Event) error {
	key := matchKey(clubID, teamID, matchID)
	f.events[key] = append(f.events[key], events...)
	return nil
}

func (f *fakeEventStore) Events(_ context.Context, clubID, teamID, matchID, since string) ([]core.Event, string, error) {
	f.lastSince = since

	log := f.events[matchKey(clubID, teamID, matchID)]
	from := 0
	if since != "" {
		n, err := strconv.Atoi(since)
		if err != nil {
			return nil, "", core.ValidationError{Field: "since", Reason: "malformed cursor"}
		}
		from = min(n, len(log))
	}
	return log[from:], strconv.Itoa(len(log)), nil
}

func (f *fakeEventStore) PutPlayerStats(_ context.Context, clubID, teamID, matchID string, stats []core.PlayerMatchStats) error {
	f.stats[matchKey(clubID, teamID, matchID)] = stats
	return nil
}

func analyst(teamIDs ...string) auth.Membership {
	return auth.Membership{UID: "uid-3", ClubID: "club-1", TeamIDs: teamIDs, Role: auth.RoleAnalyst}
}

func eventRouter(t *testing.T, member auth.Membership) (http.Handler, *fakeEventStore) {
	t.Helper()

	store := newFakeEventStore()
	svc := match.NewService(store, store, store, store, store)

	router := NewRouter(Deps{
		Logger: discardLogger(),
		Match:  svc,
		Auth:   fakeAuthMiddleware(member),
	})
	seedTaggableMatch(store, "club-1", "team-1", "match-1")
	return router, store
}

// seedTaggableMatch stores a match with a full XV selected, which is what an
// event naming a player is validated against.
func seedTaggableMatch(store *fakeEventStore, clubID, teamID, matchID string) {
	starters := make([]core.LineupSlot, 0, 15)
	for jersey := 1; jersey <= 15; jersey++ {
		starters = append(starters, core.LineupSlot{Jersey: jersey, PlayerID: "player-" + strconv.Itoa(jersey)})
	}

	store.matches[matchKey(clubID, teamID, matchID)] = core.Match{
		ID:        matchID,
		ClubID:    clubID,
		TeamID:    teamID,
		SeasonID:  "2026-27",
		Opponent:  "Castelnau RC",
		KickoffAt: time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC),
		Venue:     core.VenueHome,
		Status:    core.MatchScheduled,
		Lineup:    core.Lineup{Starters: starters},
	}
}

const (
	eventsPath  = "/api/teams/team-1/matches/match-1/events"
	statePath   = "/api/teams/team-1/matches/match-1/state"
	tackleBody  = `{"events":[{"id":"e1","kind":"tackle_made","playerId":"player-7","clockMs":60000,"period":1,"deviceId":"device-1"}]}`
	kickoffBody = `{"events":[{"id":"e0","kind":"period_started","clockMs":0,"period":1,"deviceId":"device-1"}]}`
)

func decodeState(t *testing.T, body io.Reader) core.MatchState {
	t.Helper()

	var state core.MatchState
	if err := json.NewDecoder(body).Decode(&state); err != nil {
		t.Fatalf("decoding state: %v", err)
	}
	return state
}

// The permission table's first real exercise: tagging is the one action an
// analyst does not hold.
func TestAppendEventsRequiresActionTagMatch(t *testing.T) {
	analystRouter, _ := eventRouter(t, analyst("team-1"))
	if rec := do(t, analystRouter, http.MethodPost, eventsPath, tackleBody); rec.Code != http.StatusForbidden {
		t.Errorf("analyst POST status = %d, want 403", rec.Code)
	}
	// Refusing to tag must not refuse to read: an analyst still reviews a match.
	if rec := do(t, analystRouter, http.MethodGet, eventsPath, ""); rec.Code != http.StatusOK {
		t.Errorf("analyst GET status = %d, want 200", rec.Code)
	}

	coachRouter, _ := eventRouter(t, coach("team-1"))
	if rec := do(t, coachRouter, http.MethodPost, eventsPath, tackleBody); rec.Code != http.StatusOK {
		t.Errorf("coach POST status = %d, want 200", rec.Code)
	}
}

func TestAppendEventsReturnsTheFoldedState(t *testing.T) {
	router, store := eventRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodPost, eventsPath, tackleBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	state := decodeState(t, rec.Body)
	if got := state.Players["player-7"].Counts[core.KindTackleMade]; got != 1 {
		t.Errorf("player-7 tackle_made = %d, want 1", got)
	}
	if state.Status != core.MatchInProgress {
		t.Errorf("status = %q, want in_progress once the log has an event", state.Status)
	}
	if len(store.stats[matchKey("club-1", "team-1", "match-1")]) != 15 {
		t.Errorf("derived documents = %d, want one per selected player",
			len(store.stats[matchKey("club-1", "team-1", "match-1")]))
	}
}

func TestAppendEventsMalformedBodyIs400(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodPost, eventsPath, `{"events":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if body["error"] != "malformed request body" {
		t.Errorf("error = %q, want %q", body["error"], "malformed request body")
	}
}

// A bare array leaves no room for a later field, and every other endpoint here
// takes an object.
func TestAppendEventsRejectsABareArray(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))

	body := `[{"id":"e1","kind":"tackle_made","playerId":"player-7","clockMs":60000,"period":1,"deviceId":"device-1"}]`
	if rec := do(t, router, http.MethodPost, eventsPath, body); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a top-level array", rec.Code)
	}
}

func TestAppendEventsValidationFailureCarriesTheField(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))

	// player-99 was never selected, so the event would hand them minutes they
	// did not play.
	body := `{"events":[{"id":"e1","kind":"tackle_made","playerId":"player-99","clockMs":60000,"period":1,"deviceId":"device-1"}]}`
	rec := do(t, router, http.MethodPost, eventsPath, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	var errBody map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if !strings.Contains(errBody["error"], "playerId") {
		t.Errorf("error = %q, want it to name the rejected field", errBody["error"])
	}
}

func TestAppendEventsToUnknownMatchIs404(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodPost, "/api/teams/team-1/matches/nope/events", tackleBody)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestEventRoutesRefuseANonMemberOfTheSquad(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))

	paths := map[string]string{
		http.MethodPost: "/api/teams/team-99/matches/match-1/events",
		http.MethodGet:  "/api/teams/team-99/matches/match-1/state",
	}
	for method, path := range paths {
		if rec := do(t, router, method, path, tackleBody); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403", method, path, rec.Code)
		}
	}
}

func TestListEventsPassesTheCursorThrough(t *testing.T) {
	router, store := eventRouter(t, coach("team-1"))
	_ = do(t, router, http.MethodPost, eventsPath, kickoffBody)
	_ = do(t, router, http.MethodPost, eventsPath, tackleBody)

	rec := do(t, router, http.MethodGet, eventsPath+"?since=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if store.lastSince != "1" {
		t.Errorf("cursor reaching the store = %q, want %q", store.lastSince, "1")
	}

	var page match.Page
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding page: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0].ID != "e1" {
		t.Errorf("events = %+v, want only the event after the cursor", page.Events)
	}
	if page.Cursor != "2" {
		t.Errorf("cursor = %q, want the position to resume from", page.Cursor)
	}
}

// The fixture and tagging routes share the /matches/{matchID} prefix, and
// ServeMux panics on a conflicting pattern pair when it is registered — which in
// main is at startup, on a deployed revision. Every other test wires one group
// at a time, so this is the only place that would catch it.
func TestFixtureAndEventRoutesCoexist(t *testing.T) {
	store := newFakeEventStore()
	router := NewRouter(Deps{
		Logger:  discardLogger(),
		Auth:    fakeAuthMiddleware(coach("team-1")),
		Fixture: fixture.NewService(store, store, store, func() string { return "match-generated" }),
		Match:   match.NewService(store, store, store, store, store),
	})
	seedTaggableMatch(store, "club-1", "team-1", "match-1")

	paths := []string{
		"/api/teams/team-1/matches/match-1",
		"/api/teams/team-1/matches/match-1/events",
		statePath,
	}
	for _, path := range paths {
		if rec := do(t, router, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
	}
}

func TestMatchStateFoldsTheStoredLog(t *testing.T) {
	router, _ := eventRouter(t, coach("team-1"))
	_ = do(t, router, http.MethodPost, eventsPath, kickoffBody)
	_ = do(t, router, http.MethodPost, eventsPath, tackleBody)

	rec := do(t, router, http.MethodGet, statePath, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	state := decodeState(t, rec.Body)
	if got := state.Players["player-7"].Counts[core.KindTackleMade]; got != 1 {
		t.Errorf("player-7 tackle_made = %d, want 1", got)
	}
	if !state.Running {
		t.Error("Running = false, want a clock started by period_started")
	}
}
