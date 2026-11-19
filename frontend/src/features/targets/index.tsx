import { useState } from 'react'
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { PageHeader } from '@/components/layout/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  useAddTarget,
  useDeleteTarget,
  useTargets,
  useToggleTargetEnabled,
} from '@/hooks/use-nde'
import type { MeasurementType, Target, TargetGroup, TargetInput } from '@/types'

const PROTOCOLS: MeasurementType[] = ['tcp', 'dns', 'http']
const GROUPS: TargetGroup[] = ['local', 'external', 'dns', 'custom']

const emptyInput: TargetInput = {
  name: '',
  host: '',
  protocol: 'tcp',
  port: 443,
  checkInterval: 60000,
  timeout: 3000,
  enabled: true,
  warningThreshold: 150,
  criticalThreshold: 400,
  group: 'external',
}

function fmtInterval(ms: number): string {
  if (ms >= 60000) return `${Math.round(ms / 60000)}m`
  return `${Math.round(ms / 1000)}s`
}

/**
 * Targets (SPEC §5.3).
 * Phase 2: full CRUD bound to TargetService. Adding/editing a target updates
 * the running scheduler live (no restart).
 */
export function TargetsPage() {
  const { data: targets, isLoading } = useTargets()
  const addMut = useAddTarget()
  const delMut = useDeleteTarget()
  const toggleMut = useToggleTargetEnabled()
  const [showForm, setShowForm] = useState(false)
  const [input, setInput] = useState<TargetInput>(emptyInput)

  const submit = () => {
    if (!input.name.trim() || !input.host.trim()) return
    addMut.mutate(input, {
      onSuccess: () => {
        setInput(emptyInput)
        setShowForm(false)
      },
    })
  }

  const field =
    'h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring'
  const label = 'text-xs font-medium text-muted-foreground'

  return (
    <div className="mx-auto flex max-w-5xl flex-col">
      <PageHeader
        title="Targets"
        description="Configured monitoring targets and their current state."
        actions={
          <Button size="sm" onClick={() => setShowForm((v) => !v)}>
            <Plus className="h-4 w-4" />
            Add target
          </Button>
        }
      />

      {showForm && (
        <Card className="mb-4">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">New target</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
              <div className="flex flex-col gap-1">
                <span className={label}>Name</span>
                <input className={field} value={input.name}
                  onChange={(e) => setInput({ ...input, name: e.target.value })}
                  placeholder="My server" />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Host</span>
                <input className={field} value={input.host}
                  onChange={(e) => setInput({ ...input, host: e.target.value })}
                  placeholder="1.1.1.1 or https://example.com" />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Protocol</span>
                <select className={field} value={input.protocol}
                  onChange={(e) => setInput({ ...input, protocol: e.target.value as MeasurementType })}>
                  {PROTOCOLS.map((p) => (
                    <option key={p} value={p}>{p}</option>
                  ))}
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Port</span>
                <input className={field} type="number" value={input.port}
                  onChange={(e) => setInput({ ...input, port: Number(e.target.value) })} />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Check interval (ms)</span>
                <input className={field} type="number" value={input.checkInterval}
                  onChange={(e) => setInput({ ...input, checkInterval: Number(e.target.value) })} />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Timeout (ms)</span>
                <input className={field} type="number" value={input.timeout}
                  onChange={(e) => setInput({ ...input, timeout: Number(e.target.value) })} />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Warning (ms)</span>
                <input className={field} type="number" value={input.warningThreshold}
                  onChange={(e) => setInput({ ...input, warningThreshold: Number(e.target.value) })} />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Critical (ms)</span>
                <input className={field} type="number" value={input.criticalThreshold}
                  onChange={(e) => setInput({ ...input, criticalThreshold: Number(e.target.value) })} />
              </div>
              <div className="flex flex-col gap-1">
                <span className={label}>Group</span>
                <select className={field} value={input.group}
                  onChange={(e) => setInput({ ...input, group: e.target.value as TargetGroup })}>
                  {GROUPS.map((g) => (
                    <option key={g} value={g}>{g}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="mt-3 flex items-center gap-3">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={input.enabled}
                  onChange={(e) => setInput({ ...input, enabled: e.target.checked })} />
                Enabled
              </label>
              <div className="flex-1" />
              <Button variant="outline" size="sm" onClick={() => setShowForm(false)}>Cancel</Button>
              <Button size="sm" onClick={submit} disabled={addMut.isPending}>
                {addMut.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
                Save target
              </Button>
            </div>
            {addMut.isError && (
              <div className="mt-2 text-xs text-destructive">
                Error: {String((addMut.error as Error)?.message ?? addMut.error)}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Targets</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="flex h-24 items-center justify-center text-sm text-muted-foreground">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading…
            </div>
          ) : !targets || targets.length === 0 ? (
            <div className="flex h-24 items-center justify-center text-sm text-muted-foreground">
              No targets configured yet
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-muted-foreground">
                    <th className="py-2 pr-3 font-medium">Name</th>
                    <th className="py-2 pr-3 font-medium">Host</th>
                    <th className="py-2 pr-3 font-medium">Proto</th>
                    <th className="py-2 pr-3 font-medium">Interval</th>
                    <th className="py-2 pr-3 font-medium">Status</th>
                    <th className="py-2 pr-3 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {targets.map((t: Target) => (
                    <tr key={t.id} className="border-b last:border-0">
                      <td className="py-2 pr-3 font-medium">{t.name}</td>
                      <td className="py-2 pr-3 text-muted-foreground">{t.host}</td>
                      <td className="py-2 pr-3 uppercase text-muted-foreground">{t.protocol}</td>
                      <td className="py-2 pr-3 tabular-nums text-muted-foreground">
                        {fmtInterval(t.checkInterval)}
                      </td>
                      <td className="py-2 pr-3">
                        <span
                          className={cn(
                            'inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium',
                            t.enabled
                              ? 'bg-emerald-500/15 text-emerald-400'
                              : 'bg-muted text-muted-foreground',
                          )}
                        >
                          {t.enabled ? 'enabled' : 'disabled'}
                        </span>
                      </td>
                      <td className="py-2 pr-3 text-right">
                        <div className="inline-flex items-center gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={toggleMut.isPending}
                            onClick={() => toggleMut.mutate({ id: t.id, enabled: !t.enabled })}
                          >
                            {t.enabled ? 'Disable' : 'Enable'}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={delMut.isPending}
                            onClick={() => delMut.mutate(t.id)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
