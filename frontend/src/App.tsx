import { Routes, Route, Navigate } from 'react-router-dom'
import { Sidebar } from '@/components/layout/Sidebar'
import { DashboardPage } from '@/features/dashboard'
import { CollectorsPage } from '@/features/collectors'
import { DiagnosticsPage } from '@/features/diagnostics'
import { TargetsPage } from '@/features/targets'
import { HistoryPage } from '@/features/history'
import { NetworkPage } from '@/features/network'
import { SettingsPage } from '@/features/settings'
import { useNdeEventSync } from '@/hooks/use-nde'

/**
 * Root application shell.
 * Dark, dashboard-style layout: fixed sidebar + scrollable content area.
 * Phase 2 wires live backend events through useNdeEventSync so the dashboard
 * and targets reflect measurements and monitoring state in real time.
 */
export default function App() {
  useNdeEventSync()
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <Sidebar />
      <main className="flex-1 overflow-y-auto px-6 py-6">
        <Routes>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/collectors" element={<CollectorsPage />} />
          <Route path="/diagnostics" element={<DiagnosticsPage />} />
          <Route path="/targets" element={<TargetsPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/network" element={<NetworkPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </main>
    </div>
  )
}
