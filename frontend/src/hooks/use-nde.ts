/**
 * Data hooks (TanStack Query) over the Wails v3 bindings (Phase 2).
 *
 * Reads are cached and re-fetched on an interval or on event invalidation.
 * Mutations invalidate the relevant queries so the UI refreshes immediately.
 */
import { useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { nde, onNdeEvent } from '@/lib/nde'
import type { MeasurementStats, TargetInput } from '@/types'

const RECENT_LIMIT = 60

/** Build version (cached forever). */
export function useAppVersion() {
  return useQuery({
    queryKey: ['app', 'version'],
    queryFn: () => nde.App.Version(),
    staleTime: Infinity,
  })
}

/** Whether monitoring is active. Polled as a fallback; events also refresh it. */
export function useMonitoringStatus() {
  return useQuery({
    queryKey: ['app', 'monitoring'],
    queryFn: () => nde.Monitoring.IsActive(),
    refetchInterval: 5000,
    staleTime: 2000,
  })
}

/** All configured targets. */
export function useTargets() {
  return useQuery({
    queryKey: ['targets'],
    queryFn: () => nde.Target.List(),
  })
}

/** Recent measurements across all targets (newest first). */
export function useRecentMeasurements(limit = RECENT_LIMIT) {
  return useQuery({
    queryKey: ['measurements', 'recent', limit],
    queryFn: () => nde.Measurement.Recent(limit),
    refetchInterval: 5000,
  })
}

/** Chronological timeline for one target (oldest first). */
export function useTargetTimeline(targetId: string | null, limit = 60) {
  return useQuery({
    queryKey: ['measurements', 'timeline', targetId, limit],
    queryFn: () =>
      targetId ? nde.Measurement.Timeline(targetId, limit) : Promise.resolve([]),
    enabled: !!targetId,
    refetchInterval: 5000,
  })
}

/** Dashboard summary stats (avg/p95/jitter/loss). */
export function useStats() {
  return useQuery({
    queryKey: ['stats'],
    queryFn: () => nde.Measurement.Stats() as Promise<MeasurementStats>,
    refetchInterval: 5000,
  })
}

// --- Mutations ---------------------------------------------------------------

export function useAddTarget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: TargetInput) => nde.Target.Add(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['targets'] })
      qc.invalidateQueries({ queryKey: ['stats'] })
    },
  })
}

export function useDeleteTarget() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => nde.Target.Delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['targets'] })
      qc.invalidateQueries({ queryKey: ['stats'] })
      qc.invalidateQueries({ queryKey: ['measurements'] })
    },
  })
}

export function useToggleTargetEnabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      nde.Target.SetEnabled(id, enabled),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['targets'] }),
  })
}

export function useStartMonitoring() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => nde.Monitoring.Start(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['app', 'monitoring'] }),
  })
}

export function useStopMonitoring() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => nde.Monitoring.Stop(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['app', 'monitoring'] }),
  })
}

/**
 * Subscribe to backend events that refresh live data. Call once in <App/>:
 *
 *   measurement:new        → refresh measurements + stats
 *   monitoring:started     → refresh monitoring flag
 *   monitoring:stopped     → refresh monitoring flag
 *   target:status_changed  → refresh targets
 */
export function useNdeEventSync() {
  const qc = useQueryClient()
  useEffect(() => {
    const off1 = onNdeEvent('measurement:new', () => {
      qc.invalidateQueries({ queryKey: ['measurements'] })
      qc.invalidateQueries({ queryKey: ['stats'] })
    })
    const off2 = onNdeEvent('monitoring:started', () =>
      qc.invalidateQueries({ queryKey: ['app', 'monitoring'] }),
    )
    const off3 = onNdeEvent('monitoring:stopped', () =>
      qc.invalidateQueries({ queryKey: ['app', 'monitoring'] }),
    )
    const off4 = onNdeEvent('target:status_changed', () =>
      qc.invalidateQueries({ queryKey: ['targets'] }),
    )
    return () => {
      off1()
      off2()
      off3()
      off4()
    }
  }, [qc])
}
