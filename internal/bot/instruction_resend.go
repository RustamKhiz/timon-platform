package bot

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (t *Telegram) runInstructionResends(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.processInstructionResend(ctx)
		}
	}
}

func (t *Telegram) processInstructionResend(ctx context.Context) {
	deliveryCtx, cancelDelivery := context.WithTimeout(ctx, 35*time.Second)
	defer cancelDelivery()
	var result struct {
		Delivery *sim.InstructionDelivery `json:"delivery"`
	}
	claimCtx, cancelClaim := context.WithTimeout(deliveryCtx, 10*time.Second)
	err := t.client.do(claimCtx, http.MethodPost, "/api/v1/sim/instruction-deliveries/claim", nil, "", &result)
	cancelClaim()
	if err != nil {
		if ctx.Err() == nil {
			t.logger.Warn("could not claim a SIM instruction resend")
		}
		return
	}
	if result.Delivery == nil {
		return
	}
	delivery := result.Delivery
	state := t.sendInstructionResend(deliveryCtx, delivery.Order)
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelFinish()
	payload := map[string]string{"state": state}
	path := "/api/v1/sim/instruction-deliveries/" + url.PathEscape(delivery.ID) + "/finish"
	if err := t.client.do(finishCtx, http.MethodPost, path, payload, "", nil); err != nil {
		t.logger.Warn("could not record SIM instruction resend outcome")
	}
}

func (t *Telegram) sendInstructionResend(ctx context.Context, order sim.Order) string {
	_, allowed := t.allowedUserIDs[order.TelegramUserID]
	if !allowed || !order.CanResendInstruction() || ctx.Err() != nil {
		return "failed"
	}
	response, err := activationResponse(order)
	if err != nil {
		return "failed"
	}
	sendCtx, cancelSend := context.WithTimeout(ctx, 20*time.Second)
	defer cancelSend()
	if err := t.send(sendCtx, order.TelegramUserID, response); err != nil {
		return "unknown"
	}
	return "sent"
}
