package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func loadManualReview(ctx context.Context, db queryer, id string) (*sim.ManualReviewEvent, error) {
	var event sim.ManualReviewEvent
	err := db.QueryRow(ctx, `
        SELECT actor, reason, previous_status, created_at
          FROM sim_manual_reviews WHERE order_id = $1`, id).
		Scan(&event.Actor, &event.Reason, &event.PreviousStatus, &event.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manual review event: %w", err)
	}
	return &event, nil
}

func (s *Store) RequestManualReview(ctx context.Context, id, actor, reason string, now time.Time) (sim.Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return sim.Order{}, fmt.Errorf("begin manual review: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE`, id)
	if err != nil {
		return sim.Order{}, err
	}
	if order.OrderStatus == "manual_review" {
		event, err := loadManualReview(ctx, tx, id)
		if err != nil {
			return sim.Order{}, err
		}
		if event != nil && event.Actor == actor && event.Reason == reason {
			order.ManualReview = event
			return order, nil
		}
	}
	if !order.CanRequestManualReview() {
		return sim.Order{}, sim.ErrReviewConflict
	}
	var recordedAt time.Time
	if err := tx.QueryRow(ctx, `
        INSERT INTO sim_manual_reviews (order_id, actor, reason, previous_status, created_at)
        VALUES ($1, $2, $3, $4, $5) RETURNING created_at`,
		id, actor, reason, order.OrderStatus, now).Scan(&recordedAt); err != nil {
		return sim.Order{}, fmt.Errorf("record manual review: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_orders SET order_status = 'manual_review' WHERE id = $1`, id); err != nil {
		return sim.Order{}, fmt.Errorf("set manual review status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, fmt.Errorf("commit manual review: %w", err)
	}
	order.ManualReview = &sim.ManualReviewEvent{
		Actor: actor, Reason: reason, PreviousStatus: order.OrderStatus, CreatedAt: recordedAt,
	}
	order.OrderStatus = "manual_review"
	return order, nil
}
