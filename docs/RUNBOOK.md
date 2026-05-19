# Runbook — operations and demo procedures

Этот документ — пошаговые процедуры для разработки, демо и разбора инцидентов.

## 1. Подключение к учебному кластеру КИ

Учебный кластер «Кибер Инфраструктура» (КИ) доступен **только через VPN**. Каждый член команды получает персональные креденшелы — они не передаются третьим лицам.

### Что выдают организаторы

| Артефакт | Назначение |
|---|---|
| Файл-профиль OpenVPN (`.ovpn`) | Конфигурация VPN-туннеля |
| Логин + пароль OpenVPN | Аутентификация при подключении VPN |
| Логин + пароль КИ | Аутентификация в веб-консоли и через API |

### Шаги подключения

1. **Установить OpenVPN Connect 3.8.0+** — https://openvpn.net/client/
2. Импортировать выданный `.ovpn` профиль (`File → Import Profile`)
3. Подключить VPN (логин/пароль из выданных)
4. Проверить доступность:
   ```bash
   curl -k -sS -o /dev/null -w "%{http_code}\n" \
        https://edu.cyber-infrastructure.ru:8800/login/Hackhaton
   # Ожидаем 200 или 302
   ```
5. Веб-консоль (для ручных проверок): `https://edu.cyber-infrastructure.ru:8800/login/Hackhaton`
   - Domain: `Hackhaton`
   - User: ваш выданный логин
6. После работы — отключить VPN.

### Параметры для бэкенда

В `.env`:

```bash
OPENSTACK_AUTH_URL=https://edu.cyber-infrastructure.ru:5000/v3   # уточнить точный endpoint при выдаче креденшелов
OPENSTACK_USERNAME=<ваш-логин-КИ>
OPENSTACK_PASSWORD=<ваш-пароль-КИ>
OPENSTACK_DOMAIN_NAME=Hackhaton
OPENSTACK_REGION=RegionOne
```

**Если Keystone API недоступен на :5000** — попробовать:
- `https://edu.cyber-infrastructure.ru:8800/identity/v3`
- `https://edu.cyber-infrastructure.ru/identity/v3`

Проверить можно curl-ом:
```bash
curl -k https://edu.cyber-infrastructure.ru:5000/v3 | jq .
# Должен вернуть JSON c "id":"v3.x", "status":"stable"
```

### Ограничения использования (от организаторов)

> - VPN-доступ предоставлен **исключительно** для подключения к образовательному стенду в рамках выполнения лабораторных работ хакатона.
> - **Запрещено** менять настройки кластера/стенда без разрешения преподавателя.
> - **Запрещено** запускать произвольные программы и скрипты в кластере без разрешения.
> - Все логины/пароли строго **персональны**, передача третьим лицам запрещена.
> - После окончания работы VPN-соединение **обязательно** отключать.

**Что это значит для нас (Cloud Lab Gateway):**
- Наш сервис работает **только внутри выделенных нашей команде проектов** в домене `Hackhaton`. Никаких операций на уровне всего кластера.
- Ansible-проверки выполняются **только** на ВМ, развёрнутых нашим сервисом в нашем проекте. Никаких внешних целей.
- Аудит-лог фиксирует каждое API-обращение к КИ — на случай разбора с организаторами.

### Без VPN — что доступно

Не вся работа требует VPN. Можно делать локально:

- Frontend (`npm run dev`) — полностью локально, мок-данные.
- Backend против `cloud/inmem` адаптера — `OPENSTACK_AUTH_URL=""` отключает реальный клиент, в DI поднимается `inmem.Provider`.
- Юнит-тесты, integration-тесты против testcontainers Postgres — без КИ.

VPN нужен только для:
- Юнит-смоук-теста реального OpenStack-адаптера (`go test -tags=integration ./internal/adapters/cloud/openstack/...`)
- Реального деплоя через docker-compose с реальной КИ.

## 2. Локальный запуск стека

```bash
# 1. Заполнить .env (см. .env.example)
cp .env.example .env
# Сгенерировать ключи:
openssl rand -base64 32                   # → CLG_KEK_BASE64
openssl rand -base64 64                   # → CLG_JWT_SECRET

# 2. Поднять окружение
make up                                   # postgres + redis + migrate + gateway + worker + frontend + nginx
make logs                                 # tail логов

# 3. Заполнить пул проектов
make seed-pool CSV=./projects.csv         # формат: ki_project_id,ki_domain_id,name

# 4. UI на http://localhost:8080
```

## 3. Demo-день — порядок действий

T-30 минут до выхода на сцену:
1. Подключить VPN, проверить `curl edu.cyber-infrastructure.ru:8800`.
2. `make down && make up && make logs` — чистый старт.
3. Прогнать `make check` (должен быть зелёным).
4. Открыть `localhost:8080`, проверить что фронт отвечает.
5. Открыть Moodle emulator (`docker compose --profile demo up moodle-emulator`).
6. Сделать тестовый launch → дойти до READY → запустить проверку → закрыть. Засечь время.
7. Записать резервное видео-демо (см. §4).
8. Подготовить tabs в браузере: фронт, audit log, БД (psql), Moodle emulator.

На сцене:
1. **Сценарий 1 — happy path**: clickin Moodle → progress bar deploy → READY → check PASSED.
2. **Сценарий 2 — quota guard**: имитировать высокую утилизацию (вручную через `psql`: `UPDATE quota_cache SET snapshot = jsonb_set(snapshot, '{vcpus,used}', '95');`) → попытка launch → красная карточка "Кластер на 95% загружен".
3. **Сценарий 3 — freeze**: студент жалуется → стенд замораживается → преподаватель видит в своём UI → разрешает.
4. **Архитектура**: открыть `docs/STATE_MACHINES.md` в браузере (mermaid-диаграммы).

## 4. Резервное видео-демо

Если на демо упадёт сеть или КИ — переключаемся на видео.

```bash
# Записать с экрана через `screen recording`:
# - 30 секунд: launch + deploy progress
# - 30 секунд: ready + check
# - 30 секунд: quota-guard карточка
# - 30 секунд: freeze + unfreeze
# Сохранить как docs/demo.mp4
```

## 5. Разбор инцидентов

### Лаба застряла в DEPLOYING

```bash
# Проверить состояние saga
docker compose exec postgres psql -U clg -d clg \
  -c "SELECT step_name, status, attempt, last_error FROM lab_deploy_steps WHERE lab_instance_id = '...';"

# Перезапустить шаг через asynq-CLI (если устанавливали)
docker compose exec worker /worker requeue --task-id="deploy:<lab-id>:1"

# Или ручное освобождение
docker compose exec postgres psql -U clg -d clg \
  -c "UPDATE lab_instances SET state = 'failed', state_reason = 'manual' WHERE id = '...';"
```

### Проект застрял в QUARANTINE

Проверить `last_error` в `audit_events`, при необходимости вручную почистить ресурсы в КИ через веб-консоль, затем:

```sql
UPDATE projects SET state = 'free', cleanup_failures = 0 WHERE id = '...';
```

### Quota всегда показывает stale

```bash
# Принудительно дёрнуть refresher
docker compose exec worker /worker run-task refresh_quota

# Или прямо в psql
UPDATE quota_cache SET fetched_at = now() WHERE id = 1;
```

## 6. Бэкап для защиты

Подготовить на USB-stick / в облаке:

- [ ] `docs/demo.mp4` — запись демо
- [ ] Презентация (PDF + PPTX)
- [ ] PostgreSQL dump с готовыми seed-данными для быстрого восстановления
- [ ] Скриншоты ключевых экранов (на случай отказа фронта)
- [ ] `docs/SECURITY.md`, `docs/STATE_MACHINES.md`, `docs/ARCHITECTURE.md` в распечатанном виде — для жюри, если попросят
