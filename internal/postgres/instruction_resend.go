package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RustamKhiz/timon-platform/internal/sim"
)

func (s *Store) QueueInstructionResend(ctx context.Context, id, actor, key string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE`, id)
	if err != nil {
		return err
	}
	var previousOrder, previousActor string
	err = tx.QueryRow(ctx, `SELECT order_id, actor FROM sim_instruction_resends WHERE id = $1`, key).
		Scan(&previousOrder, &previousActor)
	switch {
	case err == nil && previousOrder == id && previousActor == actor:
		return nil
	case err == nil:
		return sim.ErrIdempotencyConflict
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	if !order.CanResendInstruction() {
		return sim.ErrResendConflict
	}
	var busy bool
	if err := tx.QueryRow(ctx, `
        SELECT EXISTS (SELECT 1 FROM sim_instruction_resends
                        WHERE order_id = $1 AND (state IN ('pending', 'sending') OR created_at > $2))`,
		id, now.Add(-time.Minute)).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return sim.ErrResendConflict
	}
	result, err := tx.Exec(ctx, `
        INSERT INTO sim_instruction_resends (id, order_id, actor, created_at)
        VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, key, id, actor, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return sim.ErrIdempotencyConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) ClaimInstructionResend(ctx context.Context, now time.Time) (*sim.InstructionDelivery, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// A crash after Telegram accepted a message is indistinguishable from a failed send.
	if _, err := tx.Exec(ctx, `
        UPDATE sim_instruction_resends SET state = 'unknown', lease_until = NULL, finished_at = $1
         WHERE state = 'sending' AND lease_until <= $1`, now); err != nil {
		return nil, err
	}
	var job sim.InstructionDelivery
	var orderID string
	err = tx.QueryRow(ctx, `
        WITH next_job AS (
            SELECT id FROM sim_instruction_resends WHERE state = 'pending'
            ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
        )
        UPDATE sim_instruction_resends AS job SET state = 'sending', lease_until = $1
        FROM next_job WHERE job.id = next_job.id RETURNING job.id::text, job.order_id`,
		now.Add(90*time.Second)).Scan(&job.ID, &orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	job.Order, err = queryOrder(ctx, tx, `WHERE o.id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	if !job.Order.CanResendInstruction() {
		_, err := tx.Exec(ctx, `
            UPDATE sim_instruction_resends SET state = 'failed', lease_until = NULL, finished_at = $2 WHERE id = $1`,
			job.ID, now)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) FinishInstructionResend(ctx context.Context, id, state string, now time.Time) error {
	result, err := s.pool.Exec(ctx, `
        UPDATE sim_instruction_resends SET state = $2, lease_until = NULL, finished_at = $3
         WHERE id = $1 AND state = 'sending' AND lease_until > $3`, id, state, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return sim.ErrResendConflict
	}
	return nil
}

func readInstructionResends(ctx context.Context, tx pgx.Tx, order *sim.Order) error {
	rows, err := tx.Query(ctx, `
        SELECT id::text, actor, state, created_at, finished_at
          FROM sim_instruction_resends WHERE order_id = $1 ORDER BY created_at DESC, id DESC LIMIT 20`, order.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var attempt sim.InstructionResend
		if err := rows.Scan(&attempt.ID, &attempt.Actor, &attempt.State, &attempt.CreatedAt, &attempt.FinishedAt); err != nil {
			return err
		}
		order.InstructionResends = append(order.InstructionResends, attempt)
	}
	return rows.Err()
}
