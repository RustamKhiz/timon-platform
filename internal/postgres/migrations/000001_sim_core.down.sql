DROP TABLE IF EXISTS sim_outbox;
DROP TABLE IF EXISTS sim_orders;
DROP TABLE IF EXISTS sim_quotes;

DELETE FROM sim_schema_migrations
WHERE name = '000001_sim_core.up.sql';
