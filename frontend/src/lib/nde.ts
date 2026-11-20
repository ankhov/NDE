/**
 * Typed client for the Wails v3 bound services (Phase 2).
 *
 * Wails v3 exposes bound Go methods through `Call.ByName(fqn, ...args)` where
 * the FQN is `packagepath.TypeName.Method`. The Go side injects the
 * context.Context first argument, so JS callers pass only the user args. This
 * mirrors the contract in internal/app/bindings (see dto.go for the shapes).
 *
 * The FQN prefix matches the module path in go.mod (github.com/ankhov/NDE) and
 * the bindings package (internal/app/bindings). If the module path or service
 * types are renamed, update PREFIX only.
 */
import { Call, Events } from '@wailsio/runtime'
import type {
  CollectorStatus,
  HealthStatus,
  Measurement,
  MeasurementStats,
  Target,
  TargetInput,
} from '@/types'

const PREFIX = 'github.com/ankhov/NDE/internal/app/bindings'

async function invoke<T>(method: string, ...args: unknown[]): Promise<T> {
  // Call.ByName returns a (cancellable) promise; awaiting yields the JSON
  // result decoded into the requested TS shape.
  return (await Call.ByName(`${PREFIX}.${method}`, ...args)) as T
}

export const nde = {
  App: {
    Version: () => invoke<string>('AppService.Version'),
    Health: () => invoke<HealthStatus>('AppService.Health'),
  },
  Target: {
    List: () => invoke<Target[]>('TargetService.List'),
    Add: (input: TargetInput) => invoke<Target>('TargetService.Add', input),
    Delete: (id: string) => invoke<void>('TargetService.Delete', id),
    SetEnabled: (id: string, enabled: boolean) =>
      invoke<Target>('TargetService.SetEnabled', id, enabled),
  },
  Measurement: {
    Recent: (limit: number) =>
      invoke<Measurement[]>('MeasurementService.Recent', limit),
    ByTarget: (targetId: string, limit: number) =>
      invoke<Measurement[]>('MeasurementService.ByTarget', targetId, limit),
    Timeline: (targetId: string, limit: number) =>
      invoke<Measurement[]>('MeasurementService.Timeline', targetId, limit),
    Stats: () => invoke<MeasurementStats>('MeasurementService.Stats'),
  },
  Monitoring: {
    Start: () => invoke<void>('MonitoringService.Start'),
    Stop: () => invoke<void>('MonitoringService.Stop'),
    IsActive: () => invoke<boolean>('MonitoringService.IsActive'),
  },
  Collector: {
    Status: () => invoke<CollectorStatus[]>('CollectorService.Status'),
    ReadyTypes: () => invoke<string[]>('CollectorService.ReadyTypes'),
  },
} as const

/**
 * Subscribe to a backend event (SPEC §8 names: measurement:new,
 * monitoring:started/stopped, target:status_changed, ...). The callback
 * receives the event payload (ev.data). Returns an unsubscribe function.
 */
export function onNdeEvent(name: string, cb: (data: unknown) => void): () => void {
  return Events.On(name, (ev) => cb(ev.data))
}
