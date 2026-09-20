package httpapi

import (
	"net/http"

	"pitch-ai/internal/core"
)

func (d Deps) handleListMatches(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	matches, err := d.Fixture.List(r.Context(), member.ClubID, teamID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	if matches == nil {
		matches = []core.Match{}
	}
	writeJSON(w, http.StatusOK, matches)
}

func (d Deps) handleGetMatch(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	match, err := d.Fixture.Get(r.Context(), member.ClubID, teamID, r.PathValue("matchID"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, match)
}

func (d Deps) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body core.Match
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	created, err := d.Fixture.Schedule(r.Context(), member.ClubID, teamID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (d Deps) handleSetLineup(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body core.Lineup
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	updated, err := d.Fixture.SetLineup(r.Context(), member.ClubID, teamID, r.PathValue("matchID"), body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
