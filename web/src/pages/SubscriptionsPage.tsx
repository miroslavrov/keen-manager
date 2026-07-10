import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronDown,
  Gauge,
  Globe,
  Loader2,
  MapPin,
  Plus,
  Power,
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
import { useT } from '@/i18n'
import type { Server, Sub } from '@/lib/types'

export function SubscriptionsPage() {
  const t = useT()
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
      toast({ variant: 'success', title: t('subscriptions.refreshedTitle') })
    },
    onError: () =>
      toast({
        variant: 'error',
        title: t('subscriptions.refreshErrorTitle'),
      }),
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
        title: t('subscriptions.selectedBestTitle'),
        description: picked
          ? `${picked.name} (${Math.round(picked.latency_ms ?? 0)} ms)`
          : undefined,
      })
    },
    onError: (err) =>
      toast({
        variant: 'error',
        title: t('subscriptions.selectBestErrorTitle'),
        // The daemon explains *why* (no reachable server, or the tunnel failed
        // verification with the probe target + reason) — show it verbatim.
        description: err instanceof Error && err.message ? err.message : undefined,
      }),
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
        title: t('subscriptions.removedTitle'),
        description: name
          ? t('subscriptions.removedDesc', { name })
          : undefined,
      })
    },
    onError: () =>
      toast({
        variant: 'error',
        title: t('subscriptions.removeErrorTitle'),
      }),
  })

  const list = subs ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('subscriptions.title')}
        description={t('subscriptions.desc')}
        actions={
          <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1.5">
            <Plus className="h-4 w-4" />
            {t('subscriptions.addSubscription')}
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
          title={t('subscriptions.emptyTitle')}
          description={t('subscriptions.emptyDesc')}
          action={
            <Button size="sm" onClick={() => setAddOpen(true)}>
              {t('subscriptions.addSubscription')}
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
        title={t('subscriptions.deleteTitle')}
        description={
          deleteConfirm.payload
            ? t('subscriptions.deleteDesc', { name: deleteConfirm.payload.name })
            : undefined
        }
        confirmLabel={t('common.delete')}
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
  const t = useT()
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
      toast({
        variant: 'error',
        title: t('subscriptions.autoBestErrorTitle'),
      })
    },
    onSuccess: (updated) => {
      toast({
        variant: 'success',
        title: updated.auto_select_best
          ? t('subscriptions.autoBestEnabledTitle')
          : t('subscriptions.autoBestDisabledTitle'),
      })
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
    },
  })

  // Subscription-stream on/off — the middle egress level. Turning it off can
  // tear down the active tunnel server-side (LAN → direct), so also refresh
  // connections and state, not just the subscription list.
  const enabledMutation = useMutation({
    mutationFn: (next: boolean) =>
      api.updateSubscription(sub.id, { enabled: next }),
    onMutate: (next: boolean) => {
      const prev = queryClient.getQueryData<Sub[]>(['subscriptions'])
      queryClient.setQueryData<Sub[] | undefined>(['subscriptions'], (cur) =>
        cur?.map((s) => (s.id === sub.id ? { ...s, enabled: next } : s)),
      )
      return { prev }
    },
    onError: (_err, _next, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(['subscriptions'], ctx.prev)
      toast({ variant: 'error', title: t('subscriptions.streamErrorTitle') })
    },
    onSuccess: (updated) => {
      toast({
        variant: 'success',
        title: updated.enabled
          ? t('subscriptions.streamEnabledTitle')
          : t('subscriptions.streamDisabledTitle'),
      })
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['connections'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
    },
  })

  return (
    <Card className={cn(!sub.enabled && 'border-border/50')}>
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-sky-500/15 text-sky-400">
                <Globe className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <h3 className="truncate text-sm font-semibold text-foreground">
                    {sub.name}
                  </h3>
                  {!sub.enabled ? (
                    <span className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                      {t('subscriptions.pausedBadge')}
                    </span>
                  ) : null}
                </div>
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {sub.host}
                </p>
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div
              className={cn(
                'mr-1 flex items-center gap-2 rounded-md border px-2.5 py-1',
                sub.enabled
                  ? 'border-primary/40 bg-primary/5'
                  : 'border-border/70',
              )}
              title={t('subscriptions.streamTitle')}
            >
              <Power
                className={cn(
                  'h-3.5 w-3.5',
                  sub.enabled ? 'text-primary' : 'text-muted-foreground',
                )}
              />
              <span className="text-xs font-medium text-foreground">
                {t('subscriptions.stream')}
              </span>
              <Switch
                checked={sub.enabled}
                disabled={enabledMutation.isPending}
                onCheckedChange={(v) => enabledMutation.mutate(v)}
                aria-label={t('subscriptions.streamAria')}
              />
            </div>
            <div className="flex items-center gap-2 rounded-md border border-border/70 px-2.5 py-1">
              <Switch
                checked={sub.auto_select_best}
                disabled={autoMutation.isPending || !sub.enabled}
                onCheckedChange={(v) => autoMutation.mutate(v)}
                aria-label={t('subscriptions.autoSelectAria')}
              />
              <span className="text-xs text-muted-foreground">
                {t('subscriptions.autoBest')}
              </span>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={onSelectBest}
              disabled={selecting || !sub.enabled}
              title={
                !sub.enabled
                  ? t('subscriptions.selectBestDisabledHint')
                  : undefined
              }
              className="gap-1.5"
            >
              {selecting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Gauge className="h-3.5 w-3.5" />
              )}
              {t('subscriptions.selectBest')}
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
              {t('common.refresh')}
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onDelete}
              aria-label={t('subscriptions.deleteAria')}
              className="text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>

        {/* Meta grid */}
        <div
          className={cn(
            'mt-4 grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4',
            !sub.enabled && 'opacity-60',
          )}
        >
          <Meta label={t('subscriptions.metaServers')} value={String(sub.server_count)} />
          <Meta label={t('subscriptions.metaProtocol')} value={sub.protocol} mono />
          <Meta
            label={t('subscriptions.metaUpdateInterval')}
            value={
              sub.update_interval_hours ? `${sub.update_interval_hours}h` : '—'
            }
          />
          <Meta label={t('subscriptions.metaLastUpdate')} value={timeAgo(sub.last_update)} />
        </div>

        {/* Data usage */}
        {usage ? (
          <div className={cn('mt-4 space-y-1.5', !sub.enabled && 'opacity-60')}>
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">
                {t('subscriptions.dataUsage')}
              </span>
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
              <span>
                {t('subscriptions.usedPct', { pct: usedPct.toFixed(0) })}
              </span>
              {usage.expire ? (
                <span>
                  {t('subscriptions.expires', {
                    date: formatDate(usage.expire),
                  })}
                </span>
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
            {expanded
              ? t('subscriptions.hideServers', { count: sub.server_count })
              : t('subscriptions.showServers', { count: sub.server_count })}
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
  const t = useT()
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
          title={t('subscriptions.noServersTitle')}
          description={t('subscriptions.noServersDesc')}
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
  const t = useT()
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
              {t('common.active')}
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
        <CopyButton value={endpoint} label={t('subscriptions.copyAddress')} />
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
  const t = useT()
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
        title: t('subscriptions.createdTitle'),
        description: t('subscriptions.createdDesc', { name: name.trim() }),
      })
      reset()
      onOpenChange(false)
    },
    onError: () =>
      toast({
        variant: 'error',
        title: t('subscriptions.createErrorTitle'),
      }),
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
          <DialogTitle>{t('subscriptions.addDialogTitle')}</DialogTitle>
          <DialogDescription>
            {t('subscriptions.addDialogDesc')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="sub-name">{t('subscriptions.nameLabel')}</Label>
            <Input
              id="sub-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('subscriptions.namePlaceholder')}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="sub-url">{t('subscriptions.urlLabel')}</Label>
            <Input
              id="sub-url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={t('subscriptions.urlPlaceholder')}
              className="font-mono text-xs"
              spellCheck={false}
            />
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => createMutation.mutate()}
            disabled={!canSubmit || createMutation.isPending}
            className="gap-1.5"
          >
            {createMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : null}
            {t('subscriptions.addSubscription')}
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
