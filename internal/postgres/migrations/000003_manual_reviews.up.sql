CREATE TABLE sim_manual_reviews (
    order_id text PRIMARY KEY REFERENCES sim_orders(id),
    actor text NOT NULL,
    reason text NOT NULL CHECK (reason IN ('payment_check', 'order_data_check', 'support_request')),
    previous_status text NOT NULL,
    created_at timestamptz NOT NULL
);
