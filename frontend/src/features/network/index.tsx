import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * Network Analysis (SPEC §5.4).
 * Phase 1: placeholder. Interfaces, configuration, DNS, traceroute and route
 * changes land in Phase 2/7.
 */
export function NetworkPage() {
  return (
    <div className="mx-auto flex max-w-5xl flex-col">
      <PageHeader
        title="Network Analysis"
        description="Interfaces, current configuration, DNS and traceroute."
      />
      <Card>
        <CardHeader>
          <CardTitle>Network configuration</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-64 items-center justify-center text-sm text-muted-foreground">
            Network discovery results appear here (Phase 2)
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
