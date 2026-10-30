# SIM-013 — Admin order query

```yaml
id: SIM-013
title: Filterable read-only SIM admin order query
product: sim
status: ready-for-owner
owner: human
repository: timon-platform
base_ref: main
risk: low
allowed_paths:
  - internal/httpapi/**
  - internal/postgres/**
  - internal/sim/**
  - docs/tasks/SIM-013-ADMIN-ORDER-QUERY.md
external_writes: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Цель

Добавить безопасные read-only фильтры для списка заказов SIM Admin без изменения схемы БД и без влияния на VPN.

## Scope

- Поиск по полному/короткому order id и Telegram user id.
- Фильтр по order status и country code.
- Не более 100 последних совпадений.
- Сохранение существующего ответа при пустых фильтрах.

## Non-goals

- Изменение статуса заказа.
- Resend, refund, provider/payment операции и audit trail.
- Миграции БД, push и deployment.

## Критерии готовности

- [x] Фильтры одинаково работают в memory и PostgreSQL repositories.
- [x] Неизвестные/некорректные фильтры отклоняются без SQL-конкатенации пользовательских значений.
- [x] Admin endpoints остаются закрыты owner session.
- [x] `go test ./...` проходит.
- [x] `git diff --check` проходит.

## Проверено локально

- `go test ./...`
- `git -c safe.directory=C:/main/projects/timon/timon-platform diff --check`

## Rollback

Вернуть прежнюю сигнатуру `AdminOrders` и удалить query parsing; схема БД не меняется.
