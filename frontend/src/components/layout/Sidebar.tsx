import { NavLink } from 'react-router-dom'
import {
  Activity,
  Stethoscope,
  Crosshair,
  History,
  Network,
  Settings,
  Gauge,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAppStore } from '@/stores/app-store'

interface NavItem {
  to: string
  label: string
  icon: React.ComponentType<{ className?: string }>
}

const NAV: NavItem[] = [
  { to: '/dashboard', label: 'Dashboard', icon: Activity },
  { to: '/diagnostics', label: 'Diagnostics', icon: Stethoscope },
  { to: '/targets', label: 'Targets', icon: Crosshair },
  { to: '/history', label: 'History', icon: History },
  { to: '/network', label: 'Network', icon: Network },
  { to: '/settings', label: 'Settings', icon: Settings },
]

/**
 * Fixed left navigation. Dark, minimalist, icon + label.
 * Active monitoring indicator reflects the global app store.
 */
export function Sidebar() {
  const monitoring = useAppStore((s) => s.monitoring)

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r bg-card/40">
      <div className="flex items-center gap-2 px-4 py-5">
        <Gauge className="h-5 w-5 text-primary" />
        <div className="flex flex-col">
          <span className="text-sm font-semibold leading-tight">NDE</span>
          <span className="text-[10px] text-muted-foreground">Network Diagnostics</span>
        </div>
      </div>

      <nav className="flex flex-1 flex-col gap-1 px-2">
        {NAV.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
              )
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="border-t px-4 py-3">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              'h-2 w-2 rounded-full',
              monitoring ? 'bg-emerald-500' : 'bg-muted-foreground/50',
            )}
          />
          <span className="text-[11px] text-muted-foreground">
            {monitoring ? 'Monitoring active' : 'Monitoring idle'}
          </span>
        </div>
      </div>
    </aside>
  )
}
