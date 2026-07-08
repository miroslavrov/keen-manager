import { Loader2, WifiOff } from 'lucide-react'

import { StatusDot } from '@/components/StatusDot'
import { TypeBadge } from '@/components/TypeBadge'
import { LatencyBadge } from '@/components/LatencyBadge'
import { cn } from '@/lib/utils'
import type { AppState } from '@/lib/types'

interface StatusPillProps {
  state?: AppState
  loading?: boolean
  className?: string
}

/**
 * Global connection + health indicator shown in the top bar. Reflects the
 * active connection's transport, name, location and live latency.
 */
export function StatusPill({ state, loading, className }: StatusPillProps) {
  if (loading) {
    return (
      <div
        className={cn(
          'flex h-9 items-center gap-2 rounded-full border border-border bg-card px-3 text-xs text-muted-foreground',
          className,
        )}
      >
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        <span>Loading status…</span>
      </div>
    )
  }

  const active = state?.connections.find(
    (c) => c.id === state.active_connection_id,
  )

  if (!active) {
    return (
      <div
        className={cn(
          'flex h-9 items-center gap-2 rounded-full border border-destructive/40 bg-destructive/10 px-3 text-xs font-medium text-destructive',
          className,
        )}
      >
        <WifiOff className="h-3.5 w-3.5" />
        <span>No active connection</span>
      </div>
    )
  }

  return (
    <div
      className={cn(
        'flex h-9 items-center gap-2.5 rounded-full border border-border bg-card px-3 shadow-sm',
        className,
      )}
    >
      <StatusDot status={active.status} />
      <span className="max-w-[10rem] truncate text-xs font-medium text-foreground">
        {active.name}
      </span>
      <TypeBadge type={active.type} className="hidden sm:inline-flex" />
      {active.location ? (
        <span className="hidden text-xs text-muted-foreground md:inline">
          {active.location}
        </span>
      ) : null}
      <span className="mx-0.5 hidden h-4 w-px bg-border sm:block" />
      <LatencyBadge ms={active.latency_ms} className="hidden sm:inline" />
    </div>
  )
}
