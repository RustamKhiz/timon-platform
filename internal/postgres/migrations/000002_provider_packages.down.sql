ALTER TABLE sim_orders
    DROP COLUMN applied_markup_rub,
    DROP COLUMN fx_rate_rub_per_usd,
    DROP COLUMN provider_price_usd_cents,
    DROP COLUMN provider_bundle_id,
    DROP COLUMN total_data_mb,
    DROP COLUMN package_name,
    DROP COLUMN package_id;

ALTER TABLE sim_quotes
    DROP COLUMN applied_markup_rub,
    DROP COLUMN fx_rate_rub_per_usd,
    DROP COLUMN provider_price_usd_cents,
    DROP COLUMN provider_bundle_id,
    DROP COLUMN total_data_mb,
    DROP COLUMN package_name,
    DROP COLUMN package_id;

DELETE FROM sim_schema_migrations
WHERE name = '000002_provider_packages.up.sql';
