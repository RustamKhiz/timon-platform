# TIMON SIM closed staging deployment

This repository deploys an allowlisted sandbox staging environment. It reads packages
and wholesale prices from Keepgo Sandbox while payment and fulfillment remain mock.
It does not enable real payments, production provider purchases or public access.

## Runtime layout

- `sim-api`: Timeweb VPS, systemd service `timon-sim-api`, loopback `127.0.0.1:8182`.
- PostgreSQL: dedicated `timon_sim` role/database in an isolated PostgreSQL 18 container; never the VPN database. Its dedicated bridge network is not marked `internal`, because Docker does not publish loopback ports from an internal-only network; isolation is enforced by the explicit `127.0.0.1:55432` host binding.
- `sim-bot`: NL VPS, systemd service `timon-sim-bot`, one long-polling replica.
- Bot-to-API transport: Timeweb initiates a dedicated reverse SSH tunnel to NL. The NL endpoint listens only on `127.0.0.1:18082` and forwards to the Timeweb API loopback. This direction avoids the provider route that blocks NL-initiated SSH connections to Timeweb.
- Secrets: root-readable environment files outside Git.
- Public path: none. Neither the API nor the database has a public listening port.

## GitHub staging configuration

Create a protected `staging` environment with these secrets:

- `API_DEPLOY_HOST`
- `API_DEPLOY_USER`
- `API_DEPLOY_SSH_KEY`
- `BOT_DEPLOY_HOST`
- `BOT_DEPLOY_USER`
- `BOT_DEPLOY_SSH_KEY`
- `SIM_API_TOKEN`
- `SIM_DATABASE_PASSWORD`
- `SIM_TUNNEL_SSH_KEY`
- `SIM_TUNNEL_KNOWN_HOSTS`
- `KEEPGO_SANDBOX_API_KEY`
- `KEEPGO_SANDBOX_ACCESS_TOKEN`

`SIM_USD_RUB_RATE` is optional. If it is absent, the deployment job reads the
latest published CBR rate, adds 5%, and seals that value into the server config
as an outage fallback. At runtime the API still attempts to refresh the official
rate and caches successful responses. Existing quote and order snapshots remain
unchanged when the daily rate changes.

The pipeline renders the API and PostgreSQL environment files on the ephemeral
GitHub runner, uploads them together with the tracked PostgreSQL Compose file
and removes the uploaded copies after installation. Secret values must never
be committed or printed in CI logs.

The repository variable `STAGING_DEPLOY_ENABLED` defaults to absent/false. Keep it disabled until both servers have completed the one-time setup. A manual workflow dispatch may deploy after that setup; set the variable to `true` only when automatic deployment from `main` is desired.

## Timeweb one-time setup

1. Re-run the infrastructure preflight and confirm backup, firewall and VPN health.
2. Install Docker Engine and the Compose plugin without changing the VPN runtime.
3. Create a dedicated `timon-sim` service user and `/opt/timon-platform/api/{releases,shared}`.
4. Store `/opt/timon-platform/api/shared/.env` with mode `0640`, owned by `root:timon-sim`:

```env
SIM_API_ADDR=127.0.0.1:8182
SIM_API_TOKEN=<random bot-to-api secret>
SIM_DATABASE_URL=postgresql://timon_sim:<encoded password>@127.0.0.1:55432/timon_sim?sslmode=disable
SIM_FIXED_MARKUP_RUB=100
SIM_EXPECTED_PAYMENT_FEE_BPS=350
SIM_PROVIDER_MODE=keepgo-sandbox
SIM_FX_BUFFER_BPS=500
SIM_USD_RUB_RATE=<approved buffered USD/RUB rate>
KEEPGO_BASE_URL=https://myaccount.sandbox.keepgo.com/api/v2
KEEPGO_API_KEY=<sandbox API key>
KEEPGO_ACCESS_TOKEN=<sandbox access token>
```

5. Install the two systemd units from `deploy/systemd`.
6. Start the root-managed PostgreSQL stack from `deploy/postgres/compose.yaml`. It binds port `55432` to loopback only; never expose it externally.
7. Install `deploy/bootstrap/timon-sim-prepare-runtime` as root at `/usr/local/sbin/timon-sim-prepare-runtime`. Allow the deploy user to invoke it only for `/opt/timon-platform/api/releases/*`; the wrapper validates the immutable SHA release path, installs the two environment files with restricted permissions and starts only the dedicated PostgreSQL Compose project.
8. Add narrowly scoped sudo rules for the deploy user: invoke the runtime wrapper, start `timon-sim-migrate`, restart `timon-sim-api` and no other product service.
9. Install and enable `timon-sim-api-reverse-tunnel.service` under a dedicated `timon-sim-tunnel` system user. Store its private key and pinned NL host key under `/etc/timon-sim/`, unreadable by the API user.

## NL one-time setup

1. Re-run the Amnezia and VPN-bot health baseline.
2. Create user `timon-sim` and `/opt/timon-platform/bot/{releases,shared}` without changing `/opt/timonhub`.
3. Store `/opt/timon-platform/bot/shared/.env` with mode `0640`, owned by `root:timon-sim`:

```env
TELEGRAM_BOT_TOKEN=<test bot token>
TELEGRAM_ALLOWED_USER_IDS=<closed allowlist>
SIM_API_URL=http://127.0.0.1:18082
SIM_API_TOKEN=<same bot-to-api secret as the API>
```

4. Add the dedicated tunnel public key to the deploy account with `restrict,port-forwarding,permitlisten="127.0.0.1:18082"`. This key accepts only the Timeweb-initiated reverse forward and cannot open a shell.
5. Install `deploy/systemd/timon-sim-bot.service`; add narrowly scoped deploy sudo permission to restart/status only the SIM bot. The obsolete NL-initiated tunnel unit must remain disabled.
6. Keep exactly one bot replica. Its durable Telegram offset is stored in `/var/lib/timon-sim-bot/update.offset`.

## Validation

- CI passes formatting, race tests, PostgreSQL integration and Linux builds.
- API health reports `storage=postgres`.
- API rejects missing bearer credentials.
- The reverse tunnel listens only on NL `127.0.0.1:18082`, reconnects after either host restarts and keeps its private key isolated from the API process.
- Bot `getMe` succeeds without logging the token or full URL.
- `/start`, purchase flow and `My eSIMs` work for an allowlisted user after API and bot restarts.
- Country packages and wholesale prices come from Keepgo Sandbox; the final activation
  remains visibly synthetic until the durable sandbox line-creation workflow is enabled.
- VPN API/site/bot and both Amnezia containers remain healthy.

## Rollback

Each deployment remembers the previous release symlink. If migration, service startup or health fails, the workflow restores the previous binary and restarts only the affected SIM service. Database migrations are forward-only and are never destructively rolled back automatically.
