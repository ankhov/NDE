# Network Diagnostics Engine — Анализ, архитектура и план Phase 1

**Документ:** Анализ SPEC.md + план Foundation
**Версия:** 1.0
**Дата:** 2026-11-19
**Связанный SPEC:** `upload/SPEC.md` v1.0
**Статус:** Ready for review → Phase 1 implementation

---

## 1. Анализ технического задания

SPEC описывает настольное (Windows 10/11 x64) приложение для постоянного сетевого мониторинга и объяснимой диагностики: Modular Monolith на Go 1.26+, Wails v3, React+TS фронтенд, SQLite. Ключевой продуктовый тезис — **вероятностные гипотезы с подтверждающими свидетельствами**, а не категоричные выводы. Это задаёт жёсткие требования к разделению слоёв: коллекторы не содержат диагностики, правила диагностики не зависят от UI и сборщиков (SPEC §3.3, §4.8, правило 9/14.2).

Стек указан явно и обоснован: Go + `golang.org/x/net`, `github.com/miekg/dns`, `github.com/shirou/gopsutil/v4`, `modernc.org/sqlite` (pure-Go, без CGO — важно для кросс-компиляции под Windows). Согласован с Modular Monolith: один бинарник, внутренние пакеты, интерфейсы между модулями.

Дорожная карта (SPEC §12) разбита на 8 фаз строго последовательно. Phase 1 = Foundation: Go/Wails/React/TS init, структура, SQLite, миграции, Application Core. **Бизнес-логика (коллекторы, планировщик, метрики, аналитика, диагностика) в Phase 1 не реализуется** — только контракты (интерфейсы) и инфраструктура.

---

## 2. Противоречия и пробелы в SPEC

### 2.1. Реальные противоречия / напряжения

| № | Место SPEC | Противоречие | Решение |
|---|---|---|---|
| C1 | §4.1 ICMP Echo + §9.3 «минимальные привилегии» | ICMP через raw-сокеты на Windows требует прав администратора. `golang.org/x/net/icmp` по умолчанию использует raw ICMP. | На Windows использовать unprivileged API `IcmpSendEcho` (через `golang.org/x/sys/windows` syscall), на Linux — UDP-трюк ICMP (требует `cap_net_raw` или запуск с правами, но не root-обязательно). TCP-connect RTT — всегда доступный fallback. Решение относится к Phase 2, но фиксируется архитектурно сейчас (build-tags, интерфейс `Collector`). |
| C2 | §9.3 «запретить произвольное сканирование сетей» + §4.6 Traceroute + §3.2 Network Discovery | Трение между безопасностью и функциональностью. | Discovery читает системную конфигурацию (интерфейсы/маршруты/DNS) без активного сканирования. Traceroute — только к явно настроенным целям и к шлюзу по умолчанию. Сканирование подсетей запрещено. |
| C3 | §4.2/§4.3 «при недостатке данных — `insufficient_data`» vs числовые поля в схеме `metric_aggregates` | Схема хранит числа, но статус «недостаточно данных» требует маркера. | В `metric_aggregates` значение NULL = «нет данных», плюс поле `status` (`ok`/`insufficient_data`/`error`). В Go-модели — `*float64` (как в §7), nil = нет значения. |
| C4 | §3.4 «повторные проверки после ошибок» + «защита от накопления очередей» | Ретраи могут сами накапливать очередь. | Политика: не более N ретраев (по умолчанию 2, настраивается), skip-if-overdue — если предыдущий тик ещё выполняется или просрочен > 1 интервала, новый не ставится в очередь, а пропускается с записью `skipped`. |
| C5 | §4.8 «каждая гипотеза: …Recommendations» vs схема `diagnostic_events` | В схеме нет поля/таблицы для рекомендаций. | Добавить таблицу `diagnostic_recommendations(event_id, text, priority)` (см. §3 ниже — gap-fix). |
| C6 | §4.7 «аномалия содержит связанные измерения» vs схема `anomalies` | В схеме `anomalies` нет связи с измерениями. | Добавить таблицу-связку `anomaly_measurements(anomaly_id, measurement_id)`. |
| C7 | §4.10 Notifications (события/уровни) vs схема | Нет таблицы истории уведомлений. | Добавить таблицу `notifications(id, level, event_type, message, target_id, created_at, dismissed_at)`; пользовательские настройки — в `settings`. |

### 2.2. Недостающие требования (заполняем как архитектурные решения)

- **Формат/место конфига:** bootstrap-конфиг через флаги CLI + defaults → runtime-настройки в таблице `settings`. Без внешнего YAML-файла (минимум зависимостей).
- **Логирование:** `log/slog` (stdlib), structured JSON в файл + консоль, уровни, ротация по размеру. Путь — рядом с БД.
- **Расположение БД:** dev — `./db/nde.db`; prod (Windows) — `%APPDATA%\nde\nde.db`. Путь разрешается через `os.UserConfigDir()`.
- **Миграции:** embedded SQL (go:embed), lightweight-мигратор с таблицей `schema_migrations(version, applied_at)`, без внешних зависимостей (golang-migrate не нужен).
- **Хранение времени:** все `timestamp`/`started_at`/`ended_at` — `INTEGER` (Unix-миллисекунды, UTC). Латентности — `INTEGER` мс (или REAL для дробных). Соответствует SPEC §6 «UTC» и §4.1 «миллисекунды».
- **metadata:** колонка `metadata` в `measurements` — `TEXT` (JSON). (Де)сериализация в `map[string]any` в storage-слое.
- **Confidence:** `INTEGER 0..100` (эвристика, не вероятность, SPEC §4.8). Банды: 0–29 low, 30–69 medium, 70–100 high — документируются.
- **Severity:** `TEXT` enum `INFO|WARNING|CRITICAL` (совпадает с §4.10).
- **Retention:** raw `measurements` — N дней (по умолчанию 7), `metric_aggregates` — дольше (365). Настраивается в settings. Запускается как фоновая задача core.
- **SSRF-защита для пользовательских HTTP/TCP-целей:** блокировать loopback/private/link-local/multicast/reserved диапазоны (RFC 1918/4291) при добавлении цели и перед каждым запросом; опция `allow_loopback` для dev-целей (явно).
- **Anomaly merge window:** одинаковые `anomaly_type + target_id` в пределах окна (по умолчанию 5 мин простоя) продлевают существующую аномалию, а не создают новую (SPEC §4.7 «не создавать повторное уведомление»).
- **Concurrency model:** `golang.org/x/sync/semaphore` для bounded parallel collectors; `errgroup` для оркестрации; `context.Context` везде (SPEC правило 10).

### 2.3. Технические риски (top)

| Риск | Вероятность | Влияние | Митигация |
|---|---|---|---|
| R1. Windows unprivileged ICMP | Высокая | Высокое | C1: IcmpSendEcho + TCP fallback; спроектировано в Phase 1 (интерфейс), реализовано в Phase 2. |
| R2. Wails v3 API churn / стабильность | Средняя | Среднее | Пин точной версии wails; изоляция Wails в `cmd/app` + тонком `internal/app/bindings`; домен не зависит от Wails → тестируется headless. |
| R3. modernc.org/sqlite throughput для high-frequency writes | Средняя | Среднее | WAL + `busy_timeout` + пакетная запись (транзакции) + prepared stmts + агрегация + retention. Бенчмарк в Phase 3. |
| R4. Кросс-компиляция Linux→Windows | Низкая | Среднее | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64` (modernc, gopsutil — pure Go). Wails CLI: `wails3 build -platform windows/amd64`. Здесь GUI-run невозможен (нет WebKitGTK), но build + unit-тесты проходят. |
| R5. Рост `measurements` (time-series scale) | Высокая | Среднее | Индексы `(target_id, timestamp)`; retention; агрегация; партиционирование не нужно для MVP. |
| R6. Фоновые goroutine-утечки | Средняя | Среднее | context-cancellation, errgroup, ordered shutdown в Application Core, тест lifecycle. |
| R7. Flaky real-network тесты | Высокая | Низкое | Коллекторы за интерфейсами; unit-тесты на fake-коллекторах; integration — localhost; scenario — чистые данные. |

---

## 3. Финальная архитектура

### 3.1. Слои (top → bottom)

```
Desktop UI (React/TS) ⇄ Wails bindings (тонкий адаптер) ⇄ Application Services ⇄ Domain modules
                                                              ⇄ Storage (interfaces)
```

- **Application Core** (`internal/core`): lifecycle, config bootstrap, module registry, ordered graceful shutdown, retention jobs.
- **Domain modules** (`internal/network/*`, `internal/scheduler`, `internal/metrics/*`, `internal/analytics/*`, `internal/diagnostics/*`, `internal/reporting`): каждый — пакет с экспортируемыми интерфейсами, без перекрёстных импортов кроме как через core-wiring и шину событий.
- **Storage Layer** (`internal/storage`): интерфейсы репозиториев (`storage.Repository` и подпрограммы) + sqlite-реализация (`internal/storage/sqlite`) + миграции (`internal/storage/migrations`). Домен зависит от интерфейсов, не от sqlite.
- **Cross-cutting**: `internal/events` (типизированная шина), `internal/logging` (slog), `internal/config`, `internal/errors`, `internal/measurement` (унифицированная модель), `internal/target`.

### 3.2. Коммуникация

- **Синхронно:** core → collectors/metrics/storage через прямые вызовы интерфейсов.
- **Асинхронно (decoupled):** analytics → diagnostics → UI через типизированную шину событий (`internal/events`) с именами из SPEC §8 (`measurement:new`, `anomaly:detected`, …). Шина изолирует домен от Wails: Wails-слой подписывается на шину и транслирует в Wails-события; в тестах — in-memory подписчики.

### 3.3. Жёсткие границы (из SPEC §3.3, §4.8, правило 9)

- `internal/collector`/`internal/network/*` — **только сбор**, без диагностики и аналитики.
- `internal/diagnostics` — зависит только от `metrics`-результатов + `analytics`-событий + интерфейса `MeasurementReader`; **никогда** от конкретных коллекторов или UI.
- Wails импортируется **только** в `cmd/app` и `internal/app/bindings`. Весь домен тестируется headless.

### 3.4. Диаграмма зависимостей

```
cmd/app ──► internal/app/bindings (Wails DTOs)
        ──► internal/core ──► internal/events, internal/config, internal/logging, internal/storage(ifs)
                          ──► internal/scheduler ──► internal/collector(ifs) ──► internal/network/* (Phase 2)
                          ──► internal/metrics, internal/analytics, internal/diagnostics (Phase 3-5)
                          ──► internal/reporting (Phase 7)
internal/network/*  ──► internal/measurement, internal/target, internal/errors
internal/diagnostics ──► internal/measurement, internal/storage(ifs), internal/events  (NO collector imports)
```

---

## 4. Структура директорий

Соответствует SPEC §13 + обоснованные дополнения (`internal/app`, `config`, `errors`, `events`, `logging`, `measurement`, `target`) для cross-cutting и унифицированной модели:

```
network-diagnostics-engine/   (repo root)
  cmd/app/                     # main entry, Wails app, DI wiring
  internal/
    app/bindings/              # тонкие Wails-bound методы (DTO only)
    core/                      # Application Core: lifecycle, module registry, shutdown
    config/                    # загрузка конфига, defaults, валидация
    errors/                    # типизированные ошибки
    events/                    # шина событий, типизированные события (имена SPEC §8)
    logging/                   # slog setup
    measurement/               # унифицированная Measurement + Target модели, enums
    target/                    # модель цели, валидация, группировка, SSRF-guard
    network/{discovery,icmp,tcp,dns,http,traceroute}/
    collector/                 # интерфейс Collector + registry
    scheduler/
    metrics/{calculator,aggregation}/
    analytics/{baseline,anomaly,correlation}/
    diagnostics/{engine,rules,evidence,confidence}/
    storage/                   # интерфейсы репозиториев
      sqlite/                  # реализация
      migrations/              # embedded SQL
    reporting/                 # CSV/JSON export
  frontend/src/{components,features/{dashboard,diagnostics,targets,history,network,settings},hooks,stores,types}/
  tests/{integration,scenarios,fixtures}/
  docs/{architecture,algorithms,decisions}/
  .github/workflows/
  .vscode/
  scripts/                     # dev-хелперы (env.sh, build, …)
  go.mod, go.sum, README.md, .gitignore
```

Дополнения к SPEC §13 обоснованы: cross-cutting пакеты (`config/errors/events/logging`) вынесены из `core`, чтобы core содержал только lifecycle/registry; `measurement` и `target` — общие модели для нескольких модулей (избегаем циклических импортов). Реальных противоречий со структурой SPEC нет.

---

## 5. Основные интерфейсы Go (контракты Phase 1)

В Phase 1 интерфейсы **объявляются** (компилируются), но **не реализуются** бизнес-логикой (правило 14: без фиктивных реализаций). Реализации — в последующих фазах. Исключения: `core.Module`, `events.Bus`, `storage.Migrator`, `config.Loader` — реализуются в Phase 1 (инфраструктура).

```go
// internal/measurement/measurement.go
type Type string // "icmp" | "tcp" | "dns" | "http" | "traceroute"

type Measurement struct {
    TargetID  string
    Timestamp time.Time
    Type      Type
    Latency   *float64 // ms, nil если нет значения (insufficient/timeout)
    Success   bool
    ErrorCode string
    Metadata  map[string]any
}

type Target struct {
    ID               string
    Name             string
    Host             string
    Protocol         string
    Port             int
    CheckInterval    time.Duration
    Timeout          time.Duration
    Enabled          bool
    WarningThreshold float64
    CriticalThreshold float64
    Group            string // local | external | dns | custom
}
```

```go
// internal/collector/collector.go
type Collector interface {
    Name() string
    Supports(t measurement.Type) bool
    Collect(ctx context.Context, target measurement.Target) (measurement.Measurement, error)
}
type Registry interface { Register(c Collector); Get(t measurement.Type) (Collector, bool) }
```

```go
// internal/storage/repository.go (интерфейсы; impl — Phase 3)
type MeasurementWriter interface {
    WriteMeasurements(ctx context.Context, m []measurement.Measurement) error
}
type MeasurementReader interface {
    QueryMeasurements(ctx context.Context, f QueryFilter) ([]measurement.Measurement, error)
}
type TargetStore interface {
    UpsertTarget(ctx context.Context, t measurement.Target) error
    ListTargets(ctx context.Context) ([]measurement.Target, error)
}
type Repository interface {
    MeasurementWriter
    MeasurementReader
    TargetStore
    // + anomalies, diagnostics, aggregates (Phase 4-5)
}
type Migrator interface { Migrate(ctx context.Context) error } // реализован в Phase 1
```

```go
// internal/events/events.go
type EventName string // SPEC §8: "measurement:new", "anomaly:detected", ...
type Event struct { Name EventName; Payload any; At time.Time }
type Bus interface {
    Subscribe(name EventName, h func(Event)) (unsubscribe func())
    Publish(ctx context.Context, e Event)
}
```

```go
// internal/core/core.go
type Module interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
type App struct { /* modules, bus, repo, cfg, log */ }
func (a *App) Start(ctx context.Context) error // ordered start
func (a *App) Shutdown(ctx context.Context) error // ordered stop, bounded by ctx
```

Все сетевые операции принимают `context.Context` (SPEC §7). UI/главный поток не блокируется (SPEC §9.1) — фоновые goroutine.

---

## 6. Окружение: команды, VSCode, requirements

### 6.1. Команды создания окружения (Linux dev host, Windows target)

```bash
# Go 1.27.4 (>=1.26 по SPEC) — в workspace, не системно
mkdir -p .sdk && curl -fsSL -o /tmp/go.tgz https://go.dev/dl/go1.27.4.linux-amd64.tar.gz \
  && tar -xzf /tmp/go.tgz -C .sdk && rm /tmp/go.tgz
source scripts/env.sh   # экспортирует GOROOT/GOPATH/GOCACHE/PATH

# Зависимости (Phase 1 добирает по мере необходимости; полный список ниже)
go get github.com/wailsapp/wails/v3@latest              # Wails runtime (pin в go.mod)
go get modernc.org/sqlite@latest                        # pure-Go SQLite
go get github.com/shirou/gopsutil/v4@latest             # системная инфа (Phase 2)
go get golang.org/x/net@latest                          # ICMP и пр. (Phase 2)
go get github.com/miekg/dns@latest                      # DNS collector (Phase 2)
go get golang.org/x/sync@latest                         # semaphore/errgroup
go get github.com/stretchr/testify@latest               # тесты (SPEC §2.4)

# Wails CLI v3
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 version

# Frontend
cd frontend && npm install   # React, TS, Vite, Tailwind, shadcn/ui, Recharts, Zustand, TanStack Query
```

**Сборка под Windows (кросс-компиляция с Linux):**
```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 wails3 build -platform windows/amd64
```
На Linux-dev без WebKitGTK `wails3 dev` (GUI) не запускается — это нормально; backend и unit-тесты работают полностью, GUI-интеграция проверяется на Windows-машине или в CI.

### 6.2. Расширения VSCode (`.vscode/extensions.json`)

```json
{
  "recommendations": [
    "golang.Go",
    "bradlc.vscode-tailwindcss",
    "dbaeumer.vscode-eslint",
    "esbenp.prettier-vscode",
    "ms-playwright.playwright",
    "redhat.vscode-yaml"
  ]
}
```

`.vscode/settings.json`: `gopls` formatter + `go.lintTool=golangci-lint` (если есть), Tailwind, ESLint flat config, format-on-save. `.vscode/launch.json`: `go test` + `wails3 dev` конфиги.

### 6.3. requirements / go.mod

Зависимости фиксированы в `go.mod` (Go-эквивалент requirements). Точные версии будут запинены при первом `go get` в Phase 1 шагах. Минимальный набор Phase 1: `modernc.org/sqlite`, `github.com/stretchr/testify` (для теста миграций/lifecycle), `golang.org/x/sync`. Wails runtime добавляется на шаге 1.2. gopsutil/x/net/miekg/dns — на Phase 2.

Frontend `package.json` (зависимости SPEC §2.2): react, react-dom, typescript, vite, tailwindcss, @radix-ui/* (основа shadcn/ui), recharts, zustand, @tanstack/react-query; dev: vitest, @testing-library/react, @playwright/test, eslint, prettier.

---

## 7. Подробный план Phase 1 — Foundation

Цель Phase 1: **загружаемый каркас backend + frontend + применённая схема БД, без бизнес-логики, всё компилируется, тесты зелёные.** Разбивается на маленькие проверяемые шаги; после каждого — `go build ./...` + `go test ./...` (где применимо).

### Шаг 1.1 — Go-модуль и scaffolding репозитория  ✅ (старт в этом PR)
- Расширить `.gitignore` (`.sdk/`, `build/`, `dist/`, `*.db`, `frontend/node_modules`, `frontend/dist`, sandbox-артефакты).
- `go mod init github.com/ankhov/NDE` (реальный путь репозитория https://github.com/ankhov/NDE).
- Создать дерево директорий SPEC §13 (+ обоснованные дополнения).
- `scripts/env.sh` (bootstrap Go в workspace).
- Минимальный `cmd/app/main.go` (version, exit) — placeholder до шага 1.5.
- `README.md` (статус, сборка, тесты, карта фаз).
- **Verify:** `go build ./...` OK, `go vet ./...` чисто.

### Шаг 1.2 — Wails v3 интеграция
- Установить wails CLI v3 в workspace GOBIN.
- Добавить wails runtime в `go.mod` (pin версию).
- `cmd/app/main.go` → Wails-приложение (min shell): регистрация пустого биндинга, lifecycle.
- `wails.json`/конфиг wails v3.
- **Verify:** `wails3 version` OK; `go build ./cmd/app` OK; wails runtime подключён.

### Шаг 1.3 — Frontend-каркас
- Vite + React + TS + Tailwind + shadcn/ui base + Zustand + TanStack Query + Recharts в `frontend/`.
- 6 роут-шеллов (dashboard/diagnostics/targets/history/network/settings) — пустые placeholder-страницы с заголовком.
- Тёмная тема (SPEC §5).
- **Verify:** `npm run build` и `tsc --noEmit` OK; `dist/` создан.

### Шаг 1.4 — SQLite + миграции
- `go get modernc.org/sqlite`.
- `internal/storage/sqlite`: open DB, PRAGMA `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`.
- `internal/storage/migrations/0001_init.sql`: все таблицы SPEC §6 + индексы `(target_id, timestamp)` + gap-fix'ы (C5 `diagnostic_recommendations`, C6 `anomaly_measurements`, C7 `notifications`, `status` в `metric_aggregates`).
- `internal/storage/migrations` (go:embed) + lightweight-мигратор с `schema_migrations`.
- `internal/storage.Repository`/`Migrator` интерфейсы объявлены; `Migrator` реализован.
- **Verify:** unit-тест — открыть temp-БД, применить миграции, assert таблицы/индексы/WAL.

### Шаг 1.5 — Application Core
- `internal/core`: `App`, `Module` interface, module registry, ordered Start/Shutdown, context-based lifecycle, обработка сигналов (в main).
- `internal/config`: defaults + loader + валидация.
- `internal/errors`: типизированные ошибки (`ErrConfig`, `ErrStorage`, …).
- `internal/events`: типизированная шина + имена SPEC §8 + тест pub/sub.
- `internal/logging`: slog setup (JSON, уровни).
- `internal/measurement`, `internal/target`, `internal/collector`, `internal/storage` (interfaces) — объявлены.
- `cmd/app/main.go`: собрать App (config→logging→events→storage→migrate→core), Start, ждать сигнал/ctx, Shutdown.
- **Verify:** integration-тест — старт App headless, миграции применены, регистрация no-op Module, Shutdown в пределах timeout.

### Шаг 1.6 — VSCode config, CI skeleton, docs
- `.vscode/{extensions,settings,launch}.json`.
- `.github/workflows/ci.yml`: `go build/test/vet` (CGO_ENABLED=0), `npm ci && npm run build` в frontend, кеши.
- `docs/architecture/overview.md`, `docs/algorithms/` (заглушки-темплейты для Phase 3-5), `docs/decisions/adr-001…adr-00N` (pure-Go sqlite, Windows ICMP, event bus, module path, retention, SSRF), `README.md` актуальный.
- **Verify:** YAML линтер; `go test ./...` зелёный; README корректен.

---

## 8. Критерии готовности Phase 1 (Definition of Done)

Phase 1 считается завершённой, когда **все** следующие пункты истинны (каждый проверен):

1. `go build ./...` — успех, `go vet ./...` — чисто.
2. `go test ./...` — зелёный (минимум: тест миграций БД, тест lifecycle Application Core, тест event bus, тест config validation).
3. Frontend: `npm run build` и `tsc --noEmit` успех, `frontend/dist/` создан.
4. `wails3 version` работает; wails runtime подключён в backend; `go build ./cmd/app` успех.
5. SQLite: свежая БД после миграций содержит **все** таблицы SPEC §6 + индексы `(target_id, timestamp)` + gap-fix таблицы (`diagnostic_recommendations`, `anomaly_measurements`, `notifications`); WAL подтверждён `PRAGMA journal_mode`; `foreign_keys=ON`; covered unit-тестом.
6. Application Core: стартует headless, применяет миграции, регистрирует `Module`, корректно останавливается по cancel context за ≤ 3с — covered тестом.
7. Event bus: pub/sub типизированных событий с именами SPEC §8 — covered тестом.
8. Структура директорий соответствует SPEC §13 (с задокументированными обоснованными дополнениями).
9. Основные контракты (Collector, Measurement, Target, Repository, Module, Bus) объявлены и компилируются; **нет** фиктивных/stub-реализаций бизнес-логики (правило SPEC 14).
10. `.vscode`, `.github/workflows/ci.yml`, `README.md`, `docs/architecture`, `docs/decisions` существуют и корректны.
11. Бизнес-логика последующих фаз (коллекторы, scheduler, metrics, analytics, diagnostics) **отсутствует** — Phase 1 = только foundation.
12. Логирование структурированное (slog); ошибки типизированы; `context.Context` во всех сигнатурах фоновых операций.
13. README описывает: как собрать backend, как собрать frontend, как запустить тесты, текущий статус фаз.

После выполнения всех 13 пунктов Phase 1 закрыта, и можно переходить к Phase 2 (Networking) по тому же принципу малых проверяемых шагов.

---

## 9. Соответствие SPEC (control check)

- Стек не изменён (SPEC §2): Go 1.27.4 ≥ 1.26; Wails v3; modernc.org/sqlite; gopsutil/v4; miekg/dns; golang.org/x/net; React+TS+Vite+Tailwind+shadcn/ui+Recharts+Zustand+TanStack Query; testify/vitest/RTL/Playwright; GitHub Actions. ✅
- Modular Monolith (SPEC §3): один бинарник, внутренние пакеты, интерфейсы, без микросервисов. ✅
- Интерфейс `Collector` и модель `Measurement` (SPEC §7) — сохранены 1:1 (с `*float64`, `context.Context`). ✅
- Имена событий (SPEC §8) — сохранены. ✅
- Схема БД (SPEC §6) — сохранена + задокументированные gap-fix'ы (C5/C6/C7). ✅
- Правила для AI (SPEC §14): по фазам, малыми шагами, план до реализации, без stub'ов, context.Context, явные ошибки, тесты. ✅
- Definition of Done (SPEC §15): отражён в критериях Phase 1 (§8). ✅
