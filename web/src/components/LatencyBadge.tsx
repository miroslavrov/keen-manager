import { cn } from '@/lib/utils'
import { latencyLevel } from '@/lib/utils'

const levelColor = {
  good: 'text-success',
  ok: 'text-warning',
  bad: 'text-destructive',
  unknown: 'text-muted-foreground',
} as const

interface LatencyBadgeProps {
  ms?: number
  className?: string
  showUnit?: boolean
}

/**
 * Monospace latency readout with threshold coloring:
 *  <80ms green, <200ms amber, otherwise red. Missing -> dimmed dash.
 */
export function LatencyBadge({
  ms,
  className,
  showUnit = true,
}: LatencyBadgeProps) {
  const level = latencyLevel(ms)
  if (level === 'unknown') {
    return (
      <span
        className={cn('font-mono text-xs text-muted-foreground', className)}
      >
        —
      </span>
    )
  }
  return (
    <span
      className={cn(
        'font-mono text-xs font-medium tabular-nums',
        levelColor[level],
        className,
      )}
    >
      {Math.round(ms as number)}
      {showUnit ? <span className="ml-0.5 opacity-70">ms</span> : null}
    </span>
  )
}
