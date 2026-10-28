# AGENTS.md

`timon-platform` is an independent Git repository for new TIMON products. It is not part of a monorepo.

Current implementation scope is the TIMON SIM sandbox vertical slice and its closed,
allowlisted staging deployment. Read-only Keepgo sandbox catalogue access is approved.
Provider line creation, real provider purchases, real payments and public production
access remain out of scope until separately approved.

- Keep `sim-api` and `sim-bot` as separate commands and runtimes.
- Do not import TIMON VPN code or connect to VPN databases, payment tables, secrets, bots, or deployment.
- Never commit credentials, real eSIM activation data, production payloads, or personal data.
- Provider and payment integrations require explicit owner approval and dedicated adapters.
- Run Git commands from this repository only.
- Push and closed mock staging deployment were approved by the owner on
  2026-10-26. Do not deploy real integrations or change VPN production without
  separate explicit approval.
