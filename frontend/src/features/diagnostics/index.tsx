import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * Diagnostics (SPEC §5.2).
 * Phase 1: placeholder. Active problems, hypotheses, confidence, evidence and
 * recommendations are populated in Phase 5/6.
 */
export function DiagnosticsPage() {
  return (
    <div className="mx-auto flex max-w-5xl flex-col">
      <PageHeader
        title="Diagnostics"
        description="Active problems, probable cause and supporting evidence."
      />
      <Card>
        <CardHeader>
          <CardTitle>Active problems</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-64 items-center justify-center text-sm text-muted-foreground">
            No active diagnostic events
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
