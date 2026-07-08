import * as React from 'react'
import { Check, Copy } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useT } from '@/i18n'
import { cn } from '@/lib/utils'

interface CopyButtonProps {
  value: string
  className?: string
  label?: string
}

/** Copy-to-clipboard icon button with a transient confirmation state. */
export function CopyButton({ value, className, label }: CopyButtonProps) {
  const t = useT()
  const [copied, setCopied] = React.useState(false)

  const onCopy = React.useCallback(async () => {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      /* clipboard may be unavailable; ignore */
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1400)
  }, [value])

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      onClick={onCopy}
      aria-label={label ?? t('common.copy')}
      className={cn('text-muted-foreground', className)}
    >
      {copied ? (
        <Check className="h-3.5 w-3.5 text-success" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
    </Button>
  )
}
