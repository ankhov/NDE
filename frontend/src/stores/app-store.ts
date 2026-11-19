import { create } from 'zustand'

/**
 * Global UI/app state (Zustand).
 * Network-bound state (measurements, anomalies, diagnostics) is fetched via
 * TanStack Query hooks and Wails bindings in Phase 6 — not stored here.
 */
interface AppState {
  /** Whether background monitoring is running. Toggled by monitoring:started/stopped events. */
  monitoring: boolean
  setMonitoring: (v: boolean) => void

  /** Auto-refresh cadence (ms) for live charts. */
  refreshIntervalMs: number
  setRefreshIntervalMs: (v: number) => void
}

export const useAppStore = create<AppState>((set) => ({
  monitoring: false,
  setMonitoring: (v) => set({ monitoring: v }),
  refreshIntervalMs: 5000,
  setRefreshIntervalMs: (v) => set({ refreshIntervalMs: v }),
}))
