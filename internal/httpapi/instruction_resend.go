package httpapi

import (
	"errors"
	"net/http"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (a *API) queueInstructionResend(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !readAdminAction(w, r, &request) {
		return
	}
	if a.config.BotToken == "" {
		a.writeOrderActionResult(w, sim.ErrResendUnavailable)
		return
	}
	err := a.service.QueueInstructionResend(r.Context(), r.PathValue("id"), a.config.AdminAuth.Username(), r.Header.Get("Idempotency-Key"))
	a.writeOrderActionResult(w, err)
}

func (a *API) claimInstructionResend(w http.ResponseWriter, r *http.Request) {
	delivery, err := a.service.ClaimInstructionResend(r.Context())
	if err != nil {
		a.writeOrderActionResult(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Delivery *sim.InstructionDelivery `json:"delivery,omitempty"`
	}{Delivery: delivery})
}

func (a *API) finishInstructionResend(w http.ResponseWriter, r *http.Request) {
	var request struct {
		State string `json:"state"`
	}
	if !readAdminAction(w, r, &request) {
		return
	}
	err := a.service.FinishInstructionResend(r.Context(), r.PathValue("id"), request.State)
	a.writeOrderActionResult(w, err)
}

func (a *API) writeOrderActionResult(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, sim.ErrInvalidResend):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, sim.ErrResendUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	case errors.Is(err, sim.ErrResendConflict), errors.Is(err, sim.ErrReviewConflict), errors.Is(err, sim.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, sim.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		a.logger.Error("SIM order action failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to complete SIM order action"})
	}
}
