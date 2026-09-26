package httpapi

import (
	"net/http"

	"pitch-ai/internal/core"
)

// handleCatalogue serves the event catalogue: which kinds exist, what they are
// called, where they belong on the tagging screen and what they score.
//
// Served rather than compiled into the client so that adding a kind is a
// redeploy of one binary, and so a second club can eventually be given a
// different catalogue without a fork.
func handleCatalogue(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, core.Catalogue())
}
