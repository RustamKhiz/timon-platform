# SIM-012 — Read-only administration API

```yaml
id: SIM-012
title: TIMON SIM read-only administration API and owner session
product: sim
status: ready-for-owner
owner: human
repository: timon-platform
base_ref: main
risk: medium
allowed_paths:
  - cmd/sim-api/**
  - internal/adminauth/**
  - internal/httpapi/**
  - internal/postgres/**
  - internal/sim/**
  - .env.example
  - .github/workflows/pipeline.yml
  - docs/tasks/SIM-012-ADMIN-API.md
external_writes: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Цель

Дать отдельной SIM-админке безопасный read-only доступ к заказам без VPN authentication и без изменения схемы БД.

## Scope

- Owner login по username и bcrypt password hash.
- Подписанная HMAC-сессия в `HttpOnly; SameSite=Strict` cookie.
- `GET /api/v1/admin/session`.
- `GET /api/v1/admin/orders` и `GET /api/v1/admin/orders/{id}`.
- Secure cookie по умолчанию; явное отключение только для локальной HTTP-разработки.

## Non-goals

- Изменение заказов, refund и resend.
- Реальные payment/provider операции.
- Production secrets и deployment.

## Конфигурация

- `SIM_ADMIN_USERNAME`.
- `SIM_ADMIN_PASSWORD_BCRYPT`.
- `SIM_ADMIN_SESSION_SECRET` длиной не менее 32 символов.
- `SIM_ADMIN_COOKIE_SECURE=false` только для локального HTTP smoke.

Если admin variables не заданы, SIM API продолжает работать, но admin endpoints отвечают `503`.

## Критерии готовности

- [x] Без сессии admin endpoints отвечают `401`.
- [x] Валидный login создаёт HttpOnly cookie и открывает read-only endpoints.
- [x] Истёкшая или изменённая session отклоняется.
- [x] `go test ./...` проходит.
- [x] Выполнен локальный browser smoke совместно с `sim-admin`.
- [ ] Production configuration и deployment согласованы отдельно.

## Rollback

Удалить admin routes/auth package и методы admin read из repository. Схема БД не менялась.
