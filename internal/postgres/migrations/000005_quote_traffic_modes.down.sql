-- The old check deliberately refuses rollback while stored package quotes have zero daily GB.
ALTER TABLE sim_quotes
    ADD CONSTRAINT sim_quotes_gigabytes_per_day_check CHECK (gigabytes_per_day BETWEEN 1 AND 5),
    DROP CONSTRAINT sim_quotes_traffic_mode_check;
