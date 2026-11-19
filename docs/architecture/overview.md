# NDE — Architecture Overview

Network Diagnostics Engine (NDE) is a Modular Monolith desktop application:
one Go binary embedding a React/TypeScript frontend via Wails v3, persisting
to a local SQLite database.

## Layers

```
┌──────────────────────────────────────────────────────────────┐
│  Desktop UI  (React + TS + Tailwind + shadcn/ui + Recharts)   │
└────────────▲───────────────────────────────────▲─────────────┘
             │ Wails bindings (DTOs)             │ Wails events (SPEC §8)
┌────────────┴───────────────────────────────────┴─────────────┐
│  cmd/app  (entry point; headless or GUI via build tag "gui")  │
│  internal/app/bindings  (plain-Go bound services; no Wails)   │
└────────────▲──────────────────────────────────────────────────┘
             │
┌────────────┴──────────────────────────────────────────────────┐
│  Application Core (internal/core)                              │
│  lifecycle · module registry · ordered graceful shutdown       │
│  + config · errors · events (bus) · logging                   │
└────────────▲──────────────────────────────────────────────────┘
             │
┌────────────┴──────────────────────────────────────────────────┐
│  Domain modules                                                │
│  network/{discovery,icmp,tcp,dns,http,traceroute} · collector │
│  scheduler · metrics/{calculator,aggregation}                  │
│  analytics/{baseline,anomaly,correlation}                      │
│  diagnostics/{engine,rules,evidence,confidence} · reporting    │
└────────────▲──────────────────────────────────────────────────┘
             │
┌────────────┴──────────────────────────────────────────────────┐
│  Storage (internal/storage) — repository INTERFACES            │
│  sqlite impl · embedded migrations                             │
└───────────────────────────────────────────────────────────────┘
```

## Hard boundaries (SPEC §3.3, §4.8, rule 9)

1. **Collectors carry no diagnostics.** `internal/network/*` and
   `internal/collector` only produce `measurement.Measurement`.
2. **Diagnostics depend on results, not collectors.** `internal/diagnostics`
   consumes metrics + analytics events + a `MeasurementReader` interface; it
   never imports a concrete collector or the UI.
3. **Wails is an adapter, not a dependency of the domain.** Only `cmd/app` and
   `internal/app/bindings` may import Wails. The entire domain is testable
   headless.

## Communication

- **Synchronous**: Core → collectors/metrics/storage via direct interface calls.
- **Asynchronous, decoupled**: producers publish to the typed `events.Bus`
  (`internal/events`, SPEC §8 names). The Wails GUI layer subscribes and
  re-emits to the frontend. In tests, in-memory subscribers assert behaviour.

## Build modes

| Mode | Entry | Build | Use |
|------|-------|-------|-----|
| Headless (default) | `headless.go` (`!gui`) | `go build ./cmd/app` | CI, tests, daemon |
| GUI | `gui.go` (`gui`) | `go build -tags=gui ./cmd/app` | Desktop app |

Both share bootstrap (config → logging → storage → migrate → core). The
difference is the event loop (signal wait vs. Wails `Run`) and that the GUI
mode registers bound services + embedded assets.

## Cross-compilation

`modernc.org/sqlite` (pure-Go) + `gopsutil/v4` (pure-Go on Windows) +
Wails v3 (WebView2 on Windows, pure-Go) → `CGO_ENABLED=0` Windows build:

```
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags=gui -o bin/NDE.exe ./cmd/app
```

Verified working in Phase 1 (24.6 MB binary).

## Phase status

- **Phase 1 — Foundation: COMPLETE.** Module, structure, headless + GUI entry,
  Application Core (lifecycle, config, events, logging, errors, measurement,
  target, collector/storage interfaces), SQLite + WAL + embedded migrations,
  Wails v3 GUI shell with bound `AppService` + embedded assets, frontend
  scaffold (6 route shells, dark theme, Tailwind v4, shadcn-style components,
  Zustand + TanStack Query), VSCode config, CI workflow, ADRs.
- Phases 2–8 per `docs/decisions/phase-1-plan.md` and SPEC §12.
