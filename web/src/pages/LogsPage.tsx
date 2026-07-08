import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDownToLine,
  Download,
  Eraser,
  Loader2,
  ScrollText,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { CopyButton } from '@/components/CopyButton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useEvents } from '@/hooks/use-events'
import { cn } from '@/lib/utils'
import { useT } from '@/i18n'
import type { LogService } from '@/lib/types'
import { api } from '@/lib/api'

const SERVICES: { value: LogService; label: string }[] = [
  { value: 'keen-manager', label: 'keen-manager' },
  { value: 'xray', label: 'xray' },
  { value: 'nfqws2', label: 'nfqws2' },
  { value: 'awg', label: 'awg' },
]

const LINE_OPTIONS = [100, 300, 500, 1000]
const MAX_BUFFER = 1000

/** Colorize a log line's severity token without altering its text. */
function severityClass(line: string): string {
  if (/\bERROR\b|\bERR\b/.test(line)) return 'text-destructive'
  if (/\bWARN(ING)?\b/.test(line)) return 'text-warning'
  if (/\bDEBUG\b/.test(line)) return 'text-muted-foreground'
  return 'text-foreground/85'
}

export function LogsPage() {
  const t = useT()
  const { connected, subscribeLogs } = useEvents()

  const [service, setService] = React.useState<LogService>('keen-manager')
  const [lines, setLines] = React.useState(300)
  const [autoScroll, setAutoScroll] = React.useState(true)
  const [liveLines, setLiveLines] = React.useState<string[]>([])
  const [cleared, setCleared] = React.useState(false)

  const scrollRef = React.useRef<HTMLDivElement | null>(null)

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: ['logs', service, lines],
    queryFn: () => api.logs(service, lines),
    refetchInterval: connected ? false : 10000,
  })

  // Reset live buffer whenever the service or line-count changes.
  React.useEffect(() => {
    setLiveLines([])
    setCleared(false)
  }, [service, lines])

  // Append live SSE frames for the selected service.
  React.useEffect(() => {
    const unsub = subscribeLogs((evt) => {
      if (evt.service !== service) return
      setLiveLines((prev) => {
        const next = [...prev, evt.line]
        return next.length > MAX_BUFFER
          ? next.slice(next.length - MAX_BUFFER)
          : next
      })
    })
    return unsub
  }, [subscribeLogs, service])

  const baseLines = cleared ? [] : (data?.lines ?? [])
  const allLines = React.useMemo(() => {
    const combined = [...baseLines, ...liveLines]
    return combined.length > MAX_BUFFER
      ? combined.slice(combined.length - MAX_BUFFER)
      : combined
  }, [baseLines, liveLines])

  // Auto-scroll to bottom on new content.
  React.useEffect(() => {
    if (!autoScroll) return
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [allLines, autoScroll])

  const clear = () => {
    setCleared(true)
    setLiveLines([])
  }

  const download = () => {
    const blob = new Blob([allLines.join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
    a.href = url
    a.download = `${service}-${stamp}.log`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('logs.title')}
        description={t('logs.desc')}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <div
              className="flex items-center gap-1.5 rounded-md border border-border/70 px-2.5 py-1.5"
              title={connected ? t('logs.liveTip') : t('logs.pollingTip')}
            >
              <span
                className={cn(
                  'h-1.5 w-1.5 rounded-full',
                  connected
                    ? 'bg-success animate-pulse-dot'
                    : 'bg-muted-foreground/40',
                )}
              />
              <span className="text-xs font-medium text-muted-foreground">
                {connected ? t('common.live') : t('common.polling')}
              </span>
            </div>

            <Select
              value={service}
              onValueChange={(v) => setService(v as LogService)}
            >
              <SelectTrigger className="h-8 w-[150px] font-mono text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SERVICES.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={String(lines)}
              onValueChange={(v) => setLines(Number(v))}
            >
              <SelectTrigger className="h-8 w-[110px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {LINE_OPTIONS.map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {t('logs.linesOption', { count: n })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
      />

      <Card className="overflow-hidden">
        {/* Terminal toolbar */}
        <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-2">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <ScrollText className="h-3.5 w-3.5" />
            <span className="font-mono">
              {service}
              {isFetching && !isLoading ? (
                <Loader2 className="ml-2 inline h-3 w-3 animate-spin" />
              ) : null}
            </span>
            <span className="text-muted-foreground/60">·</span>
            <span className="tabular-nums">
              {t('logs.linesCount', { count: allLines.length })}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant={autoScroll ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setAutoScroll((v) => !v)}
              className="h-7 gap-1.5 text-xs"
            >
              <ArrowDownToLine className="h-3.5 w-3.5" />
              {t('logs.autoScroll')}
            </Button>
            <CopyButton
              value={allLines.join('\n')}
              label={t('logs.copyVisible')}
            />
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={download}
              aria-label={t('logs.download')}
              className="text-muted-foreground"
            >
              <Download className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={clear}
              aria-label={t('logs.clearView')}
              className="text-muted-foreground"
            >
              <Eraser className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>

        {/* Terminal body */}
        <div
          ref={scrollRef}
          className="h-[calc(100dvh-19rem)] min-h-[320px] overflow-auto bg-background/60 p-4 font-mono text-xs leading-relaxed"
        >
          {isLoading ? (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              {t('logs.loading')}
            </div>
          ) : allLines.length === 0 ? (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              {cleared ? t('logs.clearedEmpty') : t('logs.noOutput')}
            </div>
          ) : (
            <div>
              {allLines.map((line, i) => (
                <div
                  key={i}
                  className={cn(
                    'flex gap-3 rounded px-1 py-0.5 transition-colors hover:bg-muted/40',
                    severityClass(line),
                  )}
                >
                  <span className="select-none text-muted-foreground/40 tabular-nums">
                    {String(i + 1).padStart(4, ' ')}
                  </span>
                  <span className="whitespace-pre-wrap break-all">{line}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </Card>

      {!connected ? (
        <p className="text-center text-xs text-muted-foreground">
          {t('logs.pollingNotice')}{' '}
          <button
            type="button"
            onClick={() => refetch()}
            className="font-medium text-primary hover:underline"
          >
            {t('logs.refreshNow')}
          </button>
        </p>
      ) : null}
    </div>
  )
}
