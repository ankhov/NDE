import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * History (SPEC §5.5).
 * Phase 1: placeholder. RTT/Jitter/PacketLoss history, DNS stats, anomalies
 * and diagnostic events land in Phase 6; CSV/JSON export in Phase 7.
 */
export function HistoryPage() {
  return (
    <div className="mx-auto flex max-w-5xl flex-col">
      <PageHeader
        title="History"
        description="Time-series history, anomalies and diagnostic events."
      />
      <Card>
        <CardHeader>
          <CardTitle>History</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-64 items-center justify-center text-sm text-muted-foreground">
            No history yet — start monitoring to collect measurements
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
