package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/squad"
)

// Deps holds everything the HTTP layer needs. It is populated once, in main.
type Deps struct {
	Logger *slog.Logger
	Assets fs.FS
	Auth   func(http.Handler) http.Handler
	Squad  *squad.Service
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
	}

	if d.Auth != nil && d.Squad != nil {
		mux.Handle("GET /api/players", d.protected(auth.ActionRead, d.handleListPlayers))
		mux.Handle("GET /api/players/{playerID}", d.protected(auth.ActionRead, d.handleGetPlayer))
		mux.Handle("POST /api/players", d.protected(auth.ActionManageSquad, d.handleCreatePlayer))
		mux.Handle("PUT /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleUpdatePlayer))
		mux.Handle("DELETE /api/players/{playerID}", d.protected(auth.ActionManageSquad, d.handleDeletePlayer))
	}

	// Nil in handler tests, which have no asset bundle to serve.
	if d.Assets != nil {
		mux.Handle("GET /", SPAHandler(d.Assets))
	}
	return mux
}
