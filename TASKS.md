# TASKS — план разработки и спеки для AI-агентов

Этот файл — план работ по дням и конкретные task specs, которые скармливаются Codex-агентам. Каждая задача:

- ссылается на конкретные файлы и контракты в репо
- содержит acceptance criteria
- содержит "do not touch" границы

## Координация двух потоков

| Поток | Кто | Контексты | Файлы |
|---|---|---|---|
| **A. Backend Core** | Ты + Codex №1 | Pool, Lab, Quota, Verify, Audit | `internal/domain/{pool,lab,quota,verify,audit}/`, `internal/adapters/{cloud,storage,queue,checker}/`, `internal/app/{deploy,cleanup}/` |
| **B. Integrations + UX** | Напарник + Codex №2 | Identity (Moodle/LTI), HTTP API, SSE, Frontend | `internal/adapters/{lms,httpapi,sse}/`, `internal/app/{moodle,student,teacher,admin}/`, `web/`, `cmd/moodle-emulator/` |

Граница потоков — **ports** (`internal/ports/`). Ни один поток не меняет порты в одиночку.

---

## Day 1: Foundation

### Day 1 — Stream A: Backend skeleton

**A1.1 — go.sum + main entrypoints**
- Run `go mod tidy` to materialise `go.sum`.
- Create `cmd/gateway/main.go` with cobra root + `serve`, `healthcheck`, `seed` subcommands. The `serve` command bootstraps config (viper), zap logger, pgx pool, redis client, asynq client, and starts an HTTP server on `CLG_BIND_ADDR`. No business logic yet — just `GET /healthz` returning 200.
- Create `cmd/worker/main.go` with cobra root + `run` subcommand. Bootstrap same dependencies; register asynq workers (empty handlers stubbed `// TODO: A2.x`).
- Create `cmd/migrate/main.go` thin wrapper that runs goose up/down against `PG_DSN`.

Acceptance:
- `go build ./...` succeeds.
- `docker compose up postgres redis migrate gateway worker` runs without crashes; `/healthz` returns 200.
- `make check` passes (the codebase is small).

**A1.2 — pgx connection pool + UoW**
- `internal/adapters/storage/db.go`: pgx pool factory from `PG_DSN`, with `MaxConns`, `MaxConnLifetime` from config.
- `internal/adapters/storage/uow.go`: implement `ports.UnitOfWork` using `pgxpool.Pool.BeginTx`. Implement marker `ports.Tx` via wrapper `pgxTx{tx pgx.Tx}`.

Acceptance:
- Unit test in `_test.go` that opens UoW, inserts a row, rolls back, asserts row absent.

### Day 1 — Stream B: Frontend skeleton + Moodle emulator skeleton

**B1.1 — Vite + React + Tailwind + shadcn/ui**
- In `web/`: `npm create vite@latest . -- --template react-ts` (manually init since dir exists).
- Install: `tailwindcss postcss autoprefixer`, init Tailwind. Install: `@tanstack/react-query @tanstack/react-router zustand zod react-hook-form`.
- Init shadcn/ui (`npx shadcn@latest init`). Add baseline components: `button`, `card`, `table`, `dialog`, `toast`, `input`, `form`.
- Three empty pages: `/student`, `/teacher`, `/admin`. Router skeleton with role-based redirect.

Acceptance:
- `npm run build` succeeds; `npm run dev` serves at :5173.
- `docker compose up frontend nginx-proxy` works; navigating to `localhost:8080/` shows the React app.

**B1.2 — Moodle emulator skeleton**
- `cmd/moodle-emulator/main.go`: standalone HTTP server (port 9000) with a single page listing 3 fake "courses with labs". Each lab has a "Launch" button that POSTs to the configured `EMULATOR_GATEWAY_URL/lti/launch` with a signed JWT.
- Implement LTI 1.3 launch JWT generation with a static RSA keypair (generated on startup, cached at `/tmp`). Expose `/jwks.json`.

Acceptance:
- Emulator serves a list of labs; clicking "Launch" reaches the gateway and gateway logs an unauthenticated launch attempt (B2.x implements verification).

---

## Day 2: Pool, OpenStack, Quota

### Day 2 — Stream A

**A2.1 — OpenStack adapter (CloudProvider)**
- `internal/adapters/cloud/openstack/`: implement `ports.CloudProvider` using `gophercloud/v2`.
- Methods: `GetQuotaSnapshot` (Nova `os-hypervisors/statistics` + Cinder `os-quota-sets`), `CreateKeypair`, `DeleteKeypair`, `BootServer`, `WaitForActive`, `AllocateFloatingIP`, `DeleteServer`, `CreateNetwork`, `DeleteNetwork`.
- Auth from env. Token caching with refresh.
- Implement parallel `inmem` adapter at `internal/adapters/cloud/inmem/` that simulates everything in-memory — used by tests and when КИ access is delayed.

Acceptance:
- Integration test against DevStack (or `inmem`) covering boot → wait → delete cycle.
- `GetQuotaSnapshot()` returns a `shared.QuotaSnapshot` with non-zero totals.

**A2.2 — Pool repo (Postgres)**
- `internal/adapters/storage/pgxrepo/pool.go`: implements `ports.PoolRepo`.
- `AllocateOneFree` MUST use `SELECT ... FOR UPDATE SKIP LOCKED` and update state atomically.
- `Save` must flush `p.PullEvents()` into the `outbox` table in the same tx.

Acceptance:
- Concurrency test: 50 goroutines try to allocate from a pool of 10 → exactly 10 succeed, no double-allocation.

**A2.3 — Quota guard use case + cache**
- `internal/adapters/storage/pgxrepo/quota_cache.go`: read/write `quota_cache` table (single row).
- `internal/app/quota/refresher.go`: asynq scheduled task `TaskRefreshQuota` every 30s that calls `cloud.GetQuotaSnapshot` and updates cache.
- `internal/app/lab/create.go`: use-case orchestrating create:
    1. Resolve thresholds from settings.
    2. Read quota cache. If stale (>2× TTL) → trigger refresh and return 503.
    3. Call `quota.Evaluate`. If denied → emit `KindQuotaBlocked` audit event, return `ErrQuotaExceeded`.
    4. Begin tx → `LabRepo.Create(pending_quota)` → `PoolRepo.AllocateOneFree` → `lab.AssignProject` → commit.
    5. Enqueue `TaskDeployLab`.

Acceptance:
- Unit test with mocked cloud + repo for quota=95%: returns `ErrQuotaExceeded`, AuditRepo has 1 event.
- Unit test for quota=50% + empty pool: returns `ErrPoolEmpty`, AuditRepo has 1 event, no `pending_quota` row left.

### Day 2 — Stream B

**B2.1 — Auth: LTI 1.3 verifier + session JWT issuer**
- `internal/adapters/lms/lti13/`: implements `ports.LMSProvider.VerifyLaunch`. Fetch JWKS from `LTI_JWKS_URL` (cached 1h, retry-on-miss). Validate iss, aud, exp, nonce uniqueness (Redis SETEX nonce with 5min TTL).
- `internal/adapters/httpapi/auth.go`: `/lti/launch` endpoint → verify → UpsertUser → issue session JWT → Set-Cookie → 302 redirect to `/student`.
- `internal/adapters/httpapi/middleware/auth.go`: middleware that reads `clg_session` cookie, decodes JWT, populates context with `Subject`.

Acceptance:
- Moodle emulator launch reaches gateway, returns Set-Cookie, redirects to `/student`. `/api/auth/me` returns the user.

**B2.2 — OpenAPI handlers (skeleton, no business logic)**
- Run `make gen-openapi` to generate `internal/adapters/httpapi/generated.go`.
- Implement strict-server interface: every handler returns `501 Not Implemented` initially; bind them in `serve.go` so the API surface is up.

Acceptance:
- Every path from `api/openapi.yaml` returns 501 (or 200 for healthz / auth/me). Verified via `curl` and a small spec-conformance test.

---

## Day 3: Async, Saga, State Machine

### Day 3 — Stream A

**A3.1 — Asynq worker + EventBus**
- `internal/adapters/queue/asynq/`: wrap `asynq.Client`/`asynq.Server`. Implement `ports.TaskQueue` and `ports.TaskRegistry`.
- Configure queues: `deploy` (priority 4), `cleanup` (priority 2), `checks` (priority 2), `default` (priority 1).
- Implement `ports.EventBus` as Redis pub/sub. Outbox publisher worker (separate goroutine) reads `outbox.published_at IS NULL`, publishes to bus, marks published.

Acceptance:
- Enqueue + handle round-trip with retry-on-error verified.
- Outbox publisher: insert in tx → row appears → publisher emits → row marked published.

**A3.2 — Deploy saga**
- `internal/app/deploy/saga.go`: implements the 5-step saga. Each step:
    - reads/writes `lab_deploy_steps` (idempotent: skip if SUCCEEDED).
    - on failure: increments attempt; if attempt >= 3 → set FAILED and trigger compensation; else asynq retry with backoff.
- Compensation runs in reverse: delete server → delete network → delete keypair → release project (transition cleaning→free if it gets that far).
- On success of all steps: `lab.MarkReady(cleanupAt)`, enqueue cleanup timer at `cleanup_at`.

Acceptance:
- Unit test (with inmem cloud): full happy path.
- Unit test: kill worker mid-step → new worker resumes from last SUCCEEDED step → completes.
- Unit test: cloud returns error 3 times → state goes to FAILED → compensation runs → project goes to FREE.

**A3.3 — Cleanup saga**
- `internal/app/cleanup/saga.go`: scheduled at `cleanup_at` via `TaskEnqueueAt`. 4-step saga (stop, delete-vm, delete-keypair, release-project).
- On 3 failures of any step → project → `quarantine`, lab.state = `done` with `cleanup_warning`.

Acceptance:
- Tests parallel to A3.2.

### Day 3 — Stream B

**B3.1 — SSE broker**
- `internal/adapters/sse/broker.go`: in-memory broker with per-audience channels. Authoritative source of events is the `EventBus`; SSE broker subscribes to all relevant topics.
- `internal/adapters/httpapi/sse.go`: implements `/sse/labs`. Reads `Subject` from context, subscribes to `user:{id}` and (if teacher) `course:{id}` for each course role.

Acceptance:
- e2e test: open SSE → trigger lab create in another goroutine → event received within 1s.

**B3.2 — Wire up student/teacher use cases**
- Replace 501s with real handlers for: `POST /api/v1/labs`, `GET /api/v1/labs`, `GET /api/v1/labs/:id`, `POST /api/v1/labs/:id/freeze`, `POST /api/v1/labs/:id/unfreeze`, `DELETE /api/v1/labs/:id`, `POST /api/v1/labs/:id/extend`, `GET /api/v1/labs/:id/ssh-key`.
- Apply RBAC via `identity.DefaultPolicy.Can`.

Acceptance:
- Full happy path via curl: launch → list → freeze → unfreeze → delete. Audit log has all transitions.

---

## Day 4: Frontend + Settings + Initial check

### Day 4 — Stream A

**A4.1 — Ansible runner**
- `internal/adapters/checker/ansible/`: implements `ports.CheckRunner`. Builds inventory file at `/tmp/ansible-<runID>/inv.ini` (0600), writes SSH key to `/tmp/ansible-<runID>/key` (0600). Runs `ansible-playbook -i <inv> <playbook> --ssh-extra-args='-o StrictHostKeyChecking=accept-new' --json` via `exec.CommandContext`.
- Parse Ansible JSON output → `CheckResult`. Cleanup tmp dir + zeroize key on defer.
- Implement 2 baseline playbooks in `ansible/checks/`: `linux-basics.yml` (check users, packages, file exists), `nginx-config.yml` (check service running, listening on :80, custom file exists).

Acceptance:
- Run against an in-network ubuntu container → result correctly parsed.
- Failing playbook → state=failed with per-task drill-down.

**A4.2 — Initial-check integration**
- After `WaitSSH` step in deploy saga, if `LabTemplate.DefaultCheckTemplateID != nil`: create CheckRun, enqueue, on terminal state record audit. Lab state remains READY regardless of check outcome (initial check is informational).

Acceptance:
- Deploy with default check populates `check_runs` row.

### Day 4 — Stream B

**B4.1 — Settings UI + endpoints**
- Frontend: page `/teacher/settings` showing scoped settings (global / per-course / per-template). Edit form for `default_cleanup_after_s`, `default_freeze_for_s`, `quota_threshold_pct`. Save → PUT `/api/v1/admin/settings`.
- Apply scope hierarchy: lab_template overrides course overrides global. Domain code: `settings.Resolve(...)` in repo.

Acceptance:
- Change `default_cleanup_after_s` for one course → new lab in that course has cleanup_at offset accordingly; lab in another course uses global.

**B4.2 — Student dashboard + Teacher overview**
- Student: card with current lab state, big "Сообщить о проблеме" button (freeze with reason form), "Завершить" button, SSH-key download, "Запустить проверку" with check dropdown, check history.
- Teacher: table of all labs for their courses, filters by state, actions (freeze/unfreeze/extend/delete), per-row check history.
- All updates via SSE.

Acceptance:
- Full demo flow works end-to-end against inmem cloud.

---

## Day 5: LTI 1.3 polish + Frontend polish + Admin pages

### Day 5 — Stream A

**A5.1 — Admin UI APIs**
- `GET /api/v1/admin/projects` + `POST /api/v1/admin/projects/:id/quarantine` + `POST /api/v1/admin/projects/:id/release`.
- `GET /api/v1/admin/quota` (returns current snapshot).
- `GET /api/v1/admin/audit` with filters.

Acceptance:
- Admin pages show real data.

**A5.2 — Seed CLI**
- `cmd/seed/main.go`: reads CSV `ki_project_id,ki_domain_id,name`. Inserts rows in `projects` table state=`free`. Idempotent (ON CONFLICT DO NOTHING).

Acceptance:
- Seed 30 projects across 3 domains; lab requests pull from correct domain based on course.

### Day 5 — Stream B

**B5.1 — LTI 1.3 NRPS + AGS (полноценно, бонус)**
- Names and Roles Service: GET membership list, sync `enrollments` table.
- AGS lineitems: on check.passed, post score back. Sign JWTs with our RSA key (KID exposed at `/lti/jwks`).

Acceptance:
- Score appears in Moodle emulator after successful check.

**B5.2 — Admin UI pages**
- `/admin/pool`, `/admin/quota`, `/admin/audit`. Mantine-like data tables with sorting, pagination.

Acceptance:
- Pages render real data, quarantine action works, audit search by kind/actor/since works.

---

## Day 6: E2E + Chaos + Hardening

**Coupled work, both streams.**

- E2E tests in `test/e2e/`: full flow scripted (Moodle emulator → gateway → inmem cloud → check → freeze → unfreeze → cleanup).
- Chaos: random Redis kill, Postgres restart, simulated cloud timeouts. Verify state machine recovers correctly. Document findings in `docs/RUNBOOK.md`.
- Security pass: full `make check`, fix all findings. Manual review against [SECURITY.md §10 demo checklist](docs/SECURITY.md#10-demo-day-готовность).
- Performance smoke: deploy 20 labs concurrently → no double-allocation, all states converge.

Acceptance:
- E2E test passes from clean state in <2 min.
- `make check` zero issues at HIGH/CRITICAL.

---

## Day 7: Demo prep

- `docs/RUNBOOK.md`: step-by-step "give me a working demo in 5 min" guide.
- Презентация: 15-20 слайдов, обоснование архитектурных решений (см. ARCHITECTURE.md §9 "Alternatives considered"). Mermaid диаграммы — копируем в слайды.
- Резервное видео-демо: запись полного сценария на случай сетевого сбоя на защите.
- Seed-данные для демо: 1 курс с 3 лабами, 5 студентов, готовый запущенный стенд для скриншотов.
- Репетиция вопросов жюри (см. docs/SECURITY.md §10, ARCHITECTURE.md §9, STATE_MACHINES.md полностью).

---

## Правила для AI-агентов

1. **Не меняй `internal/ports/` без согласования с другим потоком.** Если порту нужно расширение — открывай отдельный PR с одним коммитом и описанием.
2. **Не пиши логику в handler'ах.** Handler — это thin layer: parse → call use case → render response. Все use cases — в `internal/app/`.
3. **State machine — only via `.Transition()` / `.AllocateTo()` / etc.** Линтер `forbidigo` ловит прямые присвоения.
4. **Каждый PR должен зеленить `make check`.**
5. **Не коммить без `gitleaks detect` локально.** `.env` в `.gitignore` — но кто-то может закоммитить отдельный секрет.
6. **Tests parallel-safe:** `t.Parallel()`, unique resource names, no shared global state.
7. **Errors:** wrap with `%w`, use sentinel errors from `internal/domain/shared/errors.go`.
8. **Logs:** zap structured, never `fmt.Print*`. Redact secrets in payload.
9. **DB queries:** sqlc-generated; raw `pgx.Exec` только для миграций.
10. **Idempotency:** любая enqueued task имеет `IdempotencyKey` (typically `{type}:{labID}:{attempt}`).
