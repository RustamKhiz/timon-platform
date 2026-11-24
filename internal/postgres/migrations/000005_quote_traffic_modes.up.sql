ALTER TABLE sim_quotes
    DROP CONSTRAINT sim_quotes_gigabytes_per_day_check,
    ADD CONSTRAINT sim_quotes_traffic_mode_check CHECK (
        (package_id = '' AND gigabytes_per_day BETWEEN 1 AND 5)
        OR (package_id <> '' AND gigabytes_per_day = 0 AND total_data_mb > 0)
    );
