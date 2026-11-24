-- Preserve audit rows. Refuse a downgrade that would discard reviewed cycles or queued deliveries.
DO $$
BEGIN
    IF EXISTS (SELECT FROM sim_instruction_resends)
       OR EXISTS (SELECT FROM sim_manual_reviews WHERE released_at IS NOT NULL) THEN
        RAISE EXCEPTION 'admin order action history exists; retain the additive schema';
    END IF;
END $$;

DROP TABLE sim_instruction_resends;
DROP INDEX sim_manual_reviews_active_idx;
DROP INDEX sim_manual_reviews_history_idx;
ALTER TABLE sim_manual_reviews
    DROP CONSTRAINT sim_manual_reviews_pkey,
    DROP COLUMN id,
    DROP COLUMN released_by,
    DROP COLUMN released_at,
    ADD PRIMARY KEY (order_id);
