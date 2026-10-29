# SIM-010 — Bot UX v2, шаг 1

```yaml
id: SIM-010
title: TIMON SIM sandbox bot UX v2, step 1
product: sim
status: ready-for-owner
owner: human
repository: timon-platform
base_ref: main
worktree_required: false
risk: low
planner_model_class: balanced
worker_model_class: balanced
reviewer_model_class: balanced
allowed_paths:
  - internal/bot/**
  - docs/tasks/SIM-010-BOT-UX-V2.md
forbidden_paths:
  - deploy/**
  - internal/keepgo/**
  - internal/postgres/**
  - production/**
network_access: none
external_writes: false
human_approval_before_write: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Контекст

Закрытый Telegram-бот уже проходит sandbox-flow от страны до synthetic eSIM, но главное меню, тарифы, проверка совместимости и справочные экраны слишком технические и краткие.

## Цель

Сделать понятный пользовательский путь `меню -> страна -> тариф -> подробности -> совместимость -> тестовый заказ`, не меняя API, provider/payment adapters и deployment.

## Scope

- Расширить главное меню.
- Добавить «Как это работает», совместимость и FAQ.
- Показывать объём, срок, рекомендацию, итоговую цену и цену за ГБ.
- Требовать подтверждение поддержки eSIM перед созданием тестового заказа.
- Добавить обратную навигацию внутри sandbox-flow.
- Покрыть новые переходы unit-тестами.

## Non-goals

- Реальная provider line creation или payment gateway.
- Изменение API, БД, migrations, CI/CD или серверов.
- Публичный запуск, админка и второй шаг карточек заказов.
- Новые зависимости.

## Критерии готовности

- [x] Новое меню и справочные экраны доступны командами и кнопками.
- [x] Каталог и quote не показывают пользователю закупочную цену или технические provider details.
- [x] Создать заказ можно только после экрана совместимости.
- [x] Старые `/start`, `/buy`, `/my` и `/support` продолжают работать.
- [x] `go test ./...`, `go vet ./...` и `git diff --check` проходят.

## Отчёт Coder

- Изменены только Telegram engine, его unit-тесты и task contract.
- Добавлены меню, справочные экраны, рекомендации по пакетам, цена за ГБ, совместимость и обратная навигация.
- Прямой callback создания заказа заменён подтверждением совместимости.
- `go test ./...`, `go vet ./...` и `git diff --check` прошли локально.
- `go test -race ./...` не запускался: локальная Windows-среда собрана без CGO; CI остаётся источником race-проверки.
- Commit, push и deployment не выполнялись.

## Rollback

Откатить отдельный code commit обычным `git revert`; схема данных и внешнее состояние не меняются.
