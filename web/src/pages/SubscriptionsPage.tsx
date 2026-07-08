import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  Gauge,
  Globe,
  Loader2,
  MapPin,
  Plus,
  RefreshCw,
  Server as ServerIcon,
  Trash2,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { StatusBadge } from '@/components/StatusBadge'
import { StatusDot } from '@/components/StatusDot'
import { LatencyBadge } from '@/components/LatencyBadge'
import { CopyButton } from '@/components/CopyButton'
import { ConfirmDialog, useConfirm } from '@/components/ConfirmDialog'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn, formatBytes, formatDate, pct, timeAgo } from '@/lib/utils'
import type { Server, Sub } from '@/lib/types'

export function SubscriptionsPage() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const [addOpen, setAddOpen] = React.useState(false)
  const [expanded, setExpanded] = React.useState<string | null>(null)
  const deleteConfirm = useConfirm<Sub>()

  const { data: subs, isLoading } = useQuery({
    queryKey: ['subscriptions'],
    queryFn: api.subscriptions,
  })

  const refreshMutation = useMutation({
    mutationFn: (id: string) => api.refreshSubscription(id),
    onSuccess: (_d, id) => {
      queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['subscription-servers', id] })
      queryClient.invalidateQueries({ queryKey: ['connections'] })
      toast({ variant: 'success', title: 'Subscription refreshed' })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not refresh subscription' }),
  })

  const selectBestMutation = useMutation({
    mutationFn: (id: string) => api.selectBest(id),
    onSuccess: (res, id) => {
      queryClient.invalidateQueries({ queryKey: ['subscription-servers', id] })
      queryClient.invalidateQueries({ queryKey: ['connections'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      const servers = queryClient.getQueryData<Server[]>([
        'subscription-servers',
        id,
      ])
      const picked = servers?.find((s) => s.id === res.selected_id)
      toast({
        variant: 'success',
        title: 'Selected best server',
        description: picked
          ? `${picked.name} (${Math.round(picked.latency_ms ?? 0)} ms)`
          : undefined,
      })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not select best server' }),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteSubscription(id),
    onSuccess: (_d, id) => {
      const name = subs?.find((s) => s.id === id)?.name
      queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['connections'] })
      deleteConfirm.close()
      toast({
        variant: 'success',
        title: 'Subscription removed',
        description: name ? `“${name}” was deleted.` : undefined,
      })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not delete subscription' }),
  })

  const list = subs ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title="Subscriptions"
        description="Xray subscription feeds — server lists, data usage, and auto-selection."
        actions={
          <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1.5">
            <Plus className="h-4 w-4" />
            Add subscription
          </Button>
        }
      />

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton key={i} className="h-40" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <EmptyState
          icon={Globe}
          title="No subscriptions"
          description="Add a subscription URL (e.g. https://host/s/<token>) to import a fleet of Xray servers."
          action={
            <Button size="sm" onClick={() => setAddOpen(true)}>
              Add subscription
            </Button>
          }
        />
      ) : (
        <div className="space-y-4">
          {list.map((sub) => (
            <SubscriptionCard
              key={sub.id}
              sub={sub}
              expanded={expanded === sub.id}
              onToggleExpand={() =>
                setExpanded((cur) => (cur === sub.id ? null : sub.id))
              }
              onRefresh={() => refreshMutation.mutate(sub.id)}
              refreshing={
                refreshMutation.isPending &&
                refreshMutation.variables === sub.id
              }
              onSelectBest={() => selectBestMutation.mutate(sub.id)}
              selecting={
                selectBestMutation.isPending &&
                selectBestMutation.variables === sub.id
              }
              onDelete={() => deleteConfirm.ask(sub)}
            />
          ))}
        </div>
      )}

      <AddSubscriptionDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        onCreated={() =>
          queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
        }
      />

      <ConfirmDialog
        open={deleteConfirm.open}
        onOpenChange={deleteConfirm.setOpen}
        destructive
        title="Delete subscription?"
        description={
          deleteConfirm.payload
            ? `“${deleteConfirm.payload.name}” and its imported servers will be removed.`
            : undefined
        }
        confirmLabel="Delete"
        loading={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteConfirm.payload) {
            deleteMutation.mutate(deleteConfirm.payload.id)
          }
        }}
      />
    </div>
  )
}

function SubscriptionCard({
  sub,
  expanded,
  onToggleExpand,
  onRefresh,
  refreshing,
  onSelectBest,
  selecting,
  onDelete,
}: {
  sub: Sub
  expanded: boolean
  onToggleExpand: () => void
  onRefresh: () => void
  refreshing: boolean
  onSelectBest: () => void
  selecting: boolean
  onDelete: () => void
}) {
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const usage = sub.userinfo
  const usedPct = pct(usage?.used_bytes, usage?.total_bytes)

  const autoMutation = useMutation({
    mutationFn: (next: boolean) =>
      api.updateSubscription(sub.id, { auto_select_best: next }),
    // Optimistically reflect the toggle so the control feels instant; roll back
    // to the previous cache if the request fails.
    onMutate: (next: boolean) => {
      const prev = queryClient.getQueryData<Sub[]>(['subscriptions'])
      queryClient.setQueryData<Sub[] | undefined>(['subscriptions'], (cur) =>
        cur?.map((s) =>
          s.id === sub.id ? { ...s, auto_select_best: next } : s,
        ),
      )
      return { prev }
    },
    onError: (_err, _next, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(['subscriptions'], ctx.prev)
      toast({ variant: 'error', title: 'Could not update auto-select best' })
    },
    onSuccess: (updated) => {
      toast({
        variant: 'success',
        title: updated.auto_select_best
          ? 'Auto-select best enabled'
          : 'Auto-select best disabled',
      })
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
    },
  })

  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-sky-500/15 text-sky-400">
                <Globe className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <h3 className="truncate text-sm font-semibold text-foreground">
                  {sub.name}
                </h3>
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {sub.host}
                </p>
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="mr-1 flex items-center gap-2 rounded-md border border-border/70 px-2.5 py-1">
              <Switch
                checked={sub.auto_select_best}
                disabled={autoMutation.isPending}
                onCheckedChange={(v) => autoMutation.mutate(v)}
                aria-label="Auto-select best server"
              />
              <span className="text-xs text-muted-foreground">Auto-best</span>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={onSelectBest}
              disabled={selecting}
              className="gap-1.5"
            >
              {selecting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Gauge className="h-3.5 w-3.5" />
              )}
              Select best
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={onRefresh}
              disabled={refreshing}
              className="gap-1.5"
            >
              <RefreshCw
                className={cn('h-3.5 w-3.5', refreshing && 'animate-spin')}
              />
              Refresh
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onDelete}
              aria-label="Delete subscription"
              className="text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* Meta grid */}
        <div className="mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4">
          <Meta label="Servers" value={String(sub.server_count)} />
          <Meta label="Protocol" value={sub.protocol} mono />
          <Meta
            label="Update interval"
            value={
              sub.update_interval_hours ? `${sub.update_interval_hours}h` : '—'
            }
          />
          <Meta label="Last update" value={timeAgo(sub.last_update)} />
        </div>

        {/* Data usage */}
        {usage ? (
          <div className="mt-4 space-y-1.5">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Data usage</span>
              <span className="font-mono tabular-nums text-foreground">
                {formatBytes(usage.used_bytes)} / {formatBytes(usage.total_bytes)}
              </span>
            </div>
            <Progress
              value={usedPct}
              indicatorClassName={cn(
                usedPct > 90
                  ? 'bg-destructive'
                  : usedPct > 75
                    ? 'bg-warning'
                    : 'bg-primary',
              )}
            />
            <div className="flex items-center justify-between text-[11px] text-muted-foreground">
              <span>{usedPct.toFixed(0)}% used</span>
              {usage.expire ? (
                <span>Expires {formatDate(usage.expire)}</span>
              ) : null}
            </div>
          </div>
        ) : null}

        <Separator className="my-4" />

        {/* Servers toggle */}
        <button
          type="button"
          onClick={onToggleExpand}
          className="flex w-full items-center justify-between text-left"
        >
          <span className="text-xs font-medium text-muted-foreground">
            {expanded ? 'Hide' : 'Show'} servers ({sub.server_count})
          </span>
          <ChevronDown
            className={cn(
              'h-4 w-4 text-muted-foreground transition-transform',
              expanded && 'rotate-180',
            )}
          />
        </button>

        {expanded ? <ServersList id={sub.id} /> : null}
      </CardContent>
    </Card>
  )
}

function ServersList({ id }: { id: string }) {
  const { data: servers, isLoading } = useQuery({
    queryKey: ['subscription-servers', id],
    queryFn: () => api.subscriptionServers(id),
  })

  if (isLoading) {
    return (
      <div className="mt-3 space-y-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-12" />
        ))}
      </div>
    )
  }

  if (!servers || servers.length === 0) {
    return (
      <div className="mt-3">
        <EmptyState
          icon={ServerIcon}
          title="No servers"
          description="This subscription hasn't imported any servers yet. Try refreshing."
          className="py-8"
        />
      </div>
    )
  }

  return (
    <div className="mt-3 space-y-2">
      {servers.map((server) => (
        <ServerRow key={server.id} server={server} />
      ))}
    </div>
  )
}

function ServerRow({ server }: { server: Server }) {
  const endpoint = `${server.address}:${server.port}`
  return (
    <div
      className={cn(
        'flex items-center gap-3 rounded-md border px-3 py-2.5',
        server.active
          ? 'border-primary/40 bg-primary/5'
          : 'border-border/60 bg-card',
      )}
    >
      <StatusDot status={server.status} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium text-foreground">
            {server.name}
          </span>
          {server.active ? (
            <span className="rounded-full bg-primary/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
              Active
            </span>
          ) : null}
        </div>
        <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
          <MapPin className="h-3 w-3 shrink-0" />
          <span className="truncate">{server.location}</span>
        </div>
      </div>
      <div className="flex items-center gap-1">
        <code className="hidden truncate rounded bg-muted/60 px-2 py-1 font-mono text-xs text-muted-foreground sm:block">
          {endpoint}
        </code>
        <CopyButton value={endpoint} label="Copy address" />
      </div>
      <span className="hidden font-mono text-[11px] text-muted-foreground md:inline">
        {server.protocol}
      </span>
      <div className="w-16 text-right">
        <LatencyBadge ms={server.latency_ms} />
      </div>
      <StatusBadge status={server.status} className="hidden lg:inline-flex" />
    </div>
  )
}

function AddSubscriptionDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}) {
  const { toast } = useToast()
  const [name, setName] = React.useState('')
  const [url, setUrl] = React.useState('')

  const reset = () => {
    setName('')
    setUrl('')
  }

  const createMutation = useMutation({
    mutationFn: () =>
      api.createSubscription({ name: name.trim(), url: url.trim() }),
    onSuccess: () => {
      onCreated()
      toast({
        variant: 'success',
        title: 'Subscription added',
        description: `“${name.trim()}” will import its servers shortly.`,
      })
      reset()
      onOpenChange(false)
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not add subscription' }),
  })

  const canSubmit = name.trim().length > 0 && url.trim().length > 0

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset()
        onOpenChange(o)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add subscription</DialogTitle>
          <DialogDescription>
            Import an Xray subscription feed. The server list refreshes on the
            configured interval.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="sub-name">Name</Label>
            <Input
              id="sub-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. OceanLink Premium"
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="sub-url">Subscription URL</Label>
            <Input
              id="sub-url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://host/s/<token>"
              className="font-mono text-xs"
              spellCheck={false}
            />
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => createMutation.mutate()}
            disabled={!canSubmit || createMutation.isPending}
            className="gap-1.5"
          >
            {createMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : null}
            Add subscription
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Meta({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="space-y-0.5">
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </p>
      <p
        className={cn(
          'truncate text-sm text-foreground',
          mono && 'font-mono text-xs',
        )}
      >
        {value}
      </p>
    </div>
  )
}
