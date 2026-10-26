package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type OutboxEvent struct {
	ID            int64
	EventType     string
	EntityType    string
	EntityID      string
	State         string
	Attempts      int
	NextAttemptAt time.Time
	LeaseOwner    string
	LeaseUntil    time.Time
}

func (s *Store) LeaseOutbox(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (OutboxEvent, error) {
	if workerID == "" || leaseDuration <= 0 {
		return OutboxEvent{}, errors.New("worker ID and positive lease duration are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("begin outbox lease: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var event OutboxEvent
	err = tx.QueryRow(ctx, `
        SELECT id, event_type, entity_type, entity_id, state, attempts, next_attempt_at
          FROM sim_outbox
         WHERE next_attempt_at <= $1
           AND (state = 'pending' OR (state = 'leased' AND lease_until <= $1))
         ORDER BY next_attempt_at, id
         FOR UPDATE SKIP LOCKED
         LIMIT 1`, now).Scan(
		&event.ID, &event.EventType, &event.EntityType, &event.EntityID,
		&event.State, &event.Attempts, &event.NextAttemptAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutboxEvent{}, simNoWork
	}
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("select outbox event: %w", err)
	}
	event.State = "leased"
	event.Attempts++
	event.LeaseOwner = workerID
	event.LeaseUntil = now.Add(leaseDuration)
	_, err = tx.Exec(ctx, `
        UPDATE sim_outbox
           SET state = 'leased', attempts = $2, lease_owner = $3,
               lease_until = $4, updated_at = $5
         WHERE id = $1`, event.ID, event.Attempts, event.LeaseOwner, event.LeaseUntil, now)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("lease outbox event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OutboxEvent{}, fmt.Errorf("commit outbox lease: %w", err)
	}
	return event, nil
}

func (s *Store) CompleteOutbox(ctx context.Context, eventID int64, workerID string, now time.Time) error {
	result, err := s.pool.Exec(ctx, `
        UPDATE sim_outbox
           SET state = 'done', lease_owner = NULL, lease_until = NULL,
               last_error = NULL, updated_at = $3
         WHERE id = $1 AND state = 'leased' AND lease_owner = $2`, eventID, workerID, now)
	if err != nil {
		return fmt.Errorf("complete outbox event: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("outbox lease is not owned by worker")
	}
	return nil
}

func (s *Store) RetryOutbox(ctx context.Context, eventID int64, workerID, errorCode string, nextAttemptAt, now time.Time, manualReview bool) error {
	state := "pending"
	if manualReview {
		state = "manual_review"
	}
	if len(errorCode) > 100 {
		errorCode = errorCode[:100]
	}
	result, err := s.pool.Exec(ctx, `
        UPDATE sim_outbox
           SET state = $3, next_attempt_at = $4, lease_owner = NULL,
               lease_until = NULL, last_error = $5, updated_at = $6
         WHERE id = $1 AND state = 'leased' AND lease_owner = $2`,
		eventID, workerID, state, nextAttemptAt, errorCode, now)
	if err != nil {
		return fmt.Errorf("retry outbox event: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("outbox lease is not owned by worker")
	}
	return nil
}

var simNoWork = errors.New("no outbox work available")

func IsNoOutboxWork(err error) bool {
	return errors.Is(err, simNoWork)
}
