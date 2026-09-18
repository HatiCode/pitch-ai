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

type fakeStore struct {
	players map[string]map[string]core.Player
}

func newFakeStore() *fakeStore {
	return &fakeStore{players: map[string]map[string]core.Player{}}
}

func (f *fakeStore) Player(_ context.Context, clubID, playerID string) (core.Player, error) {
	p, ok := f.players[clubID][playerID]
	if !ok {
		return core.Player{}, core.ErrNotFound
	}
	return p, nil
}

func (f *fakeStore) Players(_ context.Context, clubID string) ([]core.Player, error) {
	out := make([]core.Player, 0, len(f.players[clubID]))
	for _, p := range f.players[clubID] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastName < out[j].LastName })
	return out, nil
}

func (f *fakeStore) PutPlayer(_ context.Context, clubID string, p core.Player) error {
	if f.players[clubID] == nil {
		f.players[clubID] = map[string]core.Player{}
	}
	f.players[clubID][p.ID] = p
	return nil
}

func (f *fakeStore) DeletePlayer(_ context.Context, clubID, playerID string) error {
	if _, ok := f.players[clubID][playerID]; !ok {
		return core.ErrNotFound
	}
	delete(f.players[clubID], playerID)
	return nil
}

func routerForRole(t *testing.T, role auth.Role) (http.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	svc := squad.NewService(store, store, func() string { return "player-generated" })
	member := auth.Membership{UID: "uid-1", ClubID: "club-1", TeamIDs: []string{"team-1"}, Role: role}
	return newTestRouter(t, member, svc), store
}

func adminRouter(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	return routerForRole(t, auth.RoleAdmin)
}

const validPlayerJSON = `{"firstName":"Thomas","lastName":"Lefevre","dob":"1998-04-12","positions":["openside"],"teamIds":["team-1"],"status":"active"}`

func TestCreatePlayer(t *testing.T) {
	router, store := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players", validPlayerJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var got core.Player
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != "player-generated" || got.ClubID != "club-1" {
		t.Errorf("player = (%q, %q), want (player-generated, club-1)", got.ID, got.ClubID)
	}
	if _, ok := store.players["club-1"]["player-generated"]; !ok {
		t.Error("player was not persisted under the caller's club")
	}
}

func TestCreatePlayerIgnoresBodyClubID(t *testing.T) {
	router, store := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players",
		`{"firstName":"T","lastName":"L","positions":["openside"],"status":"active","clubId":"club-2"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if _, ok := store.players["club-2"]; ok {
		t.Error("player was written to the club named in the body, not the caller's club")
	}
	if _, ok := store.players["club-1"]["player-generated"]; !ok {
		t.Error("player was not written to the caller's club")
	}
}

func TestCreatePlayerRejectsUnknownFields(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players",
		`{"firstName":"T","lastName":"L","positions":["openside"],"status":"active","nickname":"Tommo"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unrecognised field", rec.Code)
	}
}

func TestCreatePlayerRejectsInvalidBody(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodPost, "/api/players",
		`{"firstName":"Thomas","lastName":"","positions":["openside"],"status":"active"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if body["error"] == "" {
		t.Error("error response has no message")
	}
}

func TestListPlayers(t *testing.T) {
	router, _ := adminRouter(t)
	_ = do(t, router, http.MethodPost, "/api/players", validPlayerJSON)

	rec := do(t, router, http.MethodGet, "/api/players", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []core.Player
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestListPlayersEmptyIsJSONArrayNotNull(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodGet, "/api/players", "")
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want %q — null would break Array.map in the client", got, "[]\n")
	}
}

func TestGetUnknownPlayerIs404(t *testing.T) {
	router, _ := adminRouter(t)

	rec := do(t, router, http.MethodGet, "/api/players/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUpdatePlayer(t *testing.T) {
	router, _ := adminRouter(t)
	_ = do(t, router, http.MethodPost, "/api/players", validPlayerJSON)

	body := `{"firstName":"Thomas","lastName":"Lefevre-Martin","dob":"1998-04-12","positions":["openside","blindside"],"teamIds":["team-1"],"status":"active"}`
	rec := do(t, router, http.MethodPut, "/api/players/player-generated", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var got core.Player
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.LastName != "Lefevre-Martin" || len(got.Positions) != 2 {
		t.Errorf("player = %+v, want the updated name and two positions", got)
	}
}

func TestDeletePlayer(t *testing.T) {
	router, _ := adminRouter(t)
	_ = do(t, router, http.MethodPost, "/api/players", validPlayerJSON)

	rec := do(t, router, http.MethodDelete, "/api/players/player-generated", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	if got := do(t, router, http.MethodGet, "/api/players/player-generated", ""); got.Code != http.StatusNotFound {
		t.Errorf("status after delete = %d, want 404", got.Code)
	}
}

func TestCoachCannotManageSquad(t *testing.T) {
	router, _ := routerForRole(t, auth.RoleCoach)

	if rec := do(t, router, http.MethodPost, "/api/players", validPlayerJSON); rec.Code != http.StatusForbidden {
		t.Errorf("POST status = %d, want 403", rec.Code)
	}
	if rec := do(t, router, http.MethodDelete, "/api/players/anything", ""); rec.Code != http.StatusForbidden {
		t.Errorf("DELETE status = %d, want 403", rec.Code)
	}
	if rec := do(t, router, http.MethodGet, "/api/players", ""); rec.Code != http.StatusOK {
		t.Errorf("GET status = %d, want 200 — a coach may read the squad", rec.Code)
	}
}

func TestAnalystCannotManageSquad(t *testing.T) {
	router, _ := routerForRole(t, auth.RoleAnalyst)

	if rec := do(t, router, http.MethodPost, "/api/players", validPlayerJSON); rec.Code != http.StatusForbidden {
		t.Errorf("POST status = %d, want 403", rec.Code)
	}
}
