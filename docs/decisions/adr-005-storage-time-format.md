# ADR-005: Storage time and value format

**Status:** Accepted
**Date:** 2026-11-19
**Context:** SPEC §6 says timestamps are UTC; §4.1 says latencies are
milliseconds. The schema lists `timestamp` columns without a type.

**Decision:**
- All timestamp columns (`timestamp`, `started_at`, `ended_at`, `created_at`,
  `updated_at`, `applied_at`, `interval_start`) are `INTEGER` storing
  **Unix milliseconds, UTC**. This is index-friendly for time-series range
  queries and avoids text-comparison pitfalls.
- Latency columns (`latency_ms`, `avg_latency`, `p95_latency`, `jitter`,
  `baseline_value`, `observed_value`) are `REAL` milliseconds (fractional).
- `metadata` is `TEXT` (JSON); the storage layer (de)serialises
  `map[string]any`.
- Booleans are `INTEGER` 0/1.
- Confidence is `INTEGER 0..100` (heuristic, not probability — SPEC §4.8);
  bands 0–29 low, 30–69 medium, 70–100 high.
- Severity is `TEXT` enum `INFO|WARNING|CRITICAL` (matches SPEC §4.10).
- Absent latency is `NULL` (the Go model uses `*float64`; nil = no value,
  encoding the `insufficient_data` state per SPEC §4.2 — never a sentinel).

**Consequences:** Storage conversions are localised to the SQLite repository
(Phase 3); the domain works with `time.Time` and `*float64`. Indexes on
`(target_id, timestamp)` give efficient windowed scans.

**Alternatives considered:** ISO-8601 text timestamps (rejected — slower
range scans, larger rows); float confidence 0..1 (rejected — less DB-friendly).
