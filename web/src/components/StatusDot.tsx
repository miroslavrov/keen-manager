import { cn } from '@/lib/utils'
import type { ConnStatus } from '@/lib/types'

const dotColor: Record<ConnStatus, string> = {
  up: 'bg-success',
  down: 'bg-destructive',
  degraded: 'bg-warning',
  checking: 'bg-sky-400',
  disabled: 'bg-muted-foreground/50',
}

interface StatusDotProps {
  status: ConnStatus
  className?: string
  pulse?: boolean
}

/** Small colored status indicator with an optional pulse for live states. */
export function StatusDot({ status, className, pulse }: StatusDotProps) {
  const animate =
    pulse ?? (status === 'up' || status === 'checking')
  return (
    <span className={cn('relative inline-flex h-2.5 w-2.5', className)}>
      {animate && status !== 'disabled' ? (
        <span
          className={cn(
            'absolute inline-flex h-full w-full rounded-full opacity-60 animate-pulse-dot',
            dotColor[status],
          )}
        />
      ) : null}
      <span
        className={cn(
          'relative inline-flex h-2.5 w-2.5 rounded-full',
          dotColor[status],
        )}
      />
    </span>
  )
}
