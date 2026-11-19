-- 0001_init.sql — initial schema for Network Diagnostics Engine (SPEC §6).
--
-- Conventions:
--   * Timestamps are INTEGER (Unix milliseconds, UTC) per SPEC §6 "UTC".
--   * Latencies are REAL (milliseconds, fractional) per SPEC §4.1.
--   * Intervals/timeouts are INTEGER milliseconds.
--   * Booleans are INTEGER (0/1).
--   * metadata is TEXT (JSON) — the storage layer (de)serialises map[string]any.
--   * Confidence is INTEGER 0..100 (heuristic, SPEC §4.8).
--
-- Gap-fixes beyond the literal SPEC §6 list (documented in
-- docs/decisions/phase-1-plan.md §2.1 C5/C6/C7):
--   * metric_aggregates.status          — insufficient_data / ok / error (C3)
--   * diagnostic_recommendations        — recommendations per hypothesis (C5)
--   * anomaly_measurements              — link anomalies to measurements (C6)
--   * notifications                     — notification history (C7)

PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- targets (SPEC §4.5, §6)
-- ---------------------------------------------------------------------------
CREATE TABLE targets (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    host               TEXT NOT NULL,
    protocol           TEXT NOT NULL,
    port               INTEGER NOT NULL DEFAULT 0,
    check_interval_ms  INTEGER NOT NULL,
    timeout_ms         INTEGER NOT NULL,
    enabled            INTEGER NOT NULL DEFAULT 1,
    warning_threshold  REAL NOT NULL DEFAULT 0,
    critical_threshold REAL NOT NULL DEFAULT 0,
    target_group       TEXT NOT NULL DEFAULT 'custom',
    created_at         INTEGER NOT NULL
);

-- ---------------------------------------------------------------------------
-- measurements (SPEC §6) — high-volume time series
-- ---------------------------------------------------------------------------
CREATE TABLE measurements (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id       TEXT NOT NULL,
    timestamp       INTEGER NOT NULL,
    measurement_type TEXT NOT NULL,
    latency_ms      REAL,            -- NULL = no value (timeout / insufficient)
    success         INTEGER NOT NULL DEFAULT 0,
    error_code      TEXT NOT NULL DEFAULT '',
    metadata        TEXT,            -- JSON
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);
CREATE INDEX idx_measurements_target_time ON measurements (target_id, timestamp);
CREATE INDEX idx_measurements_time        ON measurements (timestamp);
CREATE INDEX idx_measurements_type        ON measurements (measurement_type);

-- ---------------------------------------------------------------------------
-- network_interfaces (SPEC §6)
-- ---------------------------------------------------------------------------
CREATE TABLE network_interfaces (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    ip_address     TEXT NOT NULL DEFAULT '',
    mac_address    TEXT NOT NULL DEFAULT '',
    interface_type TEXT NOT NULL DEFAULT '',
    is_active      INTEGER NOT NULL DEFAULT 0,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX idx_interfaces_active ON network_interfaces (is_active);

-- ---------------------------------------------------------------------------
-- routes / route_hops (SPEC §6)
-- ---------------------------------------------------------------------------
CREATE TABLE routes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id  TEXT NOT NULL,
    timestamp  INTEGER NOT NULL,
    route_hash TEXT NOT NULL,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE
);
CREATE INDEX idx_routes_target_time ON routes (target_id, timestamp);

CREATE TABLE route_hops (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    route_id   INTEGER NOT NULL,
    hop_number INTEGER NOT NULL,
    address    TEXT NOT NULL DEFAULT '',
    latency_ms REAL,
    responded  INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (route_id) REFERENCES routes(id) ON DELETE CASCADE
);
CREATE INDEX idx_route_hops_route ON route_hops (route_id);

-- ---------------------------------------------------------------------------
-- anomalies (SPEC §6) + anomaly_measurements (gap-fix C6)
-- ---------------------------------------------------------------------------
CREATE TABLE anomalies (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id      TEXT,
    anomaly_type   TEXT NOT NULL,
    severity       TEXT NOT NULL,
    started_at     INTEGER NOT NULL,
    ended_at       INTEGER,
    baseline_value REAL,
    observed_value REAL,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE SET NULL
);
CREATE INDEX idx_anomalies_target_start ON anomalies (target_id, started_at);
CREATE INDEX idx_anomalies_open         ON anomalies (ended_at) WHERE ended_at IS NULL;

CREATE TABLE anomaly_measurements (
    anomaly_id    INTEGER NOT NULL,
    measurement_id INTEGER NOT NULL,
    PRIMARY KEY (anomaly_id, measurement_id),
    FOREIGN KEY (anomaly_id) REFERENCES anomalies(id) ON DELETE CASCADE,
    FOREIGN KEY (measurement_id) REFERENCES measurements(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- diagnostic_events / diagnostic_evidence (SPEC §6)
-- ---------------------------------------------------------------------------
CREATE TABLE diagnostic_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    category   TEXT NOT NULL,
    hypothesis TEXT NOT NULL,
    confidence INTEGER NOT NULL DEFAULT 0,   -- 0..100
    severity   TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active' -- active | resolved | superseded
);
CREATE INDEX idx_diag_events_started   ON diagnostic_events (started_at);
CREATE INDEX idx_diag_events_cat_status ON diagnostic_events (category, status);

CREATE TABLE diagnostic_evidence (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    diagnostic_event_id INTEGER NOT NULL,
    measurement_id      INTEGER,
    evidence_type       TEXT NOT NULL,        -- supporting | counter
    description         TEXT NOT NULL,
    weight              INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (diagnostic_event_id) REFERENCES diagnostic_events(id) ON DELETE CASCADE,
    FOREIGN KEY (measurement_id) REFERENCES measurements(id) ON DELETE SET NULL
);
CREATE INDEX idx_diag_evidence_event ON diagnostic_evidence (diagnostic_event_id);

-- diagnostic_recommendations (gap-fix C5: SPEC §4.8 "Recommendations")
CREATE TABLE diagnostic_recommendations (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id  INTEGER NOT NULL,
    text      TEXT NOT NULL,
    priority  INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (event_id) REFERENCES diagnostic_events(id) ON DELETE CASCADE
);
CREATE INDEX idx_diag_recs_event ON diagnostic_recommendations (event_id);

-- ---------------------------------------------------------------------------
-- metric_aggregates (SPEC §6) + status (gap-fix C3)
-- ---------------------------------------------------------------------------
CREATE TABLE metric_aggregates (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id     TEXT,
    interval_start INTEGER NOT NULL,
    interval_type  TEXT NOT NULL,             -- minute | hour | day
    avg_latency   REAL,
    p95_latency   REAL,
    jitter        REAL,
    packet_loss   REAL,
    sample_count  INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'ok', -- ok | insufficient_data | error
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE SET NULL
);
CREATE INDEX idx_aggregates_target_interval ON metric_aggregates (target_id, interval_type, interval_start);

-- ---------------------------------------------------------------------------
-- notifications (gap-fix C7: SPEC §4.10 history)
-- ---------------------------------------------------------------------------
CREATE TABLE notifications (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    level        TEXT NOT NULL,               -- INFO | WARNING | CRITICAL
    event_type   TEXT NOT NULL,
    message      TEXT NOT NULL,
    target_id    TEXT,
    created_at   INTEGER NOT NULL,
    dismissed_at INTEGER,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE SET NULL
);
CREATE INDEX idx_notifications_created ON notifications (created_at);
CREATE INDEX idx_notifications_level   ON notifications (level);

-- ---------------------------------------------------------------------------
-- settings (SPEC §6) — key/value runtime configuration
-- ---------------------------------------------------------------------------
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
