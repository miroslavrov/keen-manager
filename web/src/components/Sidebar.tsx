import { NavLink } from 'react-router-dom'
import { ShieldCheck } from 'lucide-react'

import { NAV_ITEMS } from '@/lib/nav'
import { cn } from '@/lib/utils'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

interface SidebarProps {
  className?: string
  onNavigate?: () => void
}

/** Fixed left navigation — icon + label, with brand header and version footer. */
export function Sidebar({ className, onNavigate }: SidebarProps) {
  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    staleTime: 60_000,
  })

  return (
    <aside
      className={cn(
        'flex h-full w-60 flex-col border-r border-sidebar-border bg-sidebar',
        className,
      )}
    >
      <div className="flex h-14 items-center gap-2.5 border-b border-sidebar-border px-5">
        <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/15 text-primary">
          <ShieldCheck className="h-4 w-4" />
        </div>
        <div className="leading-tight">
          <div className="text-sm font-semibold tracking-tight text-foreground">
            keen-manager
          </div>
          <div className="text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
            VPN / DPI control
          </div>
        </div>
      </div>

      <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {NAV_ITEMS.map((item) => {
          const Icon = item.icon
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={onNavigate}
              className={({ isActive }) =>
                cn(
                  'group flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  'text-sidebar-foreground hover:bg-sidebar-accent hover:text-foreground',
                  isActive &&
                    'bg-sidebar-accent text-foreground shadow-sm ring-1 ring-inset ring-border',
                )
              }
            >
              {({ isActive }) => (
                <>
                  <Icon
                    className={cn(
                      'h-4 w-4 shrink-0 transition-colors',
                      isActive
                        ? 'text-primary'
                        : 'text-muted-foreground group-hover:text-foreground',
                    )}
                  />
                  <span className="truncate">{item.label}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>

      <div className="border-t border-sidebar-border p-4">
        <div className="flex items-center justify-between text-[11px] text-muted-foreground">
          <span className="font-mono">
            v{health?.version ?? '—'}
          </span>
          <span className="font-mono uppercase">{health?.arch ?? ''}</span>
        </div>
      </div>
    </aside>
  )
}
