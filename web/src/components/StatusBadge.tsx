import { Badge } from '@/components/ui/badge'
import { StatusDot } from '@/components/StatusDot'
import { useT } from '@/i18n'
import type { ConnStatus } from '@/lib/types'

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
  const t = useT()
  return (
    <Badge variant={variant[status]} className={className}>
      <StatusDot status={status} className="h-2 w-2" />
      {t(`status.${status}`)}
    </Badge>
  )
}
