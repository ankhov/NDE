# ADR-004: Domain isolation from Wails

**Status:** Accepted
**Date:** 2026-11-19
**Context:** SPEC §3 (Modular Monolith), §4.8 (diagnostic rules independent of
UI and collectors) and rule 9 (no mixing layers) require the domain to be
testable without the GUI framework. Wails v3 is newer and its API may evolve
(risk R2).

**Decision:** Only `cmd/app` and `internal/app/bindings` may import
`github.com/wailsapp/wails/v3`. Every other package depends on interfaces:
- collectors implement `collector.Collector`;
- storage implements `storage.Repository`/`storage.Migrator`;
- the domain consumes `events.Bus`, `storage.MeasurementReader`, etc.

Bound services in `internal/app/bindings` are plain Go structs (no Wails
import), so they are unit-tested headless.

**Consequences:** `go build ./...` and `go test ./...` run fully without a
WebView backend (no WebKitGTK needed on the Linux dev host). The Wails GUI
build (`-tags=gui`) is verified separately by cross-compiling to Windows.

**Alternatives considered:** Letting domain packages import Wails directly
(rejected — untestable, locks the domain to Wails API churn).
