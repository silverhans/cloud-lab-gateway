# Security Model

Документ описывает модель угроз, контрмеры и DevSecOps-стандарты Cloud Lab Gateway. Закрывает критерий оценки 8 (DevSecOps).

## 1. Threat model (STRIDE-lite)

| Угроза | Сценарий | Контрмера |
|---|---|---|
| **Spoofing** | Студент выдаёт себя за другого через подделанный Moodle callback | LTI 1.3 c JWT-подписью (JWKS) + nonce + state, для REST-эмулятора — HMAC-подпись |
| **Tampering** | Изменение payload LTI launch для получения чужой лабы | Подпись JWT проверяется на каждом launch; mapping `lti_sub → user` хранится в БД, не доверяем launch payload |
| **Repudiation** | "Я не запрашивал заморозку" | Transactional outbox: каждое действие → `audit_event` в той же транзакции |
| **Information disclosure** | Утечка приватных SSH-ключей из БД | Envelope encryption (AES-256-GCM, KEK→DEK), redaction в логах |
| **DoS** | Студент запускает 100 лаб одновременно | Per-user rate limit (1 active lab), quota guard на cluster level (>90%) |
| **Elevation of privilege** | Студент дёргает admin API | RBAC policy на каждом use-case, не на handler-уровне (defence in depth) |
| **Supply chain** | Компрометированная зависимость | `trivy`, `govulncheck`, `gitleaks` в CI; pinned versions в go.mod / package-lock |

## 2. Secret management

### 2.1 Hierarchy

```
┌────────────────────────────────────────────────────────────┐
│  KEK (Key Encryption Key)                                  │
│  AES-256, читается из CLG_KEK_BASE64 env var               │
│  Никогда не хранится в БД                                  │
│  Ротируется вручную, см. SECURITY.md §2.4                  │
└────────────────────────────────────────────────────────────┘
                       │ encrypts
                       ▼
┌────────────────────────────────────────────────────────────┐
│  DEK (Data Encryption Key) — один на каждую запись         │
│  Случайный AES-256 ключ, генерируется per-record           │
│  Хранится в БД в зашифрованном виде (AES-256-GCM с KEK)    │
└────────────────────────────────────────────────────────────┘
                       │ encrypts
                       ▼
┌────────────────────────────────────────────────────────────┐
│  Sensitive payload (SSH private key, LTI shared secret,    │
│   Moodle webservice token, OpenStack admin password)       │
│  AES-256-GCM с DEK                                         │
└────────────────────────────────────────────────────────────┘
```

### 2.2 Структура зашифрованной записи

```sql
CREATE TABLE encrypted_secrets (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL,        -- 'ssh_private_key' | 'lti_shared_secret' | ...
    ref_id UUID NOT NULL,      -- ссылка на доменную сущность (lab_instance, course, ...)
    dek_ciphertext BYTEA NOT NULL,    -- DEK зашифрован KEK
    dek_nonce BYTEA NOT NULL,
    payload_ciphertext BYTEA NOT NULL,
    payload_nonce BYTEA NOT NULL,
    aad TEXT NOT NULL,          -- additional auth data: "{kind}:{ref_id}"
    kek_version INT NOT NULL,   -- для ротации
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

AAD (Additional Authenticated Data) включает `kind:ref_id` — защита от cross-record swap-атак (нельзя подсунуть DEK от одного ключа другому).

### 2.3 Алгоритм encrypt/decrypt

```
Encrypt(payload, kind, ref_id):
  1. DEK = random(32)
  2. payload_ct, payload_nonce = AES-256-GCM(DEK, payload, aad="{kind}:{ref_id}")
  3. dek_ct, dek_nonce = AES-256-GCM(KEK, DEK, aad="{kind}:{ref_id}:{kek_version}")
  4. store {dek_ct, dek_nonce, payload_ct, payload_nonce, aad, kek_version}
  5. zeroize DEK in memory

Decrypt(record):
  1. DEK = AES-256-GCM-decrypt(KEK[record.kek_version], record.dek_ct, record.dek_nonce, aad=record.aad+":"+kek_version)
  2. payload = AES-256-GCM-decrypt(DEK, record.payload_ct, record.payload_nonce, aad=record.aad)
  3. zeroize DEK
  4. return payload (caller obligated to zeroize after use)
```

### 2.4 Ротация KEK

1. Сгенерировать новый KEK_v2, добавить в env как `CLG_KEK_V2_BASE64`.
2. Установить `CLG_KEK_CURRENT_VERSION=2` (новые DEK шифруются v2).
3. Запустить admin-команду `gateway rotate-deks --to-version=2`. Для каждой записи:
   - Decrypt DEK с KEK_v1
   - Encrypt DEK с KEK_v2
   - Update `kek_version=2`
4. После завершения — удалить `CLG_KEK_V1_BASE64`.

Payload (SSH-ключи) **не** перешифровывается — это десятки KB, а DEK всего 32 байта.

### 2.5 KeyProvider интерфейс — готовность к Vault

```go
type KeyProvider interface {
    // EncryptDEK шифрует DEK мастер-ключом текущей версии.
    EncryptDEK(ctx context.Context, dek []byte, aad string) (ciphertext []byte, nonce []byte, version int, err error)
    // DecryptDEK расшифровывает DEK ключом указанной версии.
    DecryptDEK(ctx context.Context, ct, nonce []byte, version int, aad string) (dek []byte, err error)
    CurrentVersion(ctx context.Context) (int, error)
}
```

Имплементации:
- `envKeyProvider` — KEK из env (MVP)
- `vaultTransitKeyProvider` — HashiCorp Vault Transit (готов к проду, не пишем для хакатона)
- `awsKMSKeyProvider` — AWS KMS (опционально)

## 3. Authentication & Authorization

### 3.1 Auth flows

```
┌────────────────────┐                  ┌─────────────────────┐
│   Student via      │                  │   Gateway           │
│   Moodle (LTI 1.3) │ ──launch JWT──>  │   /lti/launch       │
└────────────────────┘                  │   verify JWKS       │
                                        │   create session    │
                                        │   issue our JWT     │
                                        └──────┬──────────────┘
                                               │
                                               ▼
                                        Set-Cookie: clg_session
                                        (HttpOnly, Secure, SameSite=Lax)

┌────────────────────┐                  ┌─────────────────────┐
│ Teacher/Admin via  │ ──POST creds──>  │  /api/auth/login    │
│ direct login       │ <──our JWT────── │  bcrypt verify       │
└────────────────────┘                  │  issue JWT          │
                                        └─────────────────────┘
```

### 3.2 Tokens

| Токен | Алгоритм | TTL | Где хранится | Назначение |
|---|---|---|---|---|
| LTI launch JWT | RS256 (от Moodle) | 5 min | приходит в `id_token` | Аутентификация студента из LMS |
| Session JWT (наш) | HS256, ключ `CLG_JWT_SECRET` | 8h, refresh-able | HttpOnly cookie | Сессия в браузере |
| Moodle WS token | static | rotated externally | env var, encrypted in DB | Доступ к Moodle REST API |
| Service-to-service | mTLS (опционально) | — | env | gateway ↔ worker (один процесс — не нужен) |

### 3.3 RBAC

Роли: `student`, `teacher`, `admin`.

Policy реализована в `internal/app/policy/`, проверяется на уровне use-case (не handler — это deeper defence):

```go
type Policy interface {
    Can(ctx context.Context, action Action, resource Resource) error
}

// Examples:
policy.Can(ctx, ActionLabCreate, course)            // student? enrolled?
policy.Can(ctx, ActionLabFreeze, labInstance)       // teacher of course OR owner
policy.Can(ctx, ActionAdminSettings, settings)      // admin only
policy.Can(ctx, ActionPoolQuarantine, project)      // admin only
```

Каждая отрицательная проверка → `audit_event` (`access_denied`).

## 4. Network security

### 4.1 Boundaries

```
Internet
   │
   ▼
[nginx-proxy :80/:443]
   │
   ├── /          → frontend (static)
   ├── /api/*     → gateway :8080
   ├── /sse/*     → gateway :8080  (long-lived, no buffering)
   └── /lti/*     → gateway :8080
        │
        ▼
[gateway] ──┬── postgres :5432 (internal network only)
            ├── redis :6379    (internal network only)
            └── КИ API         (TLS, токен в env, encrypted in DB)
[worker] ──┬── те же
           └── SSH к ВМ студента (через КИ-managed floating IP)
```

### 4.2 Контроль

- `docker-compose`: всё кроме `nginx-proxy` — на internal-сети, не экспонируется наружу.
- Postgres / Redis — без публичных портов.
- TLS terminate на nginx (self-signed для хакатон-демо, Let's Encrypt — для прода).
- HSTS, secure cookies, CSP заголовки.
- Rate limit на `/api/auth/login` и `/lti/launch` (10/min/IP).

### 4.3 SSH к ВМ студента

- Gateway/worker имеют **только приватный ключ для проверок** — он generated per-lab, никогда не реюзается.
- Студент скачивает **свою копию** ключа (separate keypair) через UI; gateway хранит шифрованным.
- Соединение: `ssh -i {key} -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 ubuntu@{floating_ip}`.
- `StrictHostKeyChecking=accept-new` — TOFU, host fingerprint сохраняется при первой проверке; mismatch на повторе → fail.

## 5. Input validation

### 5.1 Boundaries

| Boundary | Валидатор | Что проверяется |
|---|---|---|
| HTTP request body | `validator.v10` через struct tags + custom `Validate()` | Типы, длины, regex, бизнес-инварианты |
| URL path params | `chi` + `uuid.Parse` | UUID format |
| Query params | manual + validator | enum values, ranges |
| LTI JWT claims | `jwt.Parse` + manual checks | iss, aud, exp, nonce uniqueness |
| Moodle webhook | HMAC signature | Подписано нашим shared secret |
| Ansible playbook output | JSON schema | Только expected fields |

### 5.2 Никогда не доверяем

- `lti_launch.email` / `lti_launch.name` — отображаем, но **не используем как primary key**, только `lti_sub`.
- Параметры из URL — все через validator.
- Output Ansible — парсим только known JSON keys, ignore unknown.

## 6. Audit log

См. [STATE_MACHINES.md](STATE_MACHINES.md). Кратко:

- Every state transition → `audit_event` (transactional, в той же tx что изменение).
- Every access denial → `audit_event` (kind: `access_denied`).
- Every secret access (decrypt) → `audit_event` (kind: `secret_accessed`, без payload).
- Every quota block → `audit_event` (kind: `quota_blocked`).
- Every external API call to КИ → log entry с `request_id`, `duration_ms`, `http_status`.

Audit log — append-only, нет UPDATE/DELETE. Retention — 90 дней (хакатон demo: forever).

## 7. Container hardening

### Dockerfile.gateway / Dockerfile.worker

```dockerfile
# Multi-stage build
FROM golang:1.22-alpine AS build
...
# Final stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build --chown=nonroot:nonroot /app/gateway /gateway
USER nonroot:nonroot
ENTRYPOINT ["/gateway"]
```

Принципы:

- **Distroless** базовый образ — no shell, no package manager.
- **Non-root user** (`nonroot:65532`) — UID/GID 65532.
- **Read-only rootfs**: `read_only: true` в compose, writable tmpfs только для `/tmp`.
- **Drop all capabilities**: `cap_drop: [ALL]`.
- **No new privileges**: `security_opt: [no-new-privileges:true]`.
- **Health checks** — кубер-style, чтобы swarm/compose рестартил при отказе.
- **Resource limits**: `mem_limit`, `cpus` в compose, чтобы воркер не съел хост.

### Dockerfile.frontend

```dockerfile
FROM node:20-alpine AS build
...
FROM nginx:1.27-alpine-slim
COPY --from=build /app/dist /usr/share/nginx/html
# nginx runs as non-root via custom config (nginx-unprivileged image)
USER 101:101
```

## 8. DevSecOps pipeline

Запуск всех проверок: `make check`. Pre-commit запускает быстрые, CI — полные.

| Инструмент | Что ловит | Когда |
|---|---|---|
| `gofmt -s` | Стиль | pre-commit |
| `goimports` | Импорты | pre-commit |
| `golangci-lint` | 30+ линтеров, конфиг в `.golangci.yml` | pre-commit |
| `gosec` | OWASP Top 10 для Go: hardcoded creds, SQL injection, weak crypto | CI |
| `govulncheck` | Известные CVE в зависимостях | CI |
| `gitleaks` | Секреты в коде/истории | pre-commit + CI |
| `hadolint` | Dockerfile best practices | CI |
| `trivy fs` | Уязвимости в образах и зависимостях | CI |
| `dockerfile-non-root-check` (custom) | USER directive present | pre-commit |
| `eslint + typescript-eslint` | Frontend | pre-commit |
| `npm audit` / `pnpm audit` | Frontend deps | CI |

Failing любого ⇒ red. Защита: показать `make check` в зелёном на демо.

## 9. Что НЕ делаем (и почему — для защиты)

| Не делаем | Почему |
|---|---|
| Vault standalone | +1 точка отказа на демо. KeyProvider готов к подмене. |
| mTLS gateway↔worker | Один процесс, общий бинарь — не нужен. |
| Sentry/PagerDuty | Out of scope; audit log + logs покрывают. |
| Полный SAST (SonarQube) | golangci-lint + gosec + govulncheck покрывают. |
| OPA/Rego | RBAC у нас compile-time типизированный, OPA — overkill. |

## 10. Demo-day готовность

Перед защитой:

- [ ] `make check` зелёный
- [ ] `gitleaks detect --no-git` — 0 findings
- [ ] `trivy fs --severity HIGH,CRITICAL .` — 0 findings
- [ ] Скрин из docker inspect → User: nonroot
- [ ] Скрин из БД → encrypted_secrets есть, payload — нечитаемый BYTEA
- [ ] Сценарий: попытка SQL injection в форме → отвергается validator-ом, audit_event
- [ ] Сценарий: попытка студента вызвать teacher API → 403 + audit_event "access_denied"
