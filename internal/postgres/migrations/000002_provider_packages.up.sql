ALTER TABLE sim_quotes
    ADD COLUMN package_id text NOT NULL DEFAULT '',
    ADD COLUMN package_name text NOT NULL DEFAULT '',
    ADD COLUMN total_data_mb integer NOT NULL DEFAULT 0 CHECK (total_data_mb >= 0),
    ADD COLUMN provider_bundle_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN provider_price_usd_cents integer NOT NULL DEFAULT 0 CHECK (provider_price_usd_cents >= 0),
    ADD COLUMN fx_rate_rub_per_usd numeric(12,4) NOT NULL DEFAULT 0 CHECK (fx_rate_rub_per_usd >= 0),
    ADD COLUMN applied_markup_rub integer NOT NULL DEFAULT 0 CHECK (applied_markup_rub >= 0);

UPDATE sim_quotes
SET applied_markup_rub = fixed_markup_rub
WHERE applied_markup_rub = 0;

ALTER TABLE sim_orders
    ADD COLUMN package_id text NOT NULL DEFAULT '',
    ADD COLUMN package_name text NOT NULL DEFAULT '',
    ADD COLUMN total_data_mb integer NOT NULL DEFAULT 0 CHECK (total_data_mb >= 0),
    ADD COLUMN provider_bundle_id bigint NOT NULL DEFAULT 0,
    ADD COLUMN provider_price_usd_cents integer NOT NULL DEFAULT 0 CHECK (provider_price_usd_cents >= 0),
    ADD COLUMN fx_rate_rub_per_usd numeric(12,4) NOT NULL DEFAULT 0 CHECK (fx_rate_rub_per_usd >= 0),
    ADD COLUMN applied_markup_rub integer NOT NULL DEFAULT 0 CHECK (applied_markup_rub >= 0);

UPDATE sim_orders
SET applied_markup_rub = fixed_markup_rub
WHERE applied_markup_rub = 0;
