package squad

import (
	"context"
	"errors"
	"sort"
	"testing"

	"pitch-ai/internal/core"
)

// memTeamStore is an in-memory TeamReader + TeamWriter.
type memTeamStore struct {
	teams map[string]map[string]core.Team // clubID -> teamID -> team
}

func newMemTeamStore() *memTeamStore {
	return &memTeamStore{teams: map[string]map[string]core.Team{}}
}

func (m *memTeamStore) Team(_ context.Context, clubID, teamID string) (core.Team, error) {
	t, ok := m.teams[clubID][teamID]
	if !ok {
		return core.Team{}, core.ErrNotFound
	}
	return t, nil
}

func (m *memTeamStore) Teams(_ context.Context, clubID string) ([]core.Team, error) {
	out := make([]core.Team, 0, len(m.teams[clubID]))
	for _, t := range m.teams[clubID] {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *memTeamStore) PutTeam(_ context.Context, clubID string, t core.Team) error {
	if m.teams[clubID] == nil {
		m.teams[clubID] = map[string]core.Team{}
	}
	m.teams[clubID][t.ID] = t
	return nil
}

func (m *memTeamStore) DeleteTeam(_ context.Context, clubID, teamID string) error {
	if _, ok := m.teams[clubID][teamID]; !ok {
		return core.ErrNotFound
	}
	delete(m.teams[clubID], teamID)
	return nil
}

func newTeamService() (*TeamService, *memTeamStore) {
	svc, store, _ := newTeamServiceWithPlayers()
	return svc, store
}

func newTeamServiceWithPlayers() (*TeamService, *memTeamStore, *memStore) {
	teams := newMemTeamStore()
	players := newMemStore()
	return NewTeamService(teams, teams, players, func() string { return "team-generated" }), teams, players
}

func TestCreateTeamAssignsIdentityAndDefaultsActive(t *testing.T) {
	svc, _ := newTeamService()

	got, err := svc.Create(context.Background(), "club-1", core.Team{Name: "1st XV"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "team-generated" || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (team-generated, club-1)", got.ID, got.ClubID)
	}
	if !got.Active {
		t.Error("Active = false, want a new team to be active")
	}
}

func TestCreateTeamIgnoresCallerSuppliedIdentity(t *testing.T) {
	svc, store := newTeamService()

	got, err := svc.Create(context.Background(), "club-1", core.Team{
		ID: "hijack", ClubID: "club-2", Name: "Colts",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "team-generated" || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (team-generated, club-1)", got.ID, got.ClubID)
	}
	if _, leaked := store.teams["club-2"]; leaked {
		t.Error("team was written to the club named in the body")
	}
}

func TestCreateTeamRejectsInvalid(t *testing.T) {
	svc, store := newTeamService()

	if _, err := svc.Create(context.Background(), "club-1", core.Team{Name: ""}); err == nil {
		t.Fatal("Create() error = nil, want a validation error")
	}
	if len(store.teams["club-1"]) != 0 {
		t.Error("an invalid team was persisted")
	}
}

func TestUpdateTeamPreservesIdentity(t *testing.T) {
	svc, _ := newTeamService()

	created, err := svc.Create(context.Background(), "club-1", core.Team{Name: "1st XV"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := svc.Update(context.Background(), "club-1", created.ID, core.Team{
		ID: "hijack", ClubID: "club-2", Name: "1st XV (senior)", ShortName: "1XV", Active: true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.ID != created.ID || got.ClubID != "club-1" {
		t.Errorf("identity = (%q, %q), want (%q, club-1)", got.ID, got.ClubID, created.ID)
	}
	if got.Name != "1st XV (senior)" {
		t.Errorf("Name = %q, want the updated value", got.Name)
	}
}

func TestUpdateTeamAcrossClubsIsNotFound(t *testing.T) {
	svc, _ := newTeamService()

	created, err := svc.Create(context.Background(), "club-1", core.Team{Name: "1st XV"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = svc.Update(context.Background(), "club-2", created.ID, core.Team{Name: "Stolen"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Update() across clubs error = %v, want core.ErrNotFound", err)
	}
}

func TestListTeamsIsClubScoped(t *testing.T) {
	svc, _ := newTeamService()

	if _, err := svc.Create(context.Background(), "club-1", core.Team{Name: "1st XV"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Create(context.Background(), "club-2", core.Team{Name: "Other club"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.List(context.Background(), "club-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(List()) = %d, want 1", len(list))
	}
}

func TestDeleteTeamRemovesTheSquad(t *testing.T) {
	svc, store, _ := newTeamServiceWithPlayers()

	created, err := svc.Create(context.Background(), "club-1", core.Team{Name: "Colts"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "club-1", created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := store.teams["club-1"][created.ID]; ok {
		t.Error("team document survived deletion")
	}
}

func TestDeleteTeamKeepsPlayersWhoAreInOtherSquads(t *testing.T) {
	svc, teams, players := newTeamServiceWithPlayers()
	ctx := context.Background()

	if err := teams.PutTeam(ctx, "club-1", core.Team{ID: "team-doomed", Name: "Colts", Active: true}); err != nil {
		t.Fatalf("seeding team: %v", err)
	}
	dual := core.Player{
		ID: "p-dual", ClubID: "club-1", FirstName: "Thomas", LastName: "Lefevre",
		Positions: []core.Position{core.Openside}, TeamIDs: []string{"team-doomed", "team-keep"},
		Status: core.PlayerActive,
	}
	if err := players.PutPlayer(ctx, "club-1", dual); err != nil {
		t.Fatalf("seeding player: %v", err)
	}

	if err := svc.Delete(ctx, "club-1", "team-doomed"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err := players.Player(ctx, "club-1", "p-dual")
	if err != nil {
		t.Fatalf("player was deleted along with the squad: %v", err)
	}
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != "team-keep" {
		t.Errorf("TeamIDs = %v, want [team-keep] — only the deleted squad should be dropped", got.TeamIDs)
	}
}

func TestDeleteTeamKeepsAPlayerWhoseOnlySquadItWas(t *testing.T) {
	svc, teams, players := newTeamServiceWithPlayers()
	ctx := context.Background()

	if err := teams.PutTeam(ctx, "club-1", core.Team{ID: "team-only", Name: "Colts", Active: true}); err != nil {
		t.Fatalf("seeding team: %v", err)
	}
	solo := core.Player{
		ID: "p-solo", ClubID: "club-1", FirstName: "Marie", LastName: "Girard",
		Positions: []core.Position{core.Lock}, TeamIDs: []string{"team-only"},
		Status: core.PlayerActive,
	}
	if err := players.PutPlayer(ctx, "club-1", solo); err != nil {
		t.Fatalf("seeding player: %v", err)
	}

	if err := svc.Delete(ctx, "club-1", "team-only"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err := players.Player(ctx, "club-1", "p-solo")
	if err != nil {
		t.Fatalf("player was deleted with their only squad: %v", err)
	}
	if len(got.TeamIDs) != 0 {
		t.Errorf("TeamIDs = %v, want empty — the player stays on the club's books unassigned", got.TeamIDs)
	}
}

func TestDeleteTeamLeavesOtherClubsAlone(t *testing.T) {
	svc, teams, players := newTeamServiceWithPlayers()
	ctx := context.Background()

	if err := teams.PutTeam(ctx, "club-1", core.Team{ID: "shared-id", Name: "Ours", Active: true}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	other := core.Player{
		ID: "p-other", ClubID: "club-2", FirstName: "Someone", LastName: "Else",
		Positions: []core.Position{core.Hooker}, TeamIDs: []string{"shared-id"},
		Status: core.PlayerActive,
	}
	if err := players.PutPlayer(ctx, "club-2", other); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := svc.Delete(ctx, "club-1", "shared-id"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err := players.Player(ctx, "club-2", "p-other")
	if err != nil {
		t.Fatalf("looking up the other club's player: %v", err)
	}
	if len(got.TeamIDs) != 1 {
		t.Errorf("TeamIDs = %v, want the other club's player untouched", got.TeamIDs)
	}
}

func TestDeleteUnknownTeamIsNotFound(t *testing.T) {
	svc, _, _ := newTeamServiceWithPlayers()

	if err := svc.Delete(context.Background(), "club-1", "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Delete() error = %v, want core.ErrNotFound", err)
	}
}
