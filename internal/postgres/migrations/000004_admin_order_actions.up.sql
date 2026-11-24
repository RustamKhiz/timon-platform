ALTER TABLE sim_manual_reviews
    ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN released_by text,
    ADD COLUMN released_at timestamptz,
    ADD CHECK ((released_by IS NULL) = (released_at IS NULL)),
    DROP CONSTRAINT sim_manual_reviews_pkey,
    ADD PRIMARY KEY (id);

CREATE UNIQUE INDEX sim_manual_reviews_active_idx
    ON sim_manual_reviews (order_id) WHERE released_at IS NULL;
CREATE INDEX sim_manual_reviews_history_idx ON sim_manual_reviews (order_id, id);

CREATE TABLE sim_instruction_resends (
    id uuid PRIMARY KEY,
    order_id text NOT NULL REFERENCES sim_orders(id),
    actor text NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'sending', 'sent', 'failed', 'unknown')),
    created_at timestamptz NOT NULL,
    lease_until timestamptz,
    finished_at timestamptz,
    CHECK ((state = 'sending') = (lease_until IS NOT NULL))
);

CREATE UNIQUE INDEX sim_instruction_resends_active_idx
    ON sim_instruction_resends (order_id) WHERE state IN ('pending', 'sending');
CREATE INDEX sim_instruction_resends_history_idx ON sim_instruction_resends (order_id, created_at DESC);
CREATE INDEX sim_instruction_resends_pending_idx ON sim_instruction_resends (created_at) WHERE state = 'pending';
