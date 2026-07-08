import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDown,
  ChevronDown,
  ChevronUp,
  GitBranch,
  History,
  Loader2,
  Plus,
  Radio,
  Save,
  Shield,
  Trash2,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { TypeBadge } from '@/components/TypeBadge'
import { StatusDot } from '@/components/StatusDot'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn, timeAgo } from '@/lib/utils'
import type { Conn, Failover } from '@/lib/types'

const DIRECT = 'direct'

export function FailoverPage() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const { data: failover, isLoading } = useQuery({
    queryKey: ['failover'],
    queryFn: api.failover,
    refetchInterval: 8000,
  })
  const { data: connections } = useQuery({
    queryKey: ['connections'],
    queryFn: api.connections,
  })

  const [draft, setDraft] = React.useState<Failover | null>(null)
  const [dirty, setDirty] = React.useState(false)

  React.useEffect(() => {
    if (failover && !dirty) {
      setDraft(failover)
    }
  }, [failover, dirty])

  const saveMutation = useMutation({
    mutationFn: (body: Failover) => api.saveFailover(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['failover'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      setDirty(false)
      toast({ variant: 'success', title: 'Failover configuration saved' })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not save failover config' }),
  })

  const update = (patch: Partial<Failover>) => {
    setDraft((prev) => (prev ? { ...prev, ...patch } : prev))
    setDirty(true)
  }

  const connById = React.useMemo(() => {
    const map = new Map<string, Conn>()
    ;(connections ?? []).forEach((c) => map.set(c.id, c))
    return map
  }, [connections])

  const nameFor = (id: string) =>
    id === DIRECT ? 'Direct WAN' : (connById.get(id)?.name ?? id)

  const availableToAdd = React.useMemo(() => {
    const inChain = new Set(draft?.chain ?? [])
    return (connections ?? []).filter((c) => !inChain.has(c.id))
  }, [connections, draft?.chain])

  if (isLoading || !draft) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Failover"
          description="Fallback chain & health-driven route switching."
        />
        <div className="grid gap-4 lg:grid-cols-[1fr_360px]">
          <Skeleton className="h-96" />
          <Skeleton className="h-96" />
        </div>
      </div>
    )
  }

  const moveNode = (index: number, dir: -1 | 1) => {
    const next = [...draft.chain]
    const target = index + dir
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]
    update({ chain: next })
  }

  const removeNode = (index: number) => {
    const next = draft.chain.filter((_, i) => i !== index)
    update({ chain: next })
  }

  const addNode = (id: string) => {
    // Keep "direct" pinned to the end if present.
    const hasDirect = draft.chain.includes(DIRECT)
    const withoutDirect = draft.chain.filter((c) => c !== DIRECT)
    const next = hasDirect
      ? [...withoutDirect, id, DIRECT]
      : [...draft.chain, id]
    update({ chain: next })
  }

  const addDirect = () => {
    if (draft.chain.includes(DIRECT)) return
    update({ chain: [...draft.chain, DIRECT] })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Failover"
        description="Fallback chain & health-driven route switching."
        actions={
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2">
              <Switch
                checked={draft.enabled}
                onCheckedChange={(v) => update({ enabled: v })}
                aria-label="Enable failover"
              />
              <span className="text-sm text-muted-foreground">
                {draft.enabled ? 'Enabled' : 'Disabled'}
              </span>
            </div>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate(draft)}
              disabled={!dirty || saveMutation.isPending}
              className="gap-1.5"
            >
              {saveMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              Save
            </Button>
          </div>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[1fr_360px]">
        {/* Chain editor */}
        <Card className={cn(!draft.enabled && 'opacity-70')}>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div className="space-y-1">
              <CardTitle>Fallback chain</CardTitle>
              <p className="text-xs text-muted-foreground">
                Traffic prefers the top node; it descends on repeated failure.
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Select
                value=""
                onValueChange={(v) => {
                  if (v === DIRECT) addDirect()
                  else addNode(v)
                }}
              >
                <SelectTrigger className="h-8 w-[150px] text-xs">
                  <span className="flex items-center gap-1.5 text-muted-foreground">
                    <Plus className="h-3.5 w-3.5" />
                    Add node
                  </span>
                </SelectTrigger>
                <SelectContent>
                  {availableToAdd.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                  {!draft.chain.includes(DIRECT) ? (
                    <SelectItem value={DIRECT}>Direct WAN</SelectItem>
                  ) : null}
                  {availableToAdd.length === 0 &&
                  draft.chain.includes(DIRECT) ? (
                    <SelectItem value="__none" disabled>
                      All connections added
                    </SelectItem>
                  ) : null}
                </SelectContent>
              </Select>
            </div>
          </CardHeader>
          <CardContent>
            {draft.chain.length === 0 ? (
              <EmptyState
                icon={GitBranch}
                title="Empty chain"
                description="Add connections to build a fallback order. The last node is typically Direct WAN or the kill-switch."
              />
            ) : (
              <ol className="space-y-0">
                {draft.chain.map((id, index) => {
                  const isActive = index === draft.current_index
                  const isDirect = id === DIRECT
                  const conn = connById.get(id)
                  const isLast = index === draft.chain.length - 1
                  return (
                    <li key={`${id}-${index}`}>
                      <div
                        className={cn(
                          'flex items-center gap-3 rounded-lg border p-3 transition-colors',
                          isActive
                            ? 'border-primary/50 bg-primary/5 ring-1 ring-inset ring-primary/20'
                            : 'border-border/70 bg-card',
                        )}
                      >
                        <div
                          className={cn(
                            'flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-xs font-semibold tabular-nums',
                            isActive
                              ? 'bg-primary text-primary-foreground'
                              : 'bg-muted text-muted-foreground',
                          )}
                        >
                          {index + 1}
                        </div>

                        <div className="flex min-w-0 flex-1 items-center gap-2">
                          {isDirect ? (
                            <>
                              <Shield className="h-4 w-4 text-muted-foreground" />
                              <span className="text-sm font-medium text-foreground">
                                Direct WAN
                              </span>
                              <span className="text-xs text-muted-foreground">
                                (no tunnel)
                              </span>
                            </>
                          ) : (
                            <>
                              <StatusDot status={conn?.status ?? 'disabled'} />
                              <span className="truncate text-sm font-medium text-foreground">
                                {conn?.name ?? id}
                              </span>
                              {conn ? <TypeBadge type={conn.type} /> : null}
                            </>
                          )}
                          {isActive ? (
                            <span className="ml-1 inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
                              <Radio className="h-3 w-3" />
                              Live
                            </span>
                          ) : null}
                        </div>

                        <div className="flex items-center gap-0.5">
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            disabled={index === 0}
                            onClick={() => moveNode(index, -1)}
                            aria-label="Move up"
                          >
                            <ChevronUp className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            disabled={isLast}
                            onClick={() => moveNode(index, 1)}
                            aria-label="Move down"
                          >
                            <ChevronDown className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => removeNode(index)}
                            aria-label="Remove node"
                            className="text-muted-foreground hover:text-destructive"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>

                      {!isLast ? (
                        <div className="flex h-6 items-center pl-[26px]">
                          <ArrowDown className="h-4 w-4 text-muted-foreground/60" />
                        </div>
                      ) : null}
                    </li>
                  )
                })}
              </ol>
            )}
          </CardContent>
        </Card>

        {/* Settings */}
        <Card className={cn('h-fit', !draft.enabled && 'opacity-70')}>
          <CardHeader>
            <CardTitle>Health probing</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="probe-target">Probe target</Label>
              <Input
                id="probe-target"
                value={draft.probe_target}
                onChange={(e) => update({ probe_target: e.target.value })}
                className="font-mono text-xs"
                spellCheck={false}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="check-interval">Check interval (s)</Label>
                <Input
                  id="check-interval"
                  type="number"
                  min={1}
                  value={draft.check_interval_s}
                  onChange={(e) =>
                    update({ check_interval_s: Number(e.target.value) })
                  }
                  className="tabular-nums"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="fail-threshold">Failure threshold</Label>
                <Input
                  id="fail-threshold"
                  type="number"
                  min={1}
                  value={draft.failure_threshold}
                  onChange={(e) =>
                    update({ failure_threshold: Number(e.target.value) })
                  }
                  className="tabular-nums"
                />
              </div>
            </div>

            <Separator />

            <div className="flex items-center justify-between gap-3">
              <div className="space-y-0.5">
                <Label>Auto-return</Label>
                <p className="text-xs text-muted-foreground">
                  Switch back to a higher-priority node once it recovers.
                </p>
              </div>
              <Switch
                checked={draft.auto_return}
                onCheckedChange={(v) => update({ auto_return: v })}
                aria-label="Auto-return to primary"
              />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* History timeline */}
      <Card>
        <CardHeader className="flex-row items-center gap-2 space-y-0">
          <History className="h-4 w-4 text-muted-foreground" />
          <CardTitle>Switch history</CardTitle>
        </CardHeader>
        <CardContent>
          {draft.history.length === 0 ? (
            <EmptyState
              icon={History}
              title="No failover events yet"
              description="Route switches will appear here as they happen."
              className="py-8"
            />
          ) : (
            <ol className="space-y-0">
              {draft.history.map((event, i) => {
                const last = i === draft.history.length - 1
                return (
                  <li key={i} className="relative flex gap-3 pb-5 last:pb-0">
                    {!last ? (
                      <span className="absolute left-[5px] top-3 h-full w-px bg-border" />
                    ) : null}
                    <span className="relative mt-1 h-2.5 w-2.5 shrink-0 rounded-full border-2 border-background bg-muted-foreground/50" />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-sm">
                        <span className="font-medium text-foreground">
                          {nameFor(event.from)}
                        </span>
                        <ArrowDown className="h-3 w-3 -rotate-90 text-muted-foreground" />
                        <span className="font-medium text-foreground">
                          {nameFor(event.to)}
                        </span>
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {event.reason}
                      </p>
                    </div>
                    <span className="shrink-0 whitespace-nowrap text-[11px] tabular-nums text-muted-foreground">
                      {timeAgo(event.time)}
                    </span>
                  </li>
                )
              })}
            </ol>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
