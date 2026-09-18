package httpapi

import (
	"net/http"

	"pitch-ai/internal/auth"
)

func handleMe(w http.ResponseWriter, r *http.Request) {
	member, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, member)
}
