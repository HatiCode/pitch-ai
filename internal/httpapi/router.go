package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/fixture"
	"pitch-ai/internal/match"
	"pitch-ai/internal/squad"
)

// Deps holds everything the HTTP layer needs. It is populated once, in main.
type Deps struct {
	Logger  *slog.Logger
	Assets  fs.FS
	Auth    func(http.Handler) http.Handler
	Squad   *squad.Service
	Teams   *squad.TeamService
	Fixture *fixture.Service
	Match   *match.Service
}

// protected requires an authenticated caller whose role permits the action.
func (d Deps) protected(action auth.Action, h http.HandlerFunc) http.Handler {
	return d.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.FromContext(r.Context())
		if !ok || !member.Role.Can(action) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		h(w, r)
	}))
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", handleHealth)

	if d.Auth != nil {
		mux.Handle("GET /api/me", d.Auth(http.HandlerFunc(handleMe)))
		mux.Handle("GET /api/catalogue", d.protected(auth.ActionRead, handleCatalogue))
	}

	if d.Auth != nil && d.Squad != nil {
		mux.Handle("GET /api/players", d.protected(auth.ActionRead, d.handleListPlayers))
		mux.Handle("GET /api/players/{playerID}", d.protected(auth.ActionRead, d.handleGetPlayer))
		mux.Handle("POST /api/players", d.protected(auth.ActionManageSquad, d.handleCreatePlayer))
		mux.Handle("PUT /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleUpdatePlayer))
		mux.Handle("DELETE /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleDeletePlayer))
	}

	if d.Auth != nil && d.Teams != nil {
		mux.Handle("GET /api/teams", d.protected(auth.ActionRead, d.handleListTeams))
		mux.Handle("GET /api/teams/{teamID}", d.protected(auth.ActionRead, d.handleGetTeam))
		mux.Handle("POST /api/teams", d.protected(auth.ActionManageSquad, d.handleCreateTeam))
		mux.Handle("PUT /api/teams/{teamID}", d.protected(auth.ActionManageSquad, d.handleUpdateTeam))
		mux.Handle("DELETE /api/teams/{teamID}", d.protected(auth.ActionManageSquad, d.handleDeleteTeam))
	}

	if d.Auth != nil && d.Fixture != nil {
		mux.Handle("GET /api/teams/{teamID}/matches", d.protected(auth.ActionRead, d.handleListMatches))
		mux.Handle("GET /api/teams/{teamID}/matches/{matchID}", d.protected(auth.ActionRead, d.handleGetMatch))
		mux.Handle("POST /api/teams/{teamID}/matches", d.protected(auth.ActionEditMatch, d.handleCreateMatch))
		mux.Handle("PUT /api/teams/{teamID}/matches/{matchID}/lineup", d.protected(auth.ActionEditMatch, d.handleSetLineup))
	}

	if d.Auth != nil && d.Match != nil {
		mux.Handle("GET /api/teams/{teamID}/matches/{matchID}/events", d.protected(auth.ActionRead, d.handleListEvents))
		mux.Handle("POST /api/teams/{teamID}/matches/{matchID}/events", d.protected(auth.ActionTagMatch, d.handleAppendEvents))
		mux.Handle("GET /api/teams/{teamID}/matches/{matchID}/state", d.protected(auth.ActionRead, d.handleMatchState))
	}

	// Unmatched /api/ paths must not reach the SPA handler below: a client
	// hitting a mistyped endpoint should get a JSON 404, not an HTML page that
	// fails to parse. More specific patterns above still win.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})

	// Registered without a method so that "/api/" above is unambiguously more
	// specific: ServeMux panics on a pattern pair where one is narrower by
	// method and the other by path.
	//
	// Nil in handler tests, which have no asset bundle to serve.
	if d.Assets != nil {
		mux.Handle("/", SPAHandler(d.Assets))
	}
	return mux
}
