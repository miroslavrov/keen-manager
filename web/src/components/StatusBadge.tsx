import { Badge } from '@/components/ui/badge'
import { StatusDot } from '@/components/StatusDot'
import type { ConnStatus } from '@/lib/types'

const label: Record<ConnStatus, string> = {
  up: 'Up',
  down: 'Down',
  degraded: 'Degraded',
  checking: 'Checking',
  disabled: 'Disabled',
}

const variant: Record<
  ConnStatus,
  'success' | 'destructive' | 'warning' | 'secondary' | 'muted'
> = {
  up: 'success',
  down: 'destructive',
  degraded: 'warning',
  checking: 'secondary',
  disabled: 'muted',
}

interface StatusBadgeProps {
  status: ConnStatus
  className?: string
}

/** Status pill: colored dot + text label, semantically colored. */
export function StatusBadge({ status, className }: StatusBadgeProps) {
  return (
    <Badge variant={variant[status]} className={className}>
      <StatusDot status={status} className="h-2 w-2" />
      {label[status]}
    </Badge>
  )
}
