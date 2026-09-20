package httpapi

import (
	"net/http"
	"slices"

	"pitch-ai/internal/auth"
	"pitch-ai/internal/core"
)

// teamFromPath resolves the team in the URL and checks the caller may reach it.
//
// An admin reaches any team in their club — otherwise creating a squad would
// lock the creator out of it, since a new team is in nobody's membership yet.
// Coaches and analysts are limited to the teams they are listed against.
func teamFromPath(w http.ResponseWriter, r *http.Request) (auth.Membership, string, bool) {
	member, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return auth.Membership{}, "", false
	}

	teamID := r.PathValue("teamID")
	if member.Role != auth.RoleAdmin && !slices.Contains(member.TeamIDs, teamID) {
		writeError(w, http.StatusForbidden, "not a member of this squad")
		return auth.Membership{}, "", false
	}
	return member, teamID, true
}

func (d Deps) handleListTeams(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	teams, err := d.Teams.List(r.Context(), member.ClubID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}

	// Non-admins only see the squads they belong to.
	if member.Role != auth.RoleAdmin {
		teams = slices.DeleteFunc(teams, func(t core.Team) bool {
			return !slices.Contains(member.TeamIDs, t.ID)
		})
	}
	if teams == nil {
		teams = []core.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

func (d Deps) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	team, err := d.Teams.Get(r.Context(), member.ClubID, teamID)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (d Deps) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	member, _ := auth.FromContext(r.Context())

	var body core.Team
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	created, err := d.Teams.Create(r.Context(), member.ClubID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleDeleteTeam disbands a squad. Its fixtures and derived stats go with it;
// its players do not — they belong to the club and merely stop being members.
func (d Deps) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	if err := d.Teams.Delete(r.Context(), member.ClubID, teamID); err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	member, teamID, ok := teamFromPath(w, r)
	if !ok {
		return
	}

	var body core.Team
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	updated, err := d.Teams.Update(r.Context(), member.ClubID, teamID, body)
	if err != nil {
		d.writeDomainError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
