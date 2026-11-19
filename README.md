# Network Diagnostics Engine (NDE)

Desktop application for continuous monitoring, analysis and explainable
diagnostics of network connection quality. Determines **which segment** of the
network path is unstable, **which measurements** support that conclusion, and
**how confident** the diagnostic hypothesis is.

> Спецификация: [`upload/SPEC.md`](upload/SPEC.md) v1.0.
> Анализ и план Phase 1: [`docs/decisions/phase-1-plan.md`](docs/decisions/phase-1-plan.md).

---

## Status

| Phase | Scope | Status |
|-------|-------|--------|
| 1 — Foundation | Go/Wails/React/TS init, structure, SQLite, migrations, Application Core | ✅ Complete |
| 2 — Networking | Discovery, ICMP/TCP/DNS/HTTP collectors, Scheduler | ⬜ Next |
| 3 — Storage & Metrics | Unified model, repository, RTT/Jitter/PacketLoss, aggregation | ⬜ Not started |
| 4 — Analytics | Baseline, EMA, MA, Z-score, anomaly detection | ⬜ Not started |
| 5 — Diagnostics | Rule engine, evidence, confidence, reports | ⬜ Not started |
| 6 — UI | Dashboard/Targets/Diagnostics/History/Network/Settings | ⬜ Not started |
| 7 — Advanced | Traceroute, route comparison, notifications, export | ⬜ Not started |
| 8 — Testing & Release | Unit/integration/scenario/perf, CI/CD, Windows build, docs | ⬜ Not started |

Development follows SPEC §14: small verifiable steps, plan before each phase,
no stub implementations, `context.Context` everywhere.

---

## Tech stack (SPEC §2 — unchanged)

- **Backend:** Go 1.27.4 (≥1.26), Wails v3.1.0, stdlib, `golang.org/x/net`,
  `github.com/miekg/dns`, `github.com/shirou/gopsutil/v4`, `modernc.org/sqlite` (pure-Go, no CGO).
- **Frontend:** React 18, TypeScript, Vite 6, Tailwind CSS v4, shadcn/ui, Recharts, Zustand, TanStack Query.
- **Database:** SQLite (WAL, time-series indexes, migrations, batch writes, retention).
- **Testing:** Go testing + testify; Vitest + React Testing Library + Playwright.
- **CI/CD:** Git, GitHub, GitHub Actions.

---

## Repository layout (SPEC §13)

See [`docs/decisions/phase-1-plan.md`](docs/decisions/phase-1-plan.md) §4 for the
full annotated tree. Key parts:

```
cmd/app/            entry point + Wails shell
internal/
  core/             Application Core: lifecycle, module registry, shutdown
  config/ errors/ events/ logging/   cross-cutting
  measurement/ target/               unified models
  network/{discovery,icmp,tcp,dns,http,traceroute}/
  collector/ scheduler/
  metrics/{calculator,aggregation}/
  analytics/{baseline,anomaly,correlation}/
  diagnostics/{engine,rules,evidence,confidence}/
  storage/{sqlite,migrations}/       repository interfaces + sqlite impl + embedded SQL
  reporting/
frontend/src/{components,features/*,hooks,stores,types}/
tests/{integration,scenarios,fixtures}/
docs/{architecture,algorithms,decisions}/
.github/workflows/  .vscode/  scripts/
```

---

## Development environment

The dev host is Linux; the build target is Windows 10/11 x64 (SPEC §9.4).
The Go toolchain is installed inside the repo (gitignored) so the sandbox is
self-contained — no system-wide install, no writes outside the project.

```bash
# One-time: install Go 1.27.4 into .sdk/go
mkdir -p .sdk
curl -fsSL -o /tmp/go.tgz https://go.dev/dl/go1.27.4.linux-amd64.tar.gz
tar -xzf /tmp/go.tgz -C .sdk && rm /tmp/go.tgz

# Every shell: source the env helper (sets GOROOT/GOPATH/GOCACHE/PATH)
source scripts/env.sh
```

## Build & test

```bash
source scripts/env.sh

# Backend (headless default build)
go build ./...
go vet ./...
go test ./...

# Frontend
cd frontend && npm install && npm run typecheck && npm run test && npm run build
```

### Two build modes

The binary has two entry points selected by build tag:

```bash
# Headless (default) — runs the Application Core until SIGINT/SIGTERM.
go build -o bin/nde ./cmd/app

# GUI — Wails v3 desktop app with bound services + embedded frontend assets.
# Build the frontend first so frontend/dist exists for the embed.
(cd frontend && npm run build)
go build -tags=gui -o bin/nde ./cmd/app
```

### Cross-compile a Windows binary (CGO-free)

`modernc.org/sqlite`, `gopsutil/v4` and Wails v3 are all pure-Go on Windows,
so the Windows build needs no C toolchain:

```bash
# Headless Windows binary
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/nde.exe ./cmd/app

# GUI Windows binary (verified: ~24.6 MB)
(cd frontend && npm run build)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags=gui -o bin/NDE.exe ./cmd/app
```

> Note: `wails3 dev` (live GUI) requires a native WebView backend (WebView2 on
> Windows, WebKitGTK on Linux). Backend logic, unit and integration tests run
> fully on this Linux host; the GUI build is verified by cross-compiling to
> Windows (above) and by CI.

---

## Architectural decisions

Recorded as ADRs under `docs/decisions/`. Headline decisions:

1. **Pure-Go SQLite (`modernc.org/sqlite`)** — no CGO, enables clean Windows cross-compilation.
2. **Windows unprivileged ICMP** — use `IcmpSendEcho` syscall (no admin), TCP-connect RTT as always-available fallback.
3. **Typed event bus** decouples analytics→diagnostics→UI; Wails layer subscribes and re-emits SPEC §8 events.
4. **Domain isolated from Wails** — only `cmd/app` and `internal/app/bindings` import Wails; everything else is headless-testable.
5. **Time stored as INTEGER Unix-ms UTC**; latency as ms; `metadata` as JSON text.
6. **Gap-fixes to SPEC §6 schema**: `diagnostic_recommendations`, `anomaly_measurements`, `notifications`, `status` on `metric_aggregates`.

See the plan doc for contradictions, missing requirements and the full risk register.
