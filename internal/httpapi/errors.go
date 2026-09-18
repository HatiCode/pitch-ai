package httpapi

import (
	"errors"
	"net/http"

	"pitch-ai/internal/core"
)

// writeDomainError maps domain errors onto status codes. Anything unrecognised
// is a 500 with the detail logged rather than returned.
func (d Deps) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid core.ValidationError
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Error())
	default:
		d.Logger.Error("unhandled error", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
