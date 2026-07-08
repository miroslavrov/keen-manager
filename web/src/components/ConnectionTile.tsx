import { MapPin, Radio } from 'lucide-react'

import { StatusDot } from '@/components/StatusDot'
import { TypeBadge } from '@/components/TypeBadge'
import { LatencyBadge } from '@/components/LatencyBadge'
import { useT } from '@/i18n'
import { cn, timeAgo } from '@/lib/utils'
import type { Conn } from '@/lib/types'

interface ConnectionTileProps {
  conn: Conn
  onClick?: () => void
}

/** Compact connection health tile used in the dashboard grid. */
export function ConnectionTile({ conn, onClick }: ConnectionTileProps) {
  const t = useT()
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'group flex w-full flex-col gap-3 rounded-lg border bg-card p-4 text-left transition-all hover:border-border hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        conn.active
          ? 'border-primary/50 ring-1 ring-inset ring-primary/20'
          : 'border-border/70',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <StatusDot status={conn.status} />
          <span className="truncate text-sm font-medium text-foreground">
            {conn.name}
          </span>
        </div>
        <TypeBadge type={conn.type} />
      </div>

      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <MapPin className="h-3.5 w-3.5 shrink-0" />
        <span className="truncate">{conn.location ?? t('common.unknownLocation')}</span>
      </div>

      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5">
          {conn.active ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
              <Radio className="h-3 w-3" />
              {t('common.active')}
            </span>
          ) : (
            <span className="text-[11px] text-muted-foreground">
              {conn.enabled ? timeAgo(conn.last_check) : t('common.disabled')}
            </span>
          )}
        </div>
        <LatencyBadge ms={conn.latency_ms} />
      </div>
    </button>
  )
}
