# TIMON SIM closed staging deployment

This repository deploys an allowlisted mock-only staging environment. It does not enable real payments, real provider purchases or public access.

## Runtime layout

- `sim-api`: Timeweb VPS, systemd service `timon-sim-api`, loopback `127.0.0.1:8182`.
- PostgreSQL: dedicated `timon_sim` role/database in an isolated PostgreSQL 18 container; never the VPN database.
- `sim-bot`: NL VPS, systemd service `timon-sim-bot`, one long-polling replica.
- Bot-to-API transport: a dedicated `timon-sim-api-tunnel` systemd service on NL forwards only `127.0.0.1:18082` to the Timeweb API loopback over SSH.
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

The pipeline renders the API and PostgreSQL environment files on the ephemeral
GitHub runner, uploads them with the release and removes the uploaded copies
after installation. Secret values must never be committed or printed in CI
logs.

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
```

5. Install the two systemd units from `deploy/systemd`.
6. Start the root-managed PostgreSQL stack from `deploy/postgres/compose.yaml`. It binds port `55432` to loopback only; never expose it externally.
7. Install `deploy/bootstrap/timon-sim-prepare-runtime` as root at `/usr/local/sbin/timon-sim-prepare-runtime`. Allow the deploy user to invoke it only for `/opt/timon-platform/api/releases/*`; the wrapper validates the immutable SHA release path, installs the two environment files with restricted permissions and starts only the dedicated PostgreSQL Compose project.
8. Add narrowly scoped sudo rules for the deploy user: invoke the runtime wrapper, start `timon-sim-migrate`, restart `timon-sim-api` and no other product service.
9. Add the dedicated tunnel public key to `/home/deploy/.ssh/authorized_keys` with `restrict,port-forwarding,permitopen="127.0.0.1:8182"`. Do not grant shell commands or access to any VPN service through this key.

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

4. Store the dedicated tunnel private key and pinned Timeweb host key under `/etc/timon-sim/`, readable only by `timon-sim`.
5. Install `deploy/systemd/timon-sim-api-tunnel.service` and `deploy/systemd/timon-sim-bot.service`; enable the tunnel and add narrowly scoped deploy sudo permission to restart/status only these two units.
6. Keep exactly one bot replica. Its durable Telegram offset is stored in `/var/lib/timon-sim-bot/update.offset`.

## Validation

- CI passes formatting, race tests, PostgreSQL integration and Linux builds.
- API health reports `storage=postgres`.
- API rejects missing bearer credentials.
- The NL tunnel listens only on `127.0.0.1:18082`, reconnects after either host restarts and cannot forward to any other Timeweb destination.
- Bot `getMe` succeeds without logging the token or full URL.
- `/start`, purchase flow and `My eSIMs` work for an allowlisted user after API and bot restarts.
- VPN API/site/bot and both Amnezia containers remain healthy.

## Rollback

Each deployment remembers the previous release symlink. If migration, service startup or health fails, the workflow restores the previous binary and restarts only the affected SIM service. Database migrations are forward-only and are never destructively rolled back automatically.
