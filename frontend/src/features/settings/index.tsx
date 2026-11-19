import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

/**
 * Settings (SPEC §5.6).
 * Phase 1: placeholder. Intervals, timeouts, anomaly thresholds, retention,
 * notifications, autostart and display preferences land in Phase 6.
 */
export function SettingsPage() {
  return (
    <div className="mx-auto flex max-w-3xl flex-col">
      <PageHeader
        title="Settings"
        description="Monitoring intervals, thresholds, retention and notifications."
        actions={<Button size="sm">Save changes</Button>}
      />
      <Card>
        <CardHeader>
          <CardTitle>General</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
            Settings form (Phase 6)
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
