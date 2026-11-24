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
        SELECT id, actor, reason, previous_status, created_at
          FROM sim_manual_reviews WHERE order_id = $1 AND released_at IS NULL`, id).
		Scan(&event.ID, &event.Actor, &event.Reason, &event.PreviousStatus, &event.CreatedAt)
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
	event := sim.ManualReviewEvent{Actor: actor, Reason: reason, PreviousStatus: order.OrderStatus}
	if err := tx.QueryRow(ctx, `
        INSERT INTO sim_manual_reviews (order_id, actor, reason, previous_status, created_at)
        VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		id, actor, reason, order.OrderStatus, now).Scan(&event.ID, &event.CreatedAt); err != nil {
		return sim.Order{}, fmt.Errorf("record manual review: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_orders SET order_status = 'manual_review' WHERE id = $1`, id); err != nil {
		return sim.Order{}, fmt.Errorf("set manual review status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, fmt.Errorf("commit manual review: %w", err)
	}
	order.ManualReview = &event
	order.OrderStatus = "manual_review"
	return order, nil
}

func (s *Store) ReleaseManualReview(ctx context.Context, id, actor string, reviewID int64, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE`, id)
	if err != nil {
		return err
	}
	var event sim.ManualReviewEvent
	err = tx.QueryRow(ctx, `
        SELECT previous_status, released_at FROM sim_manual_reviews WHERE id = $1 AND order_id = $2`,
		reviewID, id).Scan(&event.PreviousStatus, &event.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sim.ErrReviewConflict
	}
	if err != nil || event.ReleasedAt != nil {
		return err
	}
	order.ManualReview = &event
	if !order.CanReleaseManualReview() {
		return sim.ErrReviewConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_orders SET order_status = $2 WHERE id = $1`, id, event.PreviousStatus); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_manual_reviews SET released_by = $2, released_at = $3 WHERE id = $1`, reviewID, actor, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func readManualReviewHistory(ctx context.Context, tx pgx.Tx, order *sim.Order) error {
	rows, err := tx.Query(ctx, `
        SELECT id, actor, reason, previous_status, created_at, COALESCE(released_by, ''), released_at
          FROM sim_manual_reviews WHERE order_id = $1 ORDER BY id`, order.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var event sim.ManualReviewEvent
		if err := rows.Scan(&event.ID, &event.Actor, &event.Reason, &event.PreviousStatus, &event.CreatedAt, &event.ReleasedBy, &event.ReleasedAt); err != nil {
			return err
		}
		order.ManualReviews = append(order.ManualReviews, event)
		if event.ReleasedAt == nil && order.OrderStatus == "manual_review" {
			order.ManualReview = &event
		}
	}
	return rows.Err()
}
