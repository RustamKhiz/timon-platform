# TIMON SIM closed staging deployment

This repository deploys an allowlisted mock-only staging environment. It does not enable real payments, real provider purchases or public access.

## Runtime layout

- `sim-api`: Timeweb VPS, systemd service `timon-sim-api`, loopback `127.0.0.1:8182`.
- PostgreSQL: dedicated `timon_sim` role/database in an isolated PostgreSQL 18 container; never the VPN database.
- `sim-bot`: NL VPS, systemd service `timon-sim-bot`, one long-polling replica.
- Secrets: root-readable environment files outside Git.
- Public path: restricted HTTPS route from the fixed NL VPS IP to the API; no public database port.

## GitHub staging configuration

Create a protected `staging` environment with these secrets:

- `API_DEPLOY_HOST`
- `API_DEPLOY_USER`
- `API_DEPLOY_SSH_KEY`
- `BOT_DEPLOY_HOST`
- `BOT_DEPLOY_USER`
- `BOT_DEPLOY_SSH_KEY`

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
7. Add narrowly scoped sudo rules for the deploy user: start `timon-sim-migrate`, restart/status `timon-sim-api` and no other product service.
8. Add a separate restricted HTTPS Nginx route for the NL VPS, validate with `nginx -t`, then reload. Do not edit existing VPN locations inline without a saved rollback copy.

## NL one-time setup

1. Re-run the Amnezia and VPN-bot health baseline.
2. Create user `timon-sim` and `/opt/timon-platform/bot/{releases,shared}` without changing `/opt/timonhub`.
3. Store `/opt/timon-platform/bot/shared/.env` with mode `0640`, owned by `root:timon-sim`:

```env
TELEGRAM_BOT_TOKEN=<test bot token>
TELEGRAM_ALLOWED_USER_IDS=<closed allowlist>
SIM_API_URL=https://<restricted staging route>
SIM_API_TOKEN=<same bot-to-api secret as the API>
```

4. Install `deploy/systemd/timon-sim-bot.service` and add narrowly scoped sudo permission to restart/status only this unit.
5. Keep exactly one bot replica. Its durable Telegram offset is stored in `/var/lib/timon-sim-bot/update.offset`.

## Validation

- CI passes formatting, race tests, PostgreSQL integration and Linux builds.
- API health reports `storage=postgres`.
- API rejects missing bearer credentials.
- Bot `getMe` succeeds without logging the token or full URL.
- `/start`, purchase flow and `My eSIMs` work for an allowlisted user after API and bot restarts.
- VPN API/site/bot and both Amnezia containers remain healthy.

## Rollback

Each deployment remembers the previous release symlink. If migration, service startup or health fails, the workflow restores the previous binary and restarts only the affected SIM service. Database migrations are forward-only and are never destructively rolled back automatically.
