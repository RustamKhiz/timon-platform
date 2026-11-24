package httpapi

import (
	"errors"
	"mime"
	"net/http"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (a *API) requestManualReview(w http.ResponseWriter, r *http.Request) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "application/json is required"})
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid manual review request"})
		return
	}
	order, err := a.service.RequestManualReview(r.Context(), r.PathValue("id"), a.config.AdminAuth.Username(), request.Reason)
	switch {
	case err == nil:
		writeAdminOrder(w, order)
	case errors.Is(err, sim.ErrInvalidReviewReason):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, sim.ErrReviewConflict), errors.Is(err, sim.ErrNotFound):
		writeServiceError(w, err)
	default:
		a.logger.Error("manual review failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record manual review"})
	}
}
