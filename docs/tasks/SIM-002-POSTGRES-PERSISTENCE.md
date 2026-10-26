# SIM-002 — PostgreSQL persistence foundation

```yaml
id: SIM-002
title: PostgreSQL repository, migrations, durable outbox and bot-to-API auth
product: sim
status: done
owner: human
repository: timon-platform
risk: medium
network_access: dependency-download-only
external_writes: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Scope

- Keep the existing in-memory mode for local fallback.
- Add `pgx/v5` PostgreSQL storage selected only by `SIM_DATABASE_URL`.
- Add explicit migration command; API startup must not mutate schema.
- Persist quote and paid-order pricing snapshots.
- Persist mock payment, fulfillment, delivery and activation state.
- Add payload-free durable outbox fields for lease, attempts, retry and manual review.
- Require bot-to-API bearer authentication whenever PostgreSQL mode is enabled.
- Add restart/idempotency integration test gated by a dedicated `SIM_TEST_DATABASE_URL`.

## Boundaries

- No VPN database, code, runtime or secrets.
- No real provider/payment calls or activation data.
- No production deployment.
- PostgreSQL integration tests may truncate only the explicitly supplied dedicated test database.
- Real activation data remains blocked until application-level encryption and key management are approved.

## Checks

```text
go test ./...
go vet ./...
SIM_TEST_DATABASE_URL=<dedicated test database> go test ./internal/postgres -run TestPostgresRestartAndIdempotency
git diff --check
```

## Result

Completed locally on 2026-10-22:

- PostgreSQL 18 test role and dedicated `timon_sim_test` database created on local port 5432;
- explicit migration command completed successfully;
- restart/idempotency integration test passed against the dedicated database;
- local `sim-api` switched from memory to PostgreSQL and retained a mock order across an actual API restart;
- authenticated API smoke returned `storage=postgres`, while an unauthenticated product request returned HTTP 401;
- unit tests and `go vet` passed;
- database credentials remain local and are not stored in Git;
- no VPN database, production runtime, deployment or GitHub remote was changed.
