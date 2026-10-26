package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/RustamKhiz/timon-platform/internal/sim"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) SaveQuote(ctx context.Context, quote sim.Quote) error {
	_, err := s.pool.Exec(ctx, `
        INSERT INTO sim_quotes (
            id, provider_id, country_code, duration_days, gigabytes_per_day,
            provider_cost_rub, fixed_markup_rub, expected_fee_rub, retail_rub,
            provider_rate_version, created_at, expires_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		quote.ID, quote.ProviderID, quote.Selection.CountryCode, quote.Selection.DurationDays,
		quote.Selection.GigabytesPerDay, quote.ProviderCostRUB, quote.FixedMarkupRUB,
		quote.ExpectedFeeRUB, quote.RetailRUB, quote.ProviderRateVersion, quote.CreatedAt, quote.ExpiresAt)
	if err != nil {
		return fmt.Errorf("save quote: %w", err)
	}
	return nil
}

func (s *Store) CreateOrder(ctx context.Context, quoteID string, telegramUserID int64, idempotencyKey string, now time.Time) (sim.Order, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return sim.Order{}, fmt.Errorf("begin create order: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, fmt.Sprintf("%d:%s", telegramUserID, idempotencyKey)); err != nil {
		return sim.Order{}, fmt.Errorf("lock idempotency key: %w", err)
	}

	existing, err := queryOrder(ctx, tx, `WHERE o.telegram_user_id = $1 AND o.idempotency_key = $2`, telegramUserID, idempotencyKey)
	if err == nil {
		if existing.Quote.ID != quoteID {
			return sim.Order{}, sim.ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return sim.Order{}, fmt.Errorf("commit idempotent order read: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, sim.ErrNotFound) {
		return sim.Order{}, err
	}

	var quote sim.Quote
	err = tx.QueryRow(ctx, `
        SELECT id, provider_id, country_code, duration_days, gigabytes_per_day,
               provider_cost_rub, fixed_markup_rub, expected_fee_rub, retail_rub,
               provider_rate_version, created_at, expires_at
          FROM sim_quotes WHERE id = $1 FOR SHARE`, quoteID).Scan(
		&quote.ID, &quote.ProviderID, &quote.Selection.CountryCode, &quote.Selection.DurationDays,
		&quote.Selection.GigabytesPerDay, &quote.ProviderCostRUB, &quote.FixedMarkupRUB,
		&quote.ExpectedFeeRUB, &quote.RetailRUB, &quote.ProviderRateVersion, &quote.CreatedAt, &quote.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sim.Order{}, sim.ErrNotFound
	}
	if err != nil {
		return sim.Order{}, fmt.Errorf("load quote: %w", err)
	}
	if !now.Before(quote.ExpiresAt) {
		return sim.Order{}, sim.ErrQuoteExpired
	}
	order := sim.Order{
		ID: newOrderID(), TelegramUserID: telegramUserID, Quote: quote,
		OrderStatus: "awaiting_payment", PaymentStatus: "pending",
		FulfillmentStatus: "not_started", DeliveryStatus: "not_ready", CreatedAt: now,
	}
	_, err = tx.Exec(ctx, `
        INSERT INTO sim_orders (
            id, telegram_user_id, idempotency_key, quote_id, provider_id, country_code,
            duration_days, gigabytes_per_day, provider_cost_rub, fixed_markup_rub,
            expected_fee_rub, retail_rub, provider_rate_version, quote_created_at,
            quote_expires_at, order_status, payment_status, fulfillment_status,
            delivery_status, created_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		order.ID, order.TelegramUserID, idempotencyKey, quote.ID, quote.ProviderID,
		quote.Selection.CountryCode, quote.Selection.DurationDays, quote.Selection.GigabytesPerDay,
		quote.ProviderCostRUB, quote.FixedMarkupRUB, quote.ExpectedFeeRUB, quote.RetailRUB,
		quote.ProviderRateVersion, quote.CreatedAt, quote.ExpiresAt, order.OrderStatus,
		order.PaymentStatus, order.FulfillmentStatus, order.DeliveryStatus, order.CreatedAt)
	if err != nil {
		return sim.Order{}, fmt.Errorf("insert order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, fmt.Errorf("commit create order: %w", err)
	}
	return order, nil
}

func (s *Store) PayAndProvision(ctx context.Context, orderID string, telegramUserID int64, paidAt, deadline time.Time, activation sim.ActivationData) (sim.Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return sim.Order{}, fmt.Errorf("begin provisioning: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE`, orderID)
	if err != nil {
		return sim.Order{}, err
	}
	if order.TelegramUserID != telegramUserID {
		return sim.Order{}, sim.ErrForbidden
	}
	if order.FulfillmentStatus != "provisioned" {
		_, err = tx.Exec(ctx, `
            UPDATE sim_orders
               SET order_status = 'ready', payment_status = 'succeeded',
                   fulfillment_status = 'provisioned', delivery_status = 'pending',
                   activation_sm_dp_address = $2, activation_code = $3,
                   activation_manual_code = $4, activation_is_mock = $5,
                   paid_at = $6, fulfillment_deadline_at = $7
             WHERE id = $1`, orderID, activation.SMAddress, activation.ActivationCode,
			activation.ManualCode, activation.IsMock, paidAt, deadline)
		if err != nil {
			return sim.Order{}, fmt.Errorf("update provisioned order: %w", err)
		}
		_, err = tx.Exec(ctx, `
            INSERT INTO sim_outbox (event_type, entity_type, entity_id, state, next_attempt_at)
            VALUES ('deliver_esim', 'sim_order', $1, 'pending', $2)
            ON CONFLICT (event_type, entity_type, entity_id) DO NOTHING`, orderID, paidAt)
		if err != nil {
			return sim.Order{}, fmt.Errorf("insert delivery outbox event: %w", err)
		}
	}
	order, err = queryOrder(ctx, tx, `WHERE o.id = $1`, orderID)
	if err != nil {
		return sim.Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, fmt.Errorf("commit provisioning: %w", err)
	}
	return order, nil
}

func (s *Store) MarkDelivered(ctx context.Context, orderID string, telegramUserID int64, now time.Time) (sim.Order, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return sim.Order{}, fmt.Errorf("begin mark delivered: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	order, err := queryOrder(ctx, tx, `WHERE o.id = $1 FOR UPDATE`, orderID)
	if err != nil {
		return sim.Order{}, err
	}
	if order.TelegramUserID != telegramUserID {
		return sim.Order{}, sim.ErrForbidden
	}
	if order.FulfillmentStatus != "provisioned" {
		return sim.Order{}, errors.New("order is not provisioned")
	}
	if _, err := tx.Exec(ctx, `UPDATE sim_orders SET order_status = 'completed', delivery_status = 'sent' WHERE id = $1`, orderID); err != nil {
		return sim.Order{}, fmt.Errorf("mark order delivered: %w", err)
	}
	if _, err := tx.Exec(ctx, `
        UPDATE sim_outbox SET state = 'done', lease_until = NULL, updated_at = $2
         WHERE event_type = 'deliver_esim' AND entity_type = 'sim_order' AND entity_id = $1`, orderID, now); err != nil {
		return sim.Order{}, fmt.Errorf("complete delivery outbox event: %w", err)
	}
	order, err = queryOrder(ctx, tx, `WHERE o.id = $1`, orderID)
	if err != nil {
		return sim.Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sim.Order{}, fmt.Errorf("commit mark delivered: %w", err)
	}
	return order, nil
}

func (s *Store) GetOrder(ctx context.Context, orderID string, telegramUserID int64) (sim.Order, error) {
	order, err := queryOrder(ctx, s.pool, `WHERE o.id = $1`, orderID)
	if err != nil {
		return sim.Order{}, err
	}
	if order.TelegramUserID != telegramUserID {
		return sim.Order{}, sim.ErrForbidden
	}
	return order, nil
}

func (s *Store) OrdersForUser(ctx context.Context, telegramUserID int64) ([]sim.Order, error) {
	rows, err := s.pool.Query(ctx, orderSelect+` WHERE o.telegram_user_id = $1 ORDER BY o.created_at DESC`, telegramUserID)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()
	orders := make([]sim.Order, 0)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	return orders, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type scanner interface {
	Scan(...any) error
}

const orderSelect = `
    SELECT o.id, o.telegram_user_id, o.quote_id, o.provider_id, o.country_code,
           o.duration_days, o.gigabytes_per_day, o.provider_cost_rub,
           o.fixed_markup_rub, o.expected_fee_rub, o.retail_rub,
           o.provider_rate_version, o.quote_created_at, o.quote_expires_at,
           o.order_status, o.payment_status, o.fulfillment_status,
           o.delivery_status, o.activation_sm_dp_address, o.activation_code,
           o.activation_manual_code, o.activation_is_mock, o.created_at,
           o.paid_at, o.fulfillment_deadline_at
      FROM sim_orders o`

func queryOrder(ctx context.Context, db queryer, clause string, args ...any) (sim.Order, error) {
	order, err := scanOrder(db.QueryRow(ctx, orderSelect+" "+clause, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return sim.Order{}, sim.ErrNotFound
	}
	return order, err
}

func scanOrder(row scanner) (sim.Order, error) {
	var order sim.Order
	var activationAddress, activationCode, activationManual pgtype.Text
	var activationIsMock pgtype.Bool
	var paidAt, deadline pgtype.Timestamptz
	err := row.Scan(
		&order.ID, &order.TelegramUserID, &order.Quote.ID, &order.Quote.ProviderID,
		&order.Quote.Selection.CountryCode, &order.Quote.Selection.DurationDays,
		&order.Quote.Selection.GigabytesPerDay, &order.Quote.ProviderCostRUB,
		&order.Quote.FixedMarkupRUB, &order.Quote.ExpectedFeeRUB, &order.Quote.RetailRUB,
		&order.Quote.ProviderRateVersion, &order.Quote.CreatedAt, &order.Quote.ExpiresAt,
		&order.OrderStatus, &order.PaymentStatus, &order.FulfillmentStatus,
		&order.DeliveryStatus, &activationAddress, &activationCode, &activationManual,
		&activationIsMock, &order.CreatedAt, &paidAt, &deadline)
	if err != nil {
		return sim.Order{}, fmt.Errorf("scan order: %w", err)
	}
	if activationAddress.Valid && activationCode.Valid && activationManual.Valid && activationIsMock.Valid {
		order.Activation = &sim.ActivationData{
			SMAddress: activationAddress.String, ActivationCode: activationCode.String,
			ManualCode: activationManual.String, IsMock: activationIsMock.Bool,
		}
	}
	if paidAt.Valid {
		value := paidAt.Time
		order.PaidAt = &value
	}
	if deadline.Valid {
		value := deadline.Time
		order.FulfillmentDeadlineAt = &value
	}
	return order, nil
}

func newOrderID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return "o_" + hex.EncodeToString(buffer)
}
