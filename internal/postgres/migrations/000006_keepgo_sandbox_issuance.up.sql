CREATE TABLE sim_keepgo_sandbox_issuance (
    order_id text PRIMARY KEY REFERENCES sim_orders(id),
    started_at timestamptz NOT NULL,
    sandbox_iccid text UNIQUE
);
