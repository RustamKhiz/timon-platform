# SIM-011 — Bot order UX and persistent navigation

```yaml
id: SIM-011
title: TIMON SIM order cards, installation guides and persistent Telegram navigation
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
  - docs/tasks/SIM-011-BOT-ORDER-UX.md
forbidden_paths:
  - deploy/**
  - internal/keepgo/**
  - internal/postgres/**
network_access: none
external_writes: false
human_approval_before_write: false
human_approval_before_push: true
human_approval_before_deploy: true
```

## Цель

Сделать sandbox-бот удобным без обязательного ввода `/start`, улучшить «Мои eSIM» и разделить выдачу QR, инструкции и ручные данные без изменения API и схемы БД.

## Scope

- Постоянная Telegram reply keyboard с главным меню и основными действиями.
- Обработка текстовых кнопок как существующих команд.
- Карточки заказов с коротким display id и пользовательскими статусами.
- Отдельные экраны QR, iPhone, Android и ручной установки.
- Помощь с безопасной привязкой к display id заказа.
- Безопасные пользовательские ошибки без URL и внутренних сообщений.
- Country context в package callback для продолжения выбора после перезапуска bot process.

## Non-goals

- Provider purchase, реальные activation data и payment gateway.
- Автоматическая отправка support ticket во внешний сервис.
- Изменение API, PostgreSQL schema, CI/CD или deployment.
- Фоновое уведомление о готовности eSIM; оно остаётся отдельной задачей после durable notification design.

## Критерии готовности

- [x] После открытия главного меню Telegram показывает постоянные кнопки управления.
- [x] Возврат в меню не требует `/start`.
- [x] «Мои eSIM» не показывает внутренние статусы и полный order id.
- [x] QR и инструкции разделены по назначению.
- [x] Ошибка API не раскрывает URL или внутренний текст пользователю.
- [x] Новый package callback работает после создания нового `Engine`.
- [x] `go test ./...`, `go vet ./...` и `git diff --check` проходят.

## Rollback

Откатить отдельный commit обычным `git revert`; внешние данные и схема БД не меняются.

## Отчёт Coder

- Изменены только `internal/bot` и этот task contract.
- Добавлены unit-тесты persistent reply keyboard, новых экранов, session-independent package callback и безопасной ошибки.
- Новые зависимости не добавлены.
- Commit, push и deployment ещё не выполнялись.
