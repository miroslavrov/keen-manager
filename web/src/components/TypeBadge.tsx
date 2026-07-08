import { Shield, Zap } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { ConnType } from '@/lib/types'

interface TypeBadgeProps {
  type: ConnType
  className?: string
}

/** Connection transport badge — AWG (AmneziaWG) vs Xray. */
export function TypeBadge({ type, className }: TypeBadgeProps) {
  if (type === 'awg') {
    return (
      <Badge
        variant="outline"
        className={cn('border-emerald-500/30 text-emerald-500', className)}
      >
        <Shield className="h-3 w-3" />
        AWG
      </Badge>
    )
  }
  return (
    <Badge
      variant="outline"
      className={cn('border-sky-500/30 text-sky-400', className)}
    >
      <Zap className="h-3 w-3" />
      Xray
    </Badge>
  )
}
