package postgres

import (
	"context"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (s *Store) BeginKeepgoSandboxIssuance(ctx context.Context, id string, userID int64, now time.Time) (sim.Order, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return sim.Order{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE OF o`, id)
	if err != nil {
		return sim.Order{}, false, err
	}
	if order.TelegramUserID != userID || userID <= 0 {
		return sim.Order{}, false, sim.ErrForbidden
	}
	if order.Quote.ProviderID != sim.KeepgoSandboxProviderID {
		return sim.Order{}, false, sim.ErrSandboxIssuance
	}
	if order.FulfillmentStatus == "provisioning" {
		if order.PaidAt != nil && now.Sub(*order.PaidAt) >= time.Minute {
			if _, err := tx.Exec(ctx, keepgoSandboxReview, id); err != nil {
				return sim.Order{}, false, err
			}
			order.OrderStatus, order.FulfillmentStatus = "manual_review", "manual_review"
			return order, false, tx.Commit(ctx)
		}
		return order, false, nil
	}
	if order.FulfillmentStatus == "manual_review" || order.FulfillmentStatus == "provisioned" {
		return order, false, nil
	}
	if !order.CanRequestManualReview() {
		return sim.Order{}, false, sim.ErrReviewConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sim_keepgo_sandbox_issuance (order_id, started_at) VALUES ($1, $2)`, id, now); err != nil {
		return sim.Order{}, false, err
	}
	deadline := now.Add(time.Minute)
	if _, err := tx.Exec(ctx, `
        UPDATE sim_orders SET order_status = 'provisioning', fulfillment_status = 'provisioning',
            payment_status = 'succeeded', paid_at = $2, fulfillment_deadline_at = $3 WHERE id = $1`, id, now, deadline); err != nil {
		return sim.Order{}, false, err
	}
	order.OrderStatus, order.FulfillmentStatus = "provisioning", "provisioning"
	order.PaymentStatus, order.PaidAt, order.FulfillmentDeadlineAt = "succeeded", &now, &deadline
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, false, err
	}
	return order, true, nil
}

const keepgoSandboxReview = `UPDATE sim_orders SET order_status = 'manual_review', fulfillment_status = 'manual_review'
    WHERE id = $1 AND fulfillment_status = 'provisioning'
      AND EXISTS (SELECT FROM sim_keepgo_sandbox_issuance WHERE order_id = $1)`

func (s *Store) ReviewKeepgoSandboxIssuance(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, keepgoSandboxReview, id)
	return err
}

func (s *Store) FinishKeepgoSandboxIssuance(ctx context.Context, id string, line sim.SandboxLine) (sim.Order, error) {
	activation := line.Activation
	if !activation.IsSandbox || activation.IsMock || line.ICCID == "" {
		return sim.Order{}, sim.ErrSandboxIssuance
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return sim.Order{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result, err := tx.Exec(ctx, `
        UPDATE sim_orders SET order_status = 'ready', fulfillment_status = 'provisioned', delivery_status = 'pending',
            activation_sm_dp_address = $2, activation_code = $3, activation_manual_code = $4, activation_is_mock = false
        WHERE id = $1 AND fulfillment_status = 'provisioning'
          AND EXISTS (SELECT FROM sim_keepgo_sandbox_issuance WHERE order_id = $1)`,
		id, activation.SMAddress, activation.ActivationCode, activation.ManualCode)
	if err != nil {
		return sim.Order{}, err
	}
	if result.RowsAffected() != 1 {
		return sim.Order{}, sim.ErrSandboxIssuance
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_keepgo_sandbox_issuance SET sandbox_iccid = $2 WHERE order_id = $1`, id, line.ICCID); err != nil {
		return sim.Order{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sim_outbox (event_type, entity_type, entity_id, state, next_attempt_at)
        VALUES ('deliver_esim', 'sim_order', $1, 'pending', now()) ON CONFLICT DO NOTHING`, id); err != nil {
		return sim.Order{}, err
	}
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1`, id)
	if err != nil {
		return sim.Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, err
	}
	return order, nil
}
