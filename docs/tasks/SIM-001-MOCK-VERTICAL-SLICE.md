# SIM-001 — local mock vertical slice

```yaml
id: SIM-001
title: Local TIMON SIM API and Telegram bot mock slice
product: sim
status: ready-for-owner
owner: human
repository: timon-platform
base_ref: empty repository approved by owner
worktree_required: false
risk: medium
planner_model_class: strong
worker_model_class: strong
reviewer_model_class: strong
allowed_paths:
  - "**"
forbidden_paths:
  - "production/**"
network_access: none
external_writes: false
human_approval_before_write: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Контекст

Владелец одобрил создание отдельного `timon-platform` и локальную подготовку TIMON SIM без реального provider и payment gateway. Каталог пока унифицирован: страна, срок от 1 до 30 дней и объём от 1 до 5 ГБ в день.

## Цель

Локально пройти путь `страна -> дни -> ГБ/день -> quote -> order -> mock payment -> mock eSIM` через независимые `sim-api` и `sim-bot`.

## Scope

- Семь согласованных стран: Турция, Египет, ОАЭ, Индия, Таиланд, Вьетнам и Китай.
- Provider-independent selection and quote model.
- In-memory repository, mock payment and mock provider.
- Telegram long-polling UI with inline buttons.
- HTTP API for catalog, quotes, orders, mock payment and owned order listing.

## Non-goals

- PostgreSQL, migrations, Docker and production deployment.
- Real provider, payment gateway, webhook, refund or QR image generation.
- Changes to VPN repositories or production.

## Контракты и ограничения

- `duration_days`: integer 1..30.
- `gigabytes_per_day`: integer 1..5.
- Fixed markup is configurable and defaults to 100 RUB.
- Quote TTL is 15 minutes; paid order stores immutable pricing snapshots.
- Mock data must be unmistakably synthetic.
- No external Go dependencies.

## План реализации

1. Add domain, catalog, pricing, in-memory store and mock adapters.
2. Add HTTP API and Telegram bot commands.
3. Add tests and local run documentation.

## Критерии готовности

- [x] Full local scenario is available.
- [x] Validation rejects values outside the approved ranges.
- [x] Order creation is idempotent.
- [x] No credentials or real activation data are present.
- [x] Mandatory checks pass.

## Обязательные проверки

```text
go test ./...
go vet ./...
git diff --check
```

## Условия остановки и эскалации

- A real external dependency, payment/provider request, persistent database or production access becomes necessary.
- A change outside `timon-platform` is required.

## Rollback

Before the first commit, remove the newly created `timon-platform` directory only after owner approval. After a commit, use a normal Git revert.

## Отчёт Coder

- Добавлены независимые `sim-api` и `sim-bot`, mock domain/service, HTTP client, Telegram long polling UI и тесты.
- `go test ./...`, `go vet ./...`, `gofmt -l cmd internal` и `git diff --cached --check` через временный index прошли.
- Локальный smoke на `127.0.0.1:18080` подтвердил 7 стран, пределы 30 дней/5 ГБ в день и переход `quote -> order -> mock payment -> provisioned`.
- Реальный Telegram API не проверялся: тестовый token не передавался и внешний write не разрешался.
- PostgreSQL, outbox, provider/payment adapters, auth и production остаются за отдельными gates.
