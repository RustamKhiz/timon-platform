# TIMON Platform

Independent Go repository for new TIMON products. The current code is a **mock-only TIMON SIM vertical slice** with an in-memory default and an optional PostgreSQL repository. It does not use VPN services, a real payment gateway, a real eSIM provider, production credentials, or deployment configuration.

## Implemented flow

```text
SIM Telegram bot
  -> country: Turkey / Egypt / UAE / India / Thailand / Vietnam / China
  -> duration: 1..30 days
  -> allowance: 1..5 GB per day
  -> 15-minute RUB quote
  -> idempotent order
  -> explicit mock payment
  -> synthetic mock eSIM activation data
  -> My eSIMs
```

The same temporary selection matrix is available for every country. A later provider adapter may replace it with provider-specific packages without changing the bot's ownership of navigation or the order's pricing snapshots.

Mock wholesale rates are synthetic development fixtures, not a commercial price list. Retail price uses the approved formula:

```text
retail_rub = mock_provider_cost_rub + fixed_markup_rub
```

The default markup is 100 RUB and can be changed only for new quotes with `SIM_FIXED_MARKUP_RUB`. Expected mock fees are zero, so the configured minimum remaining margin of 50 RUB is satisfied.

## Run locally

Requirements: Go 1.23 or newer.

Start the API:

```powershell
$env:SIM_API_TOKEN = '<random local bot-to-API credential>'
go run ./cmd/sim-api
```

Check it:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/healthz
Invoke-RestMethod http://127.0.0.1:18080/api/v1/sim/countries -Headers @{ Authorization = "Bearer $env:SIM_API_TOKEN" }
```

Start the bot in another terminal only after creating a separate Telegram test bot. Keep its token in the process environment; never commit it:

```powershell
$env:TELEGRAM_BOT_TOKEN = '<test token>'
$env:TELEGRAM_ALLOWED_USER_IDS = '<your numeric Telegram user ID>'
$env:SIM_API_URL = 'http://127.0.0.1:18080'
$env:SIM_API_TOKEN = '<same local bot-to-API credential>'
go run ./cmd/sim-bot
```

The closed test bot refuses to start without `TELEGRAM_ALLOWED_USER_IDS`; several IDs may be separated by commas. Keep IDs in local/server configuration and do not commit them. The bot supports `/start`, `/buy`, `/my` and `/support`, uses Telegram long polling and inline buttons, and marks every screen as `ТЕСТ / MOCK`. Sending the activation message is followed by a separate delivery acknowledgement to `sim-api`. A failed Telegram send therefore does not falsely mark the order completed.

For a persistent bot runtime, set `TELEGRAM_OFFSET_FILE` to a writable state path. The bot atomically checkpoints the next Telegram `update_id`, preventing already processed updates from being replayed after an ordinary restart.

## HTTP API

- `GET /healthz`
- `GET /api/v1/sim/countries`
- `POST /api/v1/sim/quotes`
- `POST /api/v1/sim/orders` with `Idempotency-Key`
- `GET /api/v1/sim/orders?telegram_user_id=...`
- `GET /api/v1/sim/orders/{id}?telegram_user_id=...`
- `POST /api/v1/sim/orders/{id}/mock-payment`
- `POST /api/v1/sim/orders/{id}/delivered`

Example quote body:

```json
{
  "country_code": "TR",
  "duration_days": 10,
  "gigabytes_per_day": 2
}
```

Without `SIM_DATABASE_URL`, data is in memory and disappears when `sim-api` stops. If `SIM_API_TOKEN` is set, every `/api/v1/sim/*` endpoint requires `Authorization: Bearer ...`; PostgreSQL mode refuses to start without this credential. The authenticated bot supplies `telegram_user_id`; arbitrary unauthenticated clients cannot use that identity contract.

## PostgreSQL mode

Use only a dedicated TIMON SIM database and role. Never point the migration command or integration test at VPN or another product database.

```powershell
$env:SIM_DATABASE_URL = '<dedicated PostgreSQL connection string>'
$env:SIM_API_TOKEN = '<random bot-to-API credential>'
go run ./cmd/sim-migrate
go run ./cmd/sim-api
```

Migrations are explicit and embedded into `sim-migrate`; API startup never mutates the schema. PostgreSQL stores quotes, immutable order pricing snapshots, mock fulfillment/delivery state and payload-free outbox jobs. The outbox supports leases, attempts, delayed retry and `manual_review`. Real activation data remains prohibited until encryption and key management are implemented.

## Checks

```powershell
go test ./...
go vet ./...
git diff --check
```

A real restart/idempotency check runs only against an explicitly supplied disposable database:

```powershell
$env:SIM_TEST_DATABASE_URL = '<dedicated disposable PostgreSQL database>'
go test ./internal/postgres -run TestPostgresRestartAndIdempotency
```

The check was completed successfully on 2026-10-22 against the dedicated local PostgreSQL 18 database `timon_sim_test`. Its credentials remain local and are not committed.

## Deferred gates

The following require separate owner decisions: real provider adapter and credentials, real prices, payment shop/gateway, webhook inbox, refunds, encryption/key management and any public production launch.

The owner approved a closed mock staging deployment and CI/CD on 2026-10-26. Its files and safeguards are documented in [DEPLOYMENT.md](DEPLOYMENT.md). This approval does not extend to public access, real provider purchases, payments or VPN changes.
