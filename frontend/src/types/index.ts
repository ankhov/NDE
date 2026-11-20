/**
 * Shared frontend types — mirror the Go models in internal/measurement and the
 * SPEC §6/§7/§8 contracts. Kept in sync manually (SPEC rule 8: no business
 * logic duplication; these are DTO shapes only).
 */

export type MeasurementType = 'icmp' | 'tcp' | 'dns' | 'http' | 'traceroute'

export type TargetGroup = 'local' | 'external' | 'dns' | 'custom'

export type Severity = 'INFO' | 'WARNING' | 'CRITICAL'

export type DiagnosticCategory =
  | 'LOCAL_NETWORK'
  | 'GATEWAY'
  | 'DNS'
  | 'ISP_OR_UPSTREAM'
  | 'ROUTING'
  | 'REMOTE_SERVER'
  | 'UNKNOWN'

export type AnomalyType =
  | 'latency_spike'
  | 'high_jitter'
  | 'packet_loss_increase'
  | 'dns_degradation'
  | 'target_unavailable'
  | 'route_change'
  | 'periodic_instability'

export interface Measurement {
  targetId: string
  timestamp: string // ISO-8601 UTC
  type: MeasurementType
  /** Latency in ms; null when there is no value (timeout / insufficient_data). */
  latency: number | null
  success: boolean
  errorCode: string
  metadata: Record<string, unknown>
}

export interface Target {
  id: string
  name: string
  host: string
  protocol: MeasurementType
  port: number
  /** Check interval in ms. */
  checkInterval: number
  /** Timeout in ms. */
  timeout: number
  enabled: boolean
  warningThreshold: number
  criticalThreshold: number
  group: TargetGroup
}

export interface Anomaly {
  id: string
  targetId: string
  anomalyType: AnomalyType
  severity: Severity
  startedAt: string
  endedAt: string | null
  baselineValue: number | null
  observedValue: number | null
}

export interface DiagnosticEvidence {
  id: string
  diagnosticEventId: string
  measurementId: string | null
  evidenceType: 'supporting' | 'counter'
  description: string
  weight: number
}

export interface DiagnosticEvent {
  id: string
  startedAt: string
  endedAt: string | null
  category: DiagnosticCategory
  hypothesis: string
  /** Heuristic confidence 0..100 (SPEC §4.8). */
  confidence: number
  severity: Severity
  status: 'active' | 'resolved' | 'superseded'
  evidence: DiagnosticEvidence[]
  recommendations: string[]
}

// ---------------------------------------------------------------------------
// Phase 2 — DTOs mirrored from internal/app/bindings/dto.go (camelCase JSON).
// These are the shapes that cross the Wails boundary via Call.ByName.
// ---------------------------------------------------------------------------

/** Application health, returned by AppService.Health. */
export interface HealthStatus {
  ok: boolean
  version: string
  startedAt: string
}

/** Input for creating a target; the id is generated server-side. */
export interface TargetInput {
  name: string
  host: string
  protocol: MeasurementType
  port: number
  /** Check interval in ms. */
  checkInterval: number
  /** Timeout in ms. */
  timeout: number
  enabled: boolean
  warningThreshold: number
  criticalThreshold: number
  group: TargetGroup
}

/** Dashboard summary, computed by MeasurementService.Stats. */
export interface MeasurementStats {
  /** Avg RTT in ms, null when there is no data. */
  avgRtt: number | null
  /** P95 RTT in ms, null when there is no data. */
  p95Rtt: number | null
  /** Mean absolute delta (jitter) in ms, null when <2 samples. */
  jitter: number | null
  /** Packet-loss percentage 0..100. */
  packetLoss: number
  sampleCount: number
  activeTargets: number
}

// ---------------------------------------------------------------------------
// Phase 3 — collector status (CollectorService.Status), mirrored from
// internal/app/bindings/collector.go.
// ---------------------------------------------------------------------------

/** Availability of one measurement type's collector. */
export interface CollectorStatus {
  /** Measurement type: icmp / tcp / dns / http / traceroute. */
  type: MeasurementType
  /** Whether a collector is registered for this type. */
  ready: boolean
  /** Registered collector's name when ready, else ''. */
  collectorName: string
}
