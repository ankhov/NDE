# Network Diagnostics Engine (NDE)

NDE is a desktop application for continuous network monitoring and
explainable diagnostics. It runs scheduled checks against the hosts and
services you care about, stores the results in a local SQLite database, and
presents live latency, jitter and packet-loss trends in a dashboard — with the
IP path to each target available on demand.

The product is built as a single self-contained binary: a Go core does all the
monitoring and storage, and a React + TypeScript UI runs inside an embedded
desktop window. There is no server to deploy and no external database to
operate.

---

## Features

- **Five measurement types.** ICMP echo, TCP connect, DNS exchange, HTTP GET
  and traceroute — all collected by the same scheduler and stored in one
  unified model.
- **Live dashboard.** Average / P95 / jitter RTT and packet-loss cards plus an
  RTT timeline that updates in real time as measurements arrive.
- **Targets management.** Add, edit, enable/disable and delete monitoring
  targets; changes take effect immediately without restarting the app.
- **Start / stop monitoring.** A single toggle pauses and resumes collection
  without tearing down the runners behind it.
- **Collector status.** A dedicated page reports which measurement types have a
  live collector registered, so you always know what can run.
- **Path discovery.** Traceroute measurements record every hop and the RTT to
  the destination, so you can see where a path degrades.
- **Local-first storage.** Every measurement is written to a local SQLite
  database (WAL mode, time-series indexes, batched writes) — your data never
  leaves the machine.
- **SSRF protection.** User-supplied targets are screened against
  private / loopback / link-local / reserved IP ranges both at configuration
  time and at request time, so a hostname that resolves to an internal address
  is never probed.

---

## How it works

```
React UI  ⇄  Wails bindings (thin adapters)  ⇄  Application services
                                                         ⇄  Domain modules
                                                         ⇄  Storage (interfaces)
```

1. **Collectors** perform a single network check and return a unified
   `Measurement`. A collector knows nothing about diagnostics or the UI — it
   only measures.
2. The **scheduler** runs one goroutine per enabled target at its configured
   interval. Concurrency is bounded by a semaphore so many targets cannot
   starve the machine. Pause/resume gates collection without restarting runners.
3. Each measurement is **persisted** to SQLite in a batched transaction and
   published on a **typed event bus**.
4. The **Wails layer** subscribes to the event bus and re-emits events to the
   frontend, so the dashboard and targets views refresh the instant new data
   lands — no polling required for live updates.

The domain never imports the UI framework, so the entire monitoring pipeline is
testable headless.

### Collectors

| Type | What it measures | How |
|------|------------------|-----|
| ICMP | Round-trip latency to a host | OS `ping` (IcmpSendEcho on Windows; cap_net_raw / UDP-ICMP on Linux and macOS) |
| TCP | Time to complete the TCP handshake to (host, port) | `net.Dialer` |
| DNS | Round-trip of a DNS query to a resolver, with the response code | `miekg/dns` |
| HTTP | Round-trip of an HTTP GET, with the status code | `net/http` (no redirect following) |
| Traceroute | The IP path to a host, hop by hop | OS `traceroute` / `tracert` |

Every collector resolves the target through the SSRF guard and contacts the
screened IP directly, closing the window that would exist if the transport
re-resolved the hostname itself. Target-side failures (timeout, refused,
unreachable, SSRF block, …) are encoded as stable error codes on the
measurement rather than raised as Go errors, so a down target is a normal
result, not a collector fault.

---

## Tech stack

- **Backend:** Go 1.27.4, Wails v3.1.0, the standard library,
  `golang.org/x/net`, `github.com/miekg/dns`, `modernc.org/sqlite`
  (pure-Go, no CGO).
- **Frontend:** React 18, TypeScript, Vite 6, Tailwind CSS v4, shadcn/ui,
  Recharts, Zustand, TanStack Query.
- **Database:** SQLite (WAL mode, time-series indexes, embedded migrations,
  batched writes).
- **Testing:** Go testing + testify; Vitest.

---

## Repository layout

```
cmd/app/                    entry point: dispatch, headless and GUI runners
internal/
  core/                     Application Core: lifecycle, module registry, shutdown
  config/ errors/ events/   cross-cutting: config, typed errors, event bus
  logging/                  structured logging (slog)
  measurement/ target/      unified measurement & target models, error codes
  collector/                Collector interface, registry, TCP/DNS/HTTP/ICMP/traceroute
  scheduler/                interval scheduler with bounded parallelism
  storage/                  repository interfaces
    sqlite/                 SQLite implementation + measurement/target repositories
    migrations/             embedded SQL migrations
  app/bindings/             thin Wails-bound services (App/Target/Measurement/Monitoring/Collector)
  app/seed/                 default target seeding on first run
frontend/src/
  components/ layout, ui primitives
  features/    dashboard, collectors, targets, diagnostics, history, network, settings
  hooks/       TanStack Query hooks over the Wails client
  lib/         typed Wails runtime client
  stores/      Zustand app store
  types/       DTO shapes mirrored from the Go bindings
docs/{architecture,decisions}/   architecture overview and ADRs
.github/workflows/  .vscode/  scripts/
```

---

## Build from source

The build target is Windows 10/11 x64, but the Go toolchain and tests run on
any Linux/macOS host. The Windows build needs no C toolchain — every
dependency (including SQLite and the WebView bindings) is pure-Go.

### Prerequisites

- Go 1.27.4 or newer
- Node.js 18+ and npm

### Backend

```bash
go build ./...
go vet ./...
go test ./...
```

### Frontend

```bash
cd frontend
npm install
npm run build      # type-checks (tsc) and bundles with Vite
npm test           # Vitest
```

### Two build modes

The binary has two entry points, selected by a build tag:

```bash
# Headless — runs the monitoring core until SIGINT/SIGTERM.
go build -o bin/nde ./cmd/app

# GUI — the desktop application with bound services and embedded frontend
# assets. Build the frontend first so frontend/dist exists for the embed.
(cd frontend && npm run build)
go build -tags=gui -o bin/NDE ./cmd/app
```

### Cross-compile a Windows binary

```bash
# Headless Windows binary
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/nde.exe ./cmd/app

# GUI Windows binary (~26 MB)
(cd frontend && npm run build)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags=gui -o bin/NDE.exe ./cmd/app
```

> The live GUI (`wails3 dev`) needs a native WebView backend (WebView2 on
> Windows, WebKitGTK on Linux). Backend logic and tests run fully on any host;
> the GUI binary is verified by cross-compiling to Windows.

---

## Usage

Run the GUI binary (`NDE.exe` on Windows) to open the desktop application. On
first launch NDE seeds a small set of safe external targets so the dashboard
shows data immediately; use the **Targets** page to add your own. Press
**Start monitoring** (or stop it) at any time — the toggle pauses and resumes
collection without losing the configured target set.

The **Collectors** page shows which measurement types are ready to run. The
**Dashboard** shows live statistics and the RTT timeline; **History** and the
per-target timeline let you look back at recent measurements.

---

## Design decisions

Recorded as ADRs under `docs/decisions/`. The headline choices:

1. **Pure-Go SQLite (`modernc.org/sqlite`)** — no CGO, which makes clean
   Windows cross-compilation possible from a single `go build`.
2. **Unprivileged ICMP** — collectors delegate to the OS `ping` binary, which
   uses `IcmpSendEcho` on Windows and the kernel ICMP facilities on Linux/macOS,
   so echo checks never require administrator rights. TCP-connect RTT is always
   available as a fallback.
3. **Typed event bus** — analytics, diagnostics and the UI communicate through
   a typed in-process bus; the Wails layer subscribes and re-emits to the
   frontend. Producers stay decoupled from the UI framework.
4. **Domain isolated from Wails** — only `cmd/app` and `internal/app/bindings`
   import Wails. Everything else is headless-testable.
5. **Time stored as INTEGER Unix-milliseconds UTC**; latency as milliseconds;
   measurement `metadata` as JSON text — so the schema is portable and the
   insufficient-data state is explicit (a NULL latency means "no value", never
   a sentinel like 0 or -1).
6. **Resolution-time SSRF screening** — targets are screened when configured
   and again when each check resolves the hostname, blocking hostnames that
   resolve to private or reserved addresses (including the cloud metadata
   endpoint).

---

## Roadmap

NDE's foundation, networking and storage layers are complete and in use. The
following modules extend the product toward its full diagnostics vision and are
under active development:

- **Metrics aggregation** — rolling RTT / jitter / packet-loss aggregates and
  baseline tracking.
- **Anomaly detection** — EMA / moving-average / Z-score based spike and
  degradation detection.
- **Explainable diagnostics** — a rule engine that turns anomalies into
  hypotheses about *which network segment* is at fault, each backed by
  supporting evidence and a confidence score.
- **Notifications** — threshold-based alerts with a dismissible history.
- **Reporting & export** — summary reports and data export.
