# ADR-006: Schema gap-fixes beyond SPEC §6

**Status:** Accepted
**Date:** 2026-11-19
**Context:** SPEC §6 lists the core tables, but several SPEC requirements have
no place to live in that literal schema. These gaps are documented in
`phase-1-plan.md` §2.1 (C3, C5, C6, C7).

**Decision:** Extend the schema in `0001_init.sql` with:
- `metric_aggregates.status TEXT` (`ok|insufficient_data|error`) — the
  `insufficient_data` state required by SPEC §4.2/§4.3 cannot be expressed by a
  numeric column alone (C3).
- `diagnostic_recommendations(event_id, text, priority)` — SPEC §4.8 requires
  each hypothesis to carry Recommendations, absent from §6 (C5).
- `anomaly_measurements(anomaly_id, measurement_id)` — SPEC §4.7 requires an
  anomaly to carry "related measurements", absent from §6 (C6).
- `notifications(id, level, event_type, message, target_id, created_at,
  dismissed_at)` — SPEC §4.10 needs notification levels/events and §5.6 a
  settings UI; a history table is missing from §6 (C7).

**Consequences:** These are additive (new tables/columns), so they do not
deviate from the SPEC's intended model — they make implicit requirements
explicit. The SPEC §6 table set itself is preserved unchanged. `group` is
stored as column `target_group` because `group` is a SQLite reserved word.

**Alternatives considered:** Encode recommendations/evidence as JSON inside
`diagnostic_events.hypothesis` (rejected — unqueryable); omit the gap-fixes
(rejected — SPEC requirements would be unimplementable).
