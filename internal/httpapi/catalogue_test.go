package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

func catalogueRouter(t *testing.T, member auth.Membership) http.Handler {
	t.Helper()

	return NewRouter(Deps{
		Logger: discardLogger(),
		Auth:   fakeAuthMiddleware(member),
	})
}

func TestCatalogueReturnsEveryEntry(t *testing.T) {
	router := catalogueRouter(t, coach("team-1"))

	rec := do(t, router, http.MethodGet, "/api/catalogue", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var got []core.CatalogueEntry
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding catalogue: %v", err)
	}
	if len(got) != len(core.Catalogue()) {
		t.Fatalf("len(catalogue) = %d, want %d", len(got), len(core.Catalogue()))
	}

	// The client builds its buttons from this, so a kind missing here is a kind
	// no coach can tag.
	served := make(map[core.EventKind]core.CatalogueEntry, len(got))
	for _, entry := range got {
		served[entry.Kind] = entry
	}
	for _, kind := range core.AllEventKinds() {
		if _, ok := served[kind]; !ok {
			t.Errorf("kind %q is missing from the served catalogue", kind)
		}
	}

	if tackle := served[core.KindTackleMade]; tackle.Group != core.GroupPlay || tackle.Label == "" || !tackle.Player {
		t.Errorf("tackle_made entry = %+v, want a labelled, player-attributed play kind", tackle)
	}
	if try := served[core.KindTry]; try.Points != 5 {
		t.Errorf("try points = %d, want 5", try.Points)
	}
}

// The catalogue is configuration, not public data: it describes a club's tagging
// setup and is served only to a caller the auth middleware has placed a role on.
func TestCatalogueRefusesARoleWithoutRead(t *testing.T) {
	stranger := auth.Membership{UID: "uid-9", ClubID: "club-1", Role: auth.Role("spectator")}
	router := catalogueRouter(t, stranger)

	if rec := do(t, router, http.MethodGet, "/api/catalogue", ""); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a role holding no permissions", rec.Code)
	}
}

// With no auth middleware there is no caller to authorise, so the route must not
// exist at all rather than serve the catalogue to anyone who asks.
func TestCatalogueIsUnroutedWithoutAuth(t *testing.T) {
	router := NewRouter(Deps{Logger: discardLogger()})

	if rec := do(t, router, http.MethodGet, "/api/catalogue", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no authentication is configured", rec.Code)
	}
}
