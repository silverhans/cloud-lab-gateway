# CLAUDE.md

Контекст для AI-агентов (Claude Code, Codex) и членов команды, работающих над этим репо.

## Что это

Безопасный шлюз оркестрации облачных лабораторных стендов для платформы «Кибер Инфраструктура» (КИ, OpenStack-совместимая) с интеграцией Moodle LMS. Хакатон-проект, бэкенд Go, фронт React.

Поток: студент кликает в LMS → шлюз выделяет проект из заранее подготовленного пула → разворачивает стенд → запускает Ansible-проверки → удаляет по таймеру.

## Архитектура — TL;DR

- **Hexagonal (Ports & Adapters)** — домен не зависит от инфры
- **Modular monolith**: один codebase, два бинаря (`gateway` HTTP + `worker` async)
- **State machine везде** — изменение состояния только через методы агрегата
- **Saga + transactional outbox** для длинных операций
- **Pessimistic locking** (`SELECT ... FOR UPDATE SKIP LOCKED`) для аренды проекта
- **Envelope encryption** (KEK→DEK, AES-256-GCM) для SSH-ключей и секретов
- **SSE** для realtime обновлений (не WebSocket — поток событий однонаправленный)
- **OpenAPI-first**: `api/openapi.yaml` — источник истины для handlers + фронта

Подробности в [docs/](docs/):
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — слои, контексты, обоснования выбора
- [DOMAIN_MODEL.md](docs/DOMAIN_MODEL.md) — агрегаты, ER, инварианты
- [STATE_MACHINES.md](docs/STATE_MACHINES.md) — Mermaid-диаграммы переходов
- [SECURITY.md](docs/SECURITY.md) — threat model, envelope encryption, RBAC, demo checklist
- [TASKS.md](TASKS.md) — план разработки по дням и спеки задач для AI-агентов

## Структура репо

```
cmd/                    — точки входа
  gateway/              — HTTP API + SSE + LTI endpoint
  worker/               — async-воркер (asynq)
  migrate/              — runner миграций
  seed/                 — наполнение пула проектов из CSV
  moodle-emulator/      — мини-эмулятор Moodle для демо

internal/
  domain/               — pure Go, никаких infra-импортов
    shared/             — ID типы, sentinel-ошибки, value objects (ResourceRequest, QuotaSnapshot)
    lab/                — LabInstance aggregate + state machine
    pool/               — Project (КИ tenant) aggregate
    quota/              — Quota guard (pure function Evaluate)
    verify/             — CheckRun + CheckRunState
    identity/           — User, Course, Enrollment, RBAC Policy
    audit/              — AuditEvent + event kinds

  ports/                — интерфейсы (граница между потоками A и B!)
    cloud.go            — CloudProvider (OpenStack/КИ)
    lms.go              — LMSProvider (Moodle/LTI)
    checker.go          — CheckRunner (Ansible)
    storage.go          — Pool/Lab/User/Audit/... репозитории + UnitOfWork
    queue.go            — TaskQueue, EventBus
    secrets.go          — KeyProvider, SecretStore
    system.go           — Clock, Random, SSEBroker

  app/                  — use cases и sagas (Codex пишет основной объём здесь)
    deploy/             — Deploy saga (5 шагов + компенсация)
    cleanup/            — Cleanup saga
    moodle/             — Use cases от LMS (handle LTI launch)
    student/teacher/admin/ — use cases по ролям

  adapters/             — реализации портов
    cloud/openstack/    — gophercloud-клиент
    cloud/inmem/        — тестовая реализация (для разработки без КИ)
    lms/lti13/          — LTI 1.3 (JWT + JWKS + AGS + NRPS)
    lms/moodlerest/     — Moodle Web Services REST
    checker/ansible/    — Ansible runner через exec.Command
    storage/pgxrepo/    — pgx + sqlc репозитории
    queue/asynq/        — Redis + asynq
    secrets/envkek/     — envelope encryption (KEK из env)
    httpapi/            — chi handlers + middleware (генерится из openapi.yaml)
    sse/                — SSE broker
  pkg/                  — logger (zap), errors, clock

api/openapi.yaml        — REST контракт (генерим chi-server + TS-клиент)
migrations/             — goose SQL миграции
ansible/checks/         — Ansible playbooks для проверок
web/                    — React + Vite + TS + Tailwind + shadcn/ui
deployments/            — docker-compose, Dockerfile'ы, nginx.conf
```

## Жёсткие правила

1. **Не меняй `internal/ports/` без согласования.** Ports — публичный контракт между потоками A (Backend Core) и B (Integrations + UX). Изменение порта — отдельный PR с описанием.
2. **State machines — только через методы.** `lab.Transition(...)`, `project.AllocateTo(...)`, `checkRun.Start(...)`. Прямое присвоение `lab.State = ...` блокируется линтером `forbidigo` в `.golangci.yml`.
3. **Handler — thin layer.** Парсинг → use case → render. Никакой бизнес-логики в `internal/adapters/httpapi/`. Use cases живут в `internal/app/`.
4. **Errors.** Wrap через `%w`. Сентинелы — в `internal/domain/shared/errors.go`. Маппинг error → HTTP status централизован.
5. **Логи.** Только `zap` (structured JSON). Никаких `fmt.Print*` (линтер ловит). Секреты в payload — redact.
6. **SQL.** Через sqlc-сгенерированные функции. Сырые `pgx.Exec` — только в миграциях.
7. **Идемпотентность.** Каждый `TaskQueue.Enqueue` имеет `IdempotencyKey` (обычно `"{type}:{labID}:{attempt}"`).
8. **Транзакционный outbox.** Domain events (`agg.PullEvents()`) пишутся в `outbox` в той же tx, что и `repo.Save(agg)`.
9. **Никаких секретов в коде.** `gitleaks` в pre-commit. `.env` в `.gitignore`. Чувствительное в БД — только через `SecretStore` (envelope encryption).
10. **Тесты parallel-safe.** `t.Parallel()`, никаких глобальных переменных, уникальные имена ресурсов на тест.

## Рабочий процесс

```bash
# Первый запуск
make tools                       # gosec, golangci-lint, goose, sqlc, oapi-codegen
cp .env.example .env
# заполнить .env: CLG_KEK_BASE64, CLG_JWT_SECRET, OPENSTACK_*
# openssl rand -base64 32 → CLG_KEK_BASE64
# openssl rand -base64 64 → CLG_JWT_SECRET

# Разработка
make up                          # docker-compose: postgres + redis + gateway + worker + frontend + nginx
make logs                        # tail логи
make migrate-up                  # вручную (compose также применяет автоматически через service migrate)
make gen                         # sqlc + openapi → код в internal/adapters/storage/sqlcgen + httpapi/generated.go

# Перед PR
make check                       # gofmt + lint + vet + gosec + govulncheck + gitleaks — все должны быть зелёные

# Тесты
make test                        # unit
make test-integration            # требует поднятого compose
```

## Жизненный цикл фичи

1. Изменения REST → сначала `api/openapi.yaml`, потом `make gen-openapi`, потом handler.
2. Новая доменная сущность → добавь в `internal/domain/<context>/`, state machine, события (`PullEvents`), unit-тест.
3. Новый порт нужен → согласуй с другим потоком, добавь в `internal/ports/`, реализуй adapter, добавь в DI.
4. Изменение схемы БД → новая миграция `migrations/000X_<descr>.sql` (никогда не редактируй существующие), обнови sqlc-queries, перегенерируй.
5. Новая asynq-задача → константа в `ports/queue.go`, handler в `internal/app/...`, регистрация в `cmd/worker/main.go`.

## Деление работы (1-2 человека + 2-3 AI-агента)

| Поток | Контексты | Файлы |
|---|---|---|
| **A. Backend Core** | Pool, Lab, Quota, Verify, Audit | `internal/domain/{pool,lab,quota,verify,audit}/`, `internal/adapters/{cloud,storage,queue,checker,secrets}/`, `internal/app/{deploy,cleanup}/`, `cmd/{gateway,worker,migrate,seed}/` |
| **B. Integrations + UX** | Identity (LMS/LTI), HTTP API, SSE, Frontend | `internal/adapters/{lms,httpapi,sse}/`, `internal/app/{moodle,student,teacher,admin}/`, `cmd/moodle-emulator/`, весь `web/` |

Граница потоков = `internal/ports/`. Подробные спеки задач — в [TASKS.md](TASKS.md).

## Что НЕ делаем (и почему — для защиты)

- **Микросервисы** — modular monolith даёт чистые границы без операционной сложности.
- **Vault standalone** — KeyProvider-интерфейс готов к Vault, но на демо лишняя точка отказа.
- **WebSocket** — поток событий односторонний; SSE проще, нет проблем с прокси.
- **GraphQL** — REST + OpenAPI достаточно, тесты проще.
- **ORM** — sqlc даёт типобезопасный SQL без магии.
- **Динамическое создание доменов в КИ** — запрещено условиями кейса (ИБ-риск).
- **Полное event sourcing** — outbox + audit log достаточно.

## Контекст для защиты

Жюри оценивает по 10 критериям × 10 баллов = 100. Самые "толстые" с точки зрения архитектуры:

- **Критерий 3** (async + state machine) → Saga + outbox + Mermaid-диаграммы (STATE_MACHINES.md)
- **Критерий 7** (качество Go) → hexagonal + sqlc + zero panics + идиоматичные интерфейсы
- **Критерий 8** (DevSecOps) → make check зелёный + envelope encryption + distroless containers
- **Критерий 10** (защита) → ARCHITECTURE.md §9 "Alternatives considered" + диаграммы

Перед защитой пройдись по [docs/SECURITY.md §10 demo checklist](docs/SECURITY.md#10-demo-day-готовность).
