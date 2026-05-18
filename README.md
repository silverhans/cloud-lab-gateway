# Cloud Lab Gateway

Безопасный шлюз оркестрации облачных лабораторных стендов для платформы «Кибер Инфраструктура» (КИ) с интеграцией Moodle LMS.

```
                   ┌──────────────────────┐
       Moodle ──── │  LTI 1.3 launch     │ ──┐
                   └──────────────────────┘  │
                                             ▼
        Student ─── HTTP ──── Cloud Lab Gateway (Go)
                                             │
                                             ├── Pool & Capacity (quota guard, project allocator)
                                             ├── Lab Lifecycle (state machine, sagas, timers)
                                             ├── Verification (Ansible runner)
                                             └── Identity & Access (RBAC, LTI)
                                             │
                                             ▼
                   КИ / OpenStack (gophercloud)  →  изолированные tenant-проекты с ВМ
```

## Документация

Чтение в этом порядке даёт полную картину:

1. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — слои, контексты, технические решения
2. [docs/DOMAIN_MODEL.md](docs/DOMAIN_MODEL.md) — ER, агрегаты, инварианты
3. [docs/STATE_MACHINES.md](docs/STATE_MACHINES.md) — Mermaid-диаграммы всех state machines
4. [docs/SECURITY.md](docs/SECURITY.md) — threat model, envelope encryption, RBAC
5. [api/openapi.yaml](api/openapi.yaml) — REST-контракт (источник истины)
6. [TASKS.md](TASKS.md) — план разработки по дням и задачи для AI-агентов

## Быстрый старт

```bash
# 1. Установить инструменты разработки
make tools

# 2. Скопировать env, заполнить секреты
cp .env.example .env
# отредактировать .env: CLG_KEK_BASE64, CLG_JWT_SECRET, OPENSTACK_*

# 3. Поднять стек (postgres, redis, gateway, worker, frontend, nginx)
make up

# 4. Применить миграции (запускается контейнером migrate автоматически)
# вручную: make migrate-up

# 5. Заполнить пул проектов из CSV (см. docs/RUNBOOK.md)
make seed-pool PATH=./projects.csv

# 6. UI доступен на http://localhost:8080
```

Для разработки (без docker):

```bash
# postgres + redis в docker, остальное локально
docker compose -f deployments/docker-compose.yml up -d postgres redis
make migrate-up
go run ./cmd/gateway serve
go run ./cmd/worker run            # в отдельном терминале
cd web && npm install && npm run dev
```

## Стек

| Layer | Tech |
|---|---|
| Backend | Go 1.22, chi, gophercloud, asynq, pgx + sqlc, goose, zap |
| Frontend | React 18, Vite, TypeScript, Tailwind, shadcn/ui, TanStack Query |
| Realtime | Server-Sent Events |
| Crypto | AES-256-GCM envelope encryption (KEK→DEK) |
| Checks | Ansible (JSON callback) |
| DB | Postgres 16 |
| Queue | Redis 7 + asynq |
| Infra | docker-compose, nginx, distroless containers |

## Безопасность

- `make check` — единая команда для запуска всех линтеров и анализаторов (`gofmt`, `golangci-lint`, `gosec`, `govulncheck`, `gitleaks`, `hadolint`, `trivy`).
- pre-commit запускается на каждый коммит — см. `.pre-commit-config.yaml`.
- Все секреты — в env, никогда в коде. Чувствительные данные в БД зашифрованы envelope-схемой.
- Контейнеры — distroless / unprivileged nginx, non-root, read-only rootfs, dropped capabilities.

Подробно: [docs/SECURITY.md](docs/SECURITY.md).

## Структура репозитория

```
cmd/                  — точки входа (gateway, worker, migrate, seed, moodle-emulator)
internal/
  domain/             — pure Go бизнес-логика, без infra
  ports/              — интерфейсы (Cloud, LMS, Storage, Queue, Secrets, ...)
  app/                — use cases, sagas
  adapters/           — реализации портов (OpenStack, Moodle, Ansible, pgx, asynq)
  pkg/                — logger, errors, clock
api/                  — OpenAPI 3.1 spec + oapi-codegen config
migrations/           — goose SQL миграции
ansible/              — playbooks для проверок
web/                  — React фронтенд
deployments/          — docker-compose, Dockerfile'ы, nginx.conf
docs/                 — архитектура, security, домен, runbook
```

## Лицензия

Внутренний проект хакатона. См. условия организаторов.
