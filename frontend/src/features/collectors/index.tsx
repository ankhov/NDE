import { Radar, Activity, Server, Globe, Network, Route } from 'lucide-react'
import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useCollectorStatus } from '@/hooks/use-nde'
import { cn } from '@/lib/utils'
import type { MeasurementType } from '@/types'

interface CollectorMeta {
  label: string
  description: string
  icon: React.ComponentType<{ className?: string }>
}

/** Static description of each measurement type, shown alongside live status. */
const META: Record<MeasurementType, CollectorMeta> = {
  icmp: {
    label: 'ICMP Echo',
    description: 'Round-trip latency to a host via the OS ping binary (IcmpSendEcho on Windows, cap_net_raw/UDP-ICMP on Linux and macOS).',
    icon: Activity,
  },
  tcp: {
    label: 'TCP Connect',
    description: 'Time to complete a TCP three-way handshake to (host, port). The primary always-available latency signal.',
    icon: Server,
  },
  dns: {
    label: 'DNS Exchange',
    description: 'Round-trip of a DNS query to a specific resolver, with the response code recorded.',
    icon: Globe,
  },
  http: {
    label: 'HTTP GET',
    description: 'Round-trip of an HTTP GET (no redirect following) with the response status code recorded.',
    icon: Network,
  },
  traceroute: {
    label: 'Traceroute',
    description: 'IP path discovery via the OS traceroute binary. Each hop is recorded; the destination RTT is the measurement latency.',
    icon: Route,
  },
}

/**
 * Collectors page. Shows which measurement types have a live collector
 * registered, with a short description of what each one measures. Availability
 * is read from CollectorService.Status, which reads the same registry the
 * scheduler uses.
 */
export function CollectorsPage() {
  const { data, isLoading } = useCollectorStatus()
  const rows = data ?? []

  return (
    <div className="mx-auto flex max-w-4xl flex-col">
      <PageHeader
        title="Collectors"
        description="Measurement types and the collectors available to run them."
      />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {Object.entries(META).map(([key, meta]) => {
          const type = key as MeasurementType
          const status = rows.find((r) => r.type === type)
          const ready = status?.ready ?? false
          const Icon = meta.icon
          return (
            <Card key={type}>
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between">
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <Icon className="h-4 w-4 text-muted-foreground" />
                    {meta.label}
                  </CardTitle>
                  <span
                    className={cn(
                      'rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                      ready
                        ? 'bg-emerald-500/15 text-emerald-400'
                        : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {ready ? 'Ready' : 'Unavailable'}
                  </span>
                </div>
              </CardHeader>
              <CardContent>
                <p className="text-xs leading-relaxed text-muted-foreground">
                  {meta.description}
                </p>
                {ready && status?.collectorName && (
                  <p className="mt-2 text-[11px] text-muted-foreground/80">
                    collector: <code className="font-mono">{status.collectorName}</code>
                  </p>
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>

      <div className="mt-6 flex items-center gap-2 text-xs text-muted-foreground">
        <Radar className="h-3.5 w-3.5" />
        {isLoading
          ? 'Reading collector registry…'
          : `${rows.filter((r) => r.ready).length} of ${rows.length || 5} collectors ready`}
      </div>
    </div>
  )
}
