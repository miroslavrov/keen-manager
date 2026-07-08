import * as React from 'react'

import { cn } from '@/lib/utils'

interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number
  indicatorClassName?: string
}

/**
 * Lightweight progress bar (no radix dependency) used for data-usage meters.
 * value is a percentage 0–100.
 */
const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  ({ className, value = 0, indicatorClassName, ...props }, ref) => {
    const clamped = Math.min(100, Math.max(0, value))
    return (
      <div
        ref={ref}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(clamped)}
        className={cn(
          'relative h-2 w-full overflow-hidden rounded-full bg-muted',
          className,
        )}
        {...props}
      >
        <div
          className={cn(
            'h-full rounded-full bg-primary transition-all duration-500',
            indicatorClassName,
          )}
          style={{ width: `${clamped}%` }}
        />
      </div>
    )
  },
)
Progress.displayName = 'Progress'

export { Progress }
