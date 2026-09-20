package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
	"pitch-ai/internal/squad"
)

type fakeTeamStore struct {
	teams map[string]map[string]core.Team
}

func newFakeTeamStore() *fakeTeamStore {
	return &fakeTeamStore{teams: map[string]map[string]core.Team{}}
}

func (f *fakeTeamStore) Team(_ context.Context, clubID, teamID string) (core.Team, error) {
	t, ok := f.teams[clubID][teamID]
	if !ok {
		return core.Team{}, core.ErrNotFound
	}
	return t, nil
}

func (f *fakeTeamStore) Teams(_ context.Context, clubID string) ([]core.Team, error) {
	out := make([]core.Team, 0, len(f.teams[clubID]))
	for _, t := range f.teams[clubID] {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeTeamStore) PutTeam(_ context.Context, clubID string, t core.Team) error {
	if f.teams[clubID] == nil {
		f.teams[clubID] = map[string]core.Team{}
	}
	f.teams[clubID][t.ID] = t
	return nil
}

func (f *fakeTeamStore) DeleteTeam(_ context.Context, clubID, teamID string) error {
	if _, ok := f.teams[clubID][teamID]; !ok {
		return core.ErrNotFound
	}
	delete(f.teams[clubID], teamID)
	return nil
}

func teamRouter(t *testing.T, member auth.Membership) (http.Handler, *fakeTeamStore) {
	router, store, _ := teamRouterWithPlayers(t, member)
	return router, store
}

func teamRouterWithPlayers(t *testing.T, member auth.Membership) (http.Handler, *fakeTeamStore, *fakeStore) {
	t.Helper()
	teams := newFakeTeamStore()
	players := newFakeStore()
	svc := squad.NewTeamService(teams, teams, players, func() string { return "team-generated" })

	return NewRouter(Deps{
		Logger: discardLogger(),
		Teams:  svc,
		Auth:   fakeAuthMiddleware(member),
	}), teams, players
}

func admin(teamIDs ...string) auth.Membership {
	return auth.Membership{UID: "uid-1", ClubID: "club-1", TeamIDs: teamIDs, Role: auth.RoleAdmin}
}

func coach(teamIDs ...string) auth.Membership {
	return auth.Membership{UID: "uid-2", ClubID: "club-1", TeamIDs: teamIDs, Role: auth.RoleCoach}
}

func TestCreateTeam(t *testing.T) {
	router, store := teamRouter(t, admin())

	rec := do(t, router, http.MethodPost, "/api/teams", `{"name":"1st XV","shortName":"1XV","active":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var got core.Team
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != "team-generated" || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (team-generated, club-1)", got.ID, got.ClubID)
	}
	if _, ok := store.teams["club-1"]["team-generated"]; !ok {
		t.Error("team was not persisted under the caller's club")
	}
}

// The reason admins bypass the membership check: a freshly created squad is in
// nobody's TeamIDs, so its creator would be locked out of it immediately.
func TestAdminCanOpenATeamTheyJustCreated(t *testing.T) {
	router, _ := teamRouter(t, admin())

	create := do(t, router, http.MethodPost, "/api/teams", `{"name":"1st XV","shortName":"1XV","active":true}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", create.Code)
	}

	rec := do(t, router, http.MethodGet, "/api/teams/team-generated", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — an admin must reach a squad they just created", rec.Code)
	}
}

func TestCoachCannotOpenATeamTheyAreNotIn(t *testing.T) {
	router, store := teamRouter(t, coach("team-other"))
	if err := store.PutTeam(context.Background(), "club-1", core.Team{ID: "team-1", Name: "1st XV", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	rec := do(t, router, http.MethodGet, "/api/teams/team-1", "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestCoachCanOpenTheirOwnTeam(t *testing.T) {
	router, store := teamRouter(t, coach("team-1"))
	if err := store.PutTeam(context.Background(), "club-1", core.Team{ID: "team-1", Name: "1st XV", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	rec := do(t, router, http.MethodGet, "/api/teams/team-1", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestListTeamsFiltersForNonAdmins(t *testing.T) {
	router, store := teamRouter(t, coach("team-1"))
	ctx := context.Background()
	if err := store.PutTeam(ctx, "club-1", core.Team{ID: "team-1", Name: "1st XV", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := store.PutTeam(ctx, "club-1", core.Team{ID: "team-2", Name: "Colts", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	rec := do(t, router, http.MethodGet, "/api/teams", "")
	var got []core.Team
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != 1 || got[0].ID != "team-1" {
		t.Errorf("teams = %+v, want only team-1", got)
	}
}

func TestListTeamsShowsAllForAdmin(t *testing.T) {
	router, store := teamRouter(t, admin())
	ctx := context.Background()
	if err := store.PutTeam(ctx, "club-1", core.Team{ID: "team-1", Name: "1st XV", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := store.PutTeam(ctx, "club-1", core.Team{ID: "team-2", Name: "Colts", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	rec := do(t, router, http.MethodGet, "/api/teams", "")
	var got []core.Team
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(teams) = %d, want 2", len(got))
	}
}

func TestListTeamsEmptyIsJSONArray(t *testing.T) {
	router, _ := teamRouter(t, admin())

	rec := do(t, router, http.MethodGet, "/api/teams", "")
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want %q", got, "[]\n")
	}
}

func TestCoachCannotCreateTeams(t *testing.T) {
	router, _ := teamRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodPost, "/api/teams", `{"name":"Sneaky XV","shortName":"","active":true}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — squad creation is admin-only", rec.Code)
	}
}

func TestDeleteTeamDetachesPlayersWithoutDeletingThem(t *testing.T) {
	router, teams, players := teamRouterWithPlayers(t, admin())
	ctx := context.Background()

	if err := teams.PutTeam(ctx, "club-1", core.Team{ID: "team-1", Name: "Colts", Active: true}); err != nil {
		t.Fatalf("seeding team: %v", err)
	}
	if err := players.PutPlayer(ctx, "club-1", core.Player{
		ID: "p-1", ClubID: "club-1", FirstName: "Thomas", LastName: "Lefevre",
		Positions: []core.Position{core.Openside}, TeamIDs: []string{"team-1", "team-2"},
		Status: core.PlayerActive,
	}); err != nil {
		t.Fatalf("seeding player: %v", err)
	}

	rec := do(t, router, http.MethodDelete, "/api/teams/team-1", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	if _, ok := teams.teams["club-1"]["team-1"]; ok {
		t.Error("team survived deletion")
	}
	got, err := players.Player(ctx, "club-1", "p-1")
	if err != nil {
		t.Fatalf("player was deleted along with the squad: %v", err)
	}
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != "team-2" {
		t.Errorf("TeamIDs = %v, want [team-2]", got.TeamIDs)
	}
}

func TestDeleteUnknownTeamIs404(t *testing.T) {
	router, _ := teamRouter(t, admin())

	rec := do(t, router, http.MethodDelete, "/api/teams/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestCoachCannotDeleteTeams(t *testing.T) {
	router, store := teamRouter(t, coach("team-1"))
	if err := store.PutTeam(context.Background(), "club-1", core.Team{ID: "team-1", Name: "1st XV", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	rec := do(t, router, http.MethodDelete, "/api/teams/team-1", "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — disbanding a squad is admin-only", rec.Code)
	}
	if _, ok := store.teams["club-1"]["team-1"]; !ok {
		t.Error("team was deleted despite the 403")
	}
}
