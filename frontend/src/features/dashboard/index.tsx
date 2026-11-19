import { useEffect } from 'react'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Pause, Play } from 'lucide-react'
import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { useAppStore } from '@/stores/app-store'
import {
  useMonitoringStatus,
  useRecentMeasurements,
  useStartMonitoring,
  useStats,
  useStopMonitoring,
} from '@/hooks/use-nde'

interface StatCardProps {
  label: string
  value: string
  hint?: string
}

function StatCard({ label, value, hint }: StatCardProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
        {hint && <div className="mt-1 text-xs text-muted-foreground">{hint}</div>}
      </CardContent>
    </Card>
  )
}

function fmtMs(v: number | null | undefined): string {
  if (v == null) return '—'
  return `${v.toFixed(1)} ms`
}
function fmtPct(v: number | undefined): string {
  if (v == null) return '—'
  return `${v.toFixed(1)}%`
}

/**
 * Dashboard (SPEC §5.1).
 * Phase 2: live stat cards + RTT timeline from real measurements, plus a
 * working Start/Stop monitoring toggle bound to MonitoringService.
 */
export function DashboardPage() {
  const { monitoring, setMonitoring } = useAppStore()
  const status = useMonitoringStatus()
  const stats = useStats()
  const recent = useRecentMeasurements(60)
  const startMut = useStartMonitoring()
  const stopMut = useStopMonitoring()

  // Single source of truth for the monitoring flag is the backend query; mirror
  // it into the store so the Sidebar indicator and the button stay in sync.
  useEffect(() => {
    setMonitoring(status.data ?? false)
  }, [status.data, setMonitoring])

  const s = stats.data
  // Newest-first → chronological for the chart.
  const timeline = (recent.data ?? [])
    .slice()
    .reverse()
    .map((m) => ({
      time: m.timestamp.slice(11, 19), // HH:MM:SS
      latency: m.latency,
    }))

  return (
    <div className="mx-auto flex max-w-6xl flex-col">
      <PageHeader
        title="Dashboard"
        description="Overall connection status and recent diagnostic activity."
        actions={
          <Button
            variant={monitoring ? 'outline' : 'default'}
            size="sm"
            disabled={startMut.isPending || stopMut.isPending}
            onClick={() => (monitoring ? stopMut.mutate() : startMut.mutate())}
          >
            {monitoring ? (
              <Pause className="h-4 w-4" />
            ) : (
              <Play className="h-4 w-4" />
            )}
            {monitoring ? 'Stop monitoring' : 'Start monitoring'}
          </Button>
        }
      />

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatCard label="Avg RTT" value={fmtMs(s?.avgRtt)} hint="last 5 min" />
        <StatCard label="P95 RTT" value={fmtMs(s?.p95Rtt)} hint="last 5 min" />
        <StatCard label="Jitter" value={fmtMs(s?.jitter)} hint="mean abs. delta" />
        <StatCard
          label="Packet loss"
          value={fmtPct(s?.packetLoss)}
          hint={`${s?.sampleCount ?? 0} samples`}
        />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>RTT timeline</CardTitle>
          </CardHeader>
          <CardContent>
            {timeline.length === 0 ? (
              <div className="flex h-48 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">
                Waiting for measurements…
              </div>
            ) : (
              <ResponsiveContainer width="100%" height={192}>
                <LineChart data={timeline} margin={{ top: 8, right: 16, bottom: 0, left: -8 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis
                    dataKey="time"
                    stroke="var(--muted-foreground)"
                    fontSize={11}
                    tickMargin={6}
                    minTickGap={24}
                  />
                  <YAxis
                    stroke="var(--muted-foreground)"
                    fontSize={11}
                    width={48}
                    unit=" ms"
                  />
                  <Tooltip
                    contentStyle={{
                      background: 'var(--card)',
                      border: '1px solid var(--border)',
                      borderRadius: 6,
                      fontSize: 12,
                    }}
                    formatter={(v: number) => [`${v.toFixed(1)} ms`, 'RTT']}
                  />
                  <Line
                    type="monotone"
                    dataKey="latency"
                    stroke="var(--primary)"
                    strokeWidth={2}
                    dot={false}
                    connectNulls={false}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Recent anomalies</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
              No active anomalies
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
