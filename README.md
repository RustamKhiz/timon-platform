# TIMON Platform

Independent Go repository for new TIMON products. The current code is a TIMON SIM test vertical slice with an in-memory default and an optional PostgreSQL repository. It can read package names and wholesale prices from the Keepgo sandbox, but payments and eSIM fulfillment remain mock. It does not use VPN services, a real payment gateway, production provider credentials, or production provider purchases.

## Implemented flow

```text
SIM Telegram bot
  -> country: Turkey / Egypt / UAE / India / Thailand / Vietnam / China
  -> provider-native package: total data + validity period
  -> 15-minute RUB quote
  -> idempotent order
  -> explicit mock payment
  -> synthetic mock eSIM activation data
  -> mock QR and iPhone/Android installation instructions
  -> My eSIMs and repeated activation display
```

Mock mode exposes a small provider-shaped package list. In `keepgo-sandbox` mode
the API reads country eSIM packages from Keepgo, removes duplicate offers with
the same data/validity combination, and keeps the cheapest wholesale option.
Packages longer than 30 days, pSIM products and sub-1-GB packages are excluded
from the first TIMON SIM catalogue. Unlimited offers are labelled separately.

Mock wholesale rates are synthetic development fixtures, not a commercial price list. Retail price uses the approved formula:

```text
provider_cost_rub = ceil(provider_price_usd * configured_fx_rate)
retail_rub = max(
  provider_cost_rub + fixed_markup_rub,
  price required to retain the minimum margin after the expected payment fee
)
```

The default fixed markup is 100 RUB. `SIM_EXPECTED_PAYMENT_FEE_BPS` defaults to
350 basis points as a planning assumption until the separate payment shop has
contractual fees. Expensive packages receive a larger applied markup when needed
to retain at least 50 RUB before taxes and support. Quotes and orders snapshot
the provider bundle, USD price, FX rate, configured/applied markup and expected fee.

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

The closed test bot refuses to start without `TELEGRAM_ALLOWED_USER_IDS`; several IDs may be separated by commas. Keep IDs in local/server configuration and do not commit them. The bot supports `/start`, `/buy`, `/my` and `/support`, uses Telegram long polling and inline buttons, and marks the purchase flow as sandbox/test. Mock activation data is encoded into a PNG QR and can be shown again from `Мои eSIM`; it remains synthetic and cannot activate a real profile. Sending the activation photo is followed by a separate delivery acknowledgement to `sim-api`. A failed Telegram send therefore does not falsely mark the order completed.

The deployed sandbox stage currently reads the real Keepgo Sandbox catalogue but does
not call `/line/create`. Keepgo Sandbox can return a test ICCID and LPA code with the
production-shaped response, but it does not issue a real installable eSIM. Provider
line creation will be enabled only after the order state machine prevents duplicate
external purchases and preserves ambiguous outcomes for manual review.

For a persistent bot runtime, set `TELEGRAM_OFFSET_FILE` to a writable state path. The bot atomically checkpoints the next Telegram `update_id`, preventing already processed updates from being replayed after an ordinary restart.

## HTTP API

- `GET /healthz`
- `GET /api/v1/sim/countries`
- `GET /api/v1/sim/packages?country_code=TR`
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
  "package_id": "keepgo:29:data:20480:30"
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

## Keepgo sandbox catalogue

The standalone sandbox catalogue probe performs read-only requests for the
account balance and bundles. The same read-only client can supply packages to
`sim-api` when `SIM_PROVIDER_MODE=keepgo-sandbox`; neither path buys a line or
deploys provider credentials.

Required process environment variables:

```text
KEEPGO_BASE_URL=https://myaccount.sandbox.keepgo.com/api/v2
KEEPGO_API_KEY=<sandbox API key>
KEEPGO_ACCESS_TOKEN=<sandbox access token>
```

Run the probe from this repository:

```powershell
.\scripts\keepgo-sandbox-catalog.ps1
```

The JSON report contains the test balance and wholesale `price_usd` refills for
bundles covering Turkey, Egypt, the UAE, India, Thailand, Vietnam, and China.
Credentials are sent only in the `apiKey` and `accessToken` headers and are never
printed or saved by the helper. The report is written to the ignored local file
`.local/keepgo-sandbox-catalog.json`. Keepgo production credentials must not be
used while the integration remains sandbox-only.

To run `sim-api` against the read-only Keepgo sandbox catalogue, supply sandbox
credentials only in the process environment:

```powershell
$env:SIM_PROVIDER_MODE = 'keepgo-sandbox'
$env:SIM_USD_RUB_RATE = '84.5572'
$env:KEEPGO_API_KEY = '<sandbox API key>'
$env:KEEPGO_ACCESS_TOKEN = '<sandbox access token>'
go run ./cmd/sim-api
```

The API normally loads the latest published USD/RUB rate from the official CBR
XML endpoint, applies `SIM_FX_BUFFER_BPS` (500 by default), and caches it for six
hours. `SIM_USD_RUB_RATE` is only an optional outage fallback and does not need
daily manual updates. When the staging variable is absent, CI calculates and
seals the buffered fallback automatically during deployment. Every quote
snapshots the actual rate and source version.
Sandbox catalogue mode remains paired with mock payment and mock fulfillment;
it never calls Keepgo line-creation endpoints.

## Deferred gates

The following require separate owner decisions: production provider credentials and purchases, real payment shop/gateway, webhook inbox, refunds, encryption/key management, provider failover and any public production launch.

The owner approved a closed mock staging deployment and CI/CD on 2026-10-26. Its files and safeguards are documented in [DEPLOYMENT.md](DEPLOYMENT.md). This approval does not extend to public access, real provider purchases, payments or VPN changes.
