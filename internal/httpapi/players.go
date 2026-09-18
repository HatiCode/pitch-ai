package httpapi

import (
	"net/http"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

func (d Deps) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	players, err := d.Squad.List(r.Context(), member.ClubID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	// Encode an empty squad as [] rather than null so the client can map it.
	if players == nil {
		players = []core.Player{}
	}
	writeJSON(w, http.StatusOK, players)
}

func (d Deps) handleGetPlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	player, err := d.Squad.Get(r.Context(), member.ClubID, r.PathValue("playerID"))
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, player)
}

func (d Deps) handleCreatePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	var body core.Player
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	created, err := d.Squad.Add(r.Context(), member.ClubID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (d Deps) handleUpdatePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	var body core.Player
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	updated, err := d.Squad.Update(r.Context(), member.ClubID, r.PathValue("playerID"), body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (d Deps) handleDeletePlayer(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	if err := d.Squad.Remove(r.Context(), member.ClubID, r.PathValue("playerID")); err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
