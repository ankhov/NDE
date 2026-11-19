# ADR-001: Use pure-Go SQLite (modernc.org/sqlite)

**Status:** Accepted
**Date:** 2026-11-19
**Context:** SPEC §2.1 mandates SQLite and `modernc.org/sqlite`; SPEC §9.4
targets Windows 10/11 x64 with future Linux/macOS support.

**Decision:** Use `modernc.org/sqlite` (pure-Go, no CGO) as the SQLite driver.
Open the database with `journal_mode=WAL`, `busy_timeout=5000`,
`foreign_keys=ON`, `synchronous=NORMAL`, `temp_store=MEMORY`; constrain
`SetMaxOpenConns(1)` for the single-writer model that WAL + batch writes need.

**Consequences:**
- The whole binary cross-compiles to Windows with `CGO_ENABLED=0` — no C
  toolchain required on the build host (verified: 24.6 MB Windows GUI binary).
- WAL requires a local filesystem (fine for desktop); `-wal`/`-shm` side files
  appear next to the database.
- Throughput is adequate for the NDE write pattern (batch inserts + retention +
  aggregation); a benchmark is scheduled for Phase 3 (risk R3).

**Alternatives considered:** `mattn/go-sqlite3` (CGO — breaks clean Windows
cross-compilation); `crawshaw.io/sqlite` (CGO, archived).
