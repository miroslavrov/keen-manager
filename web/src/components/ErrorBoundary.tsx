import * as React from 'react'
import { AlertTriangle, RotateCcw } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useT } from '@/i18n'
import { cn } from '@/lib/utils'

interface Props {
  children: React.ReactNode
  /**
   * 'page' renders a compact inline card (used per-route so a single page crash
   * never blanks the whole shell). 'app' renders the top-level catch-all.
   */
  variant?: 'page' | 'app'
  /**
   * When this value changes the boundary clears its error, so navigating to a
   * different route automatically recovers a crashed page.
   */
  resetKey?: string | number
}

interface State {
  error: Error | null
}

/**
 * ErrorBoundary is the resilience layer the UI was missing: without it, one
 * page throwing during render (e.g. reading `.length` of an absent array)
 * unmounts the entire React tree, so every tab goes blank until a full reload.
 * With a per-route boundary, a crash is contained to the content area and the
 * sidebar/top bar stay interactive.
 */
export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Surface for debugging; the daemon logs page is unaffected.
    // eslint-disable-next-line no-console
    console.error('[keen-manager] render error:', error, info.componentStack)
  }

  private reset = () => this.setState({ error: null })

  render() {
    if (this.state.error) {
      return (
        <ErrorFallback
          error={this.state.error}
          onReset={this.reset}
          variant={this.props.variant ?? 'page'}
        />
      )
    }
    return this.props.children
  }
}

function ErrorFallback({
  error,
  onReset,
  variant,
}: {
  error: Error
  onReset: () => void
  variant: 'page' | 'app'
}) {
  const t = useT()
  const [showDetails, setShowDetails] = React.useState(false)

  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-4 text-center',
        variant === 'app' ? 'min-h-dvh p-6' : 'min-h-[50vh] p-6',
      )}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10 text-destructive">
        <AlertTriangle className="h-6 w-6" />
      </div>
      <div className="max-w-md space-y-1.5">
        <h2 className="text-lg font-semibold text-foreground">{t('err.title')}</h2>
        <p className="text-sm text-muted-foreground">
          {variant === 'app' ? t('err.appDesc') : t('err.desc')}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-center gap-2">
        <Button onClick={onReset} className="gap-1.5">
          <RotateCcw className="h-3.5 w-3.5" />
          {t('err.reload')}
        </Button>
        <Button
          variant="outline"
          onClick={() => window.location.reload()}
          className="gap-1.5"
        >
          {t('err.reloadPage')}
        </Button>
      </div>
      <button
        type="button"
        onClick={() => setShowDetails((v) => !v)}
        className="text-xs font-medium text-muted-foreground underline-offset-4 hover:underline"
      >
        {t('err.details')}
      </button>
      {showDetails ? (
        <pre className="max-h-48 w-full max-w-xl overflow-auto rounded-md border border-border bg-muted/40 p-3 text-left text-[11px] leading-relaxed text-muted-foreground">
          {error.message}
          {error.stack ? `\n\n${error.stack}` : ''}
        </pre>
      ) : null}
    </div>
  )
}
