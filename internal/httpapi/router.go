package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
)

// Deps holds everything the HTTP layer needs. It is populated once, in main.
type Deps struct {
	Logger *slog.Logger
	Assets fs.FS
	Auth   func(http.Handler) http.Handler
}

func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", handleHealth)

	if d.Auth != nil {
		mux.Handle("GET /api/me", d.Auth(http.HandlerFunc(handleMe)))
	}

	// Nil in handler tests, which have no asset bundle to serve.
	if d.Assets != nil {
		mux.Handle("GET /", SPAHandler(d.Assets))
	}
	return mux
}
