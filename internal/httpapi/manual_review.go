package httpapi

import (
	"errors"
	"mime"
	"net/http"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (a *API) requestManualReview(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Reason string `json:"reason"`
	}
	if !readAdminAction(w, r, &request) {
		return
	}
	order, err := a.service.RequestManualReview(r.Context(), r.PathValue("id"), a.config.AdminAuth.Username(), request.Reason)
	switch {
	case err == nil:
		a.writeAdminOrder(w, order)
	case errors.Is(err, sim.ErrInvalidReviewReason):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, sim.ErrReviewConflict), errors.Is(err, sim.ErrNotFound):
		writeServiceError(w, err)
	default:
		a.logger.Error("manual review failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record manual review"})
	}
}

func readAdminAction(w http.ResponseWriter, r *http.Request, target any) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "application/json is required"})
		return false
	}
	if err := decodeJSON(r, target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid admin action request"})
		return false
	}
	return true
}

func (a *API) releaseManualReview(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ReviewID int64 `json:"review_id"`
	}
	if !readAdminAction(w, r, &request) {
		return
	}
	err := a.service.ReleaseManualReview(r.Context(), r.PathValue("id"), a.config.AdminAuth.Username(), request.ReviewID)
	a.writeOrderActionResult(w, err)
}
