import { Link } from 'react-router-dom'
import { Compass } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useT } from '@/i18n'

export function NotFoundPage() {
  const t = useT()
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Compass className="h-6 w-6" />
      </div>
      <div className="space-y-1">
        <h1 className="text-lg font-semibold">{t('notfound.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('notfound.desc')}</p>
      </div>
      <Button asChild variant="outline">
        <Link to="/">{t('notfound.home')}</Link>
      </Button>
    </div>
  )
}
