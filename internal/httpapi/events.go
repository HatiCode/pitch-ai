package httpapi

import (
	"net/http"

	"pitch-ai/internal/core"
)

// appendEventsRequest wraps the batch in an object rather than taking a bare
// array: a top-level array leaves no room for a field this endpoint has not
// needed yet, and every other endpoint here takes an object.
type appendEventsRequest struct {
	Events []core.Event `json:"events"`
}

// handleAppendEvents takes a batch from a tagging device and answers with the
// state the whole log now implies, so the client can reconcile against numbers
// the server computed rather than trust its own.
func (d Deps) handleAppendEvents(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body appendEventsRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	state, err := d.Match.Append(r.Context(), member.ClubID, teamID, r.PathValue("matchID"), body.Events)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// handleListEvents serves the log from a cursor, which is how a second device
// catches up on what the first one tagged.
func (d Deps) handleListEvents(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	page, err := d.Match.Events(r.Context(), member.ClubID, teamID,
		r.PathValue("matchID"), r.URL.Query().Get("since"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleMatchState folds the stored log for a caller with no local copy to fold
// itself — a laptop opening a match the iPad tagged.
func (d Deps) handleMatchState(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	state, err := d.Match.State(r.Context(), member.ClubID, teamID, r.PathValue("matchID"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
