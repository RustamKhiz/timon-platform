CREATE TABLE sim_quotes (
    id text PRIMARY KEY,
    provider_id text NOT NULL,
    country_code text NOT NULL,
    duration_days integer NOT NULL CHECK (duration_days BETWEEN 1 AND 30),
    gigabytes_per_day integer NOT NULL CHECK (gigabytes_per_day BETWEEN 1 AND 5),
    provider_cost_rub integer NOT NULL CHECK (provider_cost_rub >= 0),
    fixed_markup_rub integer NOT NULL CHECK (fixed_markup_rub >= 0),
    expected_fee_rub integer NOT NULL CHECK (expected_fee_rub >= 0),
    retail_rub integer NOT NULL CHECK (retail_rub >= 0),
    provider_rate_version text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE sim_orders (
    id text PRIMARY KEY,
    telegram_user_id bigint NOT NULL,
    idempotency_key text NOT NULL,
    quote_id text NOT NULL REFERENCES sim_quotes(id),
    provider_id text NOT NULL,
    country_code text NOT NULL,
    duration_days integer NOT NULL,
    gigabytes_per_day integer NOT NULL,
    provider_cost_rub integer NOT NULL,
    fixed_markup_rub integer NOT NULL,
    expected_fee_rub integer NOT NULL,
    retail_rub integer NOT NULL,
    provider_rate_version text NOT NULL,
    quote_created_at timestamptz NOT NULL,
    quote_expires_at timestamptz NOT NULL,
    order_status text NOT NULL,
    payment_status text NOT NULL,
    fulfillment_status text NOT NULL,
    delivery_status text NOT NULL,
    activation_sm_dp_address text,
    activation_code text,
    activation_manual_code text,
    activation_is_mock boolean,
    created_at timestamptz NOT NULL,
    paid_at timestamptz,
    fulfillment_deadline_at timestamptz,
    UNIQUE (telegram_user_id, idempotency_key)
);

CREATE INDEX sim_orders_user_created_idx
    ON sim_orders (telegram_user_id, created_at DESC);

CREATE TABLE sim_outbox (
    id bigserial PRIMARY KEY,
    event_type text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    state text NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'leased', 'done', 'manual_review')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL,
    lease_owner text,
    lease_until timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_type, entity_type, entity_id)
);

CREATE INDEX sim_outbox_ready_idx
    ON sim_outbox (state, next_attempt_at)
    WHERE state IN ('pending', 'leased');

COMMENT ON TABLE sim_outbox IS
    'Payload-free durable jobs: stores event type and entity id, never activation data.';

COMMENT ON COLUMN sim_orders.activation_code IS
    'Mock-only until application-level encryption is implemented for real provider activation data.';
