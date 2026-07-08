import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  ArrowRight,
  Gauge,
  GitBranch,
  Globe,
  Loader2,
  MapPin,
  Radio,
  RefreshCw,
  Shield,
  ShieldAlert,
  Zap,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { ConnectionTile } from '@/components/ConnectionTile'
import { StatusDot } from '@/components/StatusDot'
import { TypeBadge } from '@/components/TypeBadge'
import { LatencyBadge } from '@/components/LatencyBadge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { useConnectionActions } from '@/hooks/use-actions'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn, formatUptime, timeAgo } from '@/lib/utils'
import type { AppState, Conn } from '@/lib/types'

const STATE_POLL_MS = 5000

export function DashboardPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const actions = useConnectionActions()

  const { data: state, isLoading } = useQuery({
    queryKey: ['state'],
    queryFn: api.state,
    refetchInterval: STATE_POLL_MS,
  })

  const connections = state?.connections ?? []
  const active = connections.find((c) => c.id === state?.active_connection_id)

  const killMutation = useMutation({
    mutationFn: (next: boolean) =>
      api.saveSettings({ kill_switch_default: next }),
    onSuccess: (_data, next) => {
      queryClient.setQueryData<AppState | undefined>(['state'], (prev) =>
        prev ? { ...prev, kill_switch: next } : prev,
      )
      queryClient.invalidateQueries({ queryKey: ['state'] })
      toast({
        variant: next ? 'warning' : 'success',
        title: next ? 'Kill-switch engaged' : 'Kill-switch released',
        description: next
          ? 'Traffic is locked to active tunnels.'
          : 'Normal routing restored.',
      })
    },
  })

  const nfqwsMutation = useMutation({
    mutationFn: () => api.nfqwsAction('restart'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nfqws'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      toast({ variant: 'success', title: 'nfqws2 restarting…' })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Could not restart nfqws2' }),
  })

  const upCount = connections.filter((c) => c.status === 'up').length
  const downCount = connections.filter(
    (c) => c.status === 'down' || c.status === 'degraded',
  ).length

  const failoverNode = React.useMemo(() => {
    const fo = state?.failover
    if (!fo) return undefined
    const id = fo.chain[fo.current_index]
    if (!id) return undefined
    if (id === 'direct') return 'Direct WAN'
    return connections.find((c) => c.id === id)?.name ?? id
  }, [state?.failover, connections])

  const activateBest = () => {
    const candidates = connections
      .filter((c) => c.enabled && c.status === 'up' && !c.active)
      .sort((a, b) => (a.latency_ms ?? 1e9) - (b.latency_ms ?? 1e9))
    const best = candidates[0]
    if (!best) {
      toast({
        variant: 'warning',
        title: 'No better connection available',
        description: 'All healthy tunnels are already active or unavailable.',
      })
      return
    }
    actions.activate(best.id, best.name)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        description="Overview & quick actions for your VPN and DPI-bypass stack."
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={activateBest}
              className="gap-1.5"
            >
              <Gauge className="h-4 w-4" />
              Activate best
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => nfqwsMutation.mutate()}
              disabled={nfqwsMutation.isPending}
              className="gap-1.5"
            >
              {nfqwsMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              Restart nfqws2
            </Button>
          </>
        }
      />

      {isLoading ? (
        <DashboardSkeleton />
      ) : (
        <>
          {/* WAN + active hero */}
          <div className="grid gap-4 lg:grid-cols-3">
            <WanCard state={state} />
            <ActiveHeroCard
              active={active}
              killSwitch={state?.kill_switch ?? false}
              onToggleKill={(next) => killMutation.mutate(next)}
              killPending={killMutation.isPending}
            />
          </div>

          {/* Summary stats */}
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              icon={Activity}
              label="Connections"
              value={String(connections.length)}
              hint={`${upCount} up · ${downCount} down`}
              tone="default"
            />
            <StatCard
              icon={Radio}
              label="nfqws2 DPI bypass"
              value={
                state?.nfqws.installed
                  ? state.nfqws.running
                    ? 'Running'
                    : 'Stopped'
                  : 'Not installed'
              }
              hint={state?.nfqws.mode ?? '—'}
              tone={
                state?.nfqws.installed
                  ? state.nfqws.running
                    ? 'success'
                    : 'warning'
                  : 'muted'
              }
            />
            <StatCard
              icon={GitBranch}
              label="Failover"
              value={state?.failover.enabled ? 'Enabled' : 'Disabled'}
              hint={
                state?.failover.enabled
                  ? `Live: ${failoverNode ?? '—'}`
                  : 'No automatic fallback'
              }
              tone={state?.failover.enabled ? 'success' : 'muted'}
            />
            <StatCard
              icon={state?.kill_switch ? ShieldAlert : Shield}
              label="Kill-switch"
              value={state?.kill_switch ? 'Engaged' : 'Off'}
              hint={
                state?.kill_switch
                  ? 'Non-tunnel traffic blocked'
                  : 'Fallback to WAN allowed'
              }
              tone={state?.kill_switch ? 'warning' : 'muted'}
            />
          </div>

          {/* Connections grid */}
          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <div className="space-y-1">
                <CardTitle>Connections</CardTitle>
                <p className="text-xs text-muted-foreground">
                  Live health across all tunnels
                </p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => navigate('/connections')}
                className="gap-1 text-muted-foreground"
              >
                Manage
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            </CardHeader>
            <CardContent>
              {connections.length === 0 ? (
                <EmptyState
                  icon={Activity}
                  title="No connections yet"
                  description="Add an AmneziaWG or Xray tunnel to get started."
                  action={
                    <Button size="sm" onClick={() => navigate('/connections')}>
                      Add connection
                    </Button>
                  }
                />
              ) : (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {connections.map((conn) => (
                    <ConnectionTile
                      key={conn.id}
                      conn={conn}
                      onClick={() => navigate('/connections')}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

function WanCard({ state }: { state?: AppState }) {
  const wan = state?.wan
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <Globe className="h-4 w-4 text-muted-foreground" />
          <CardTitle>WAN</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <InfoRow label="Interface">
          <span className="font-mono text-sm text-foreground">
            {wan?.interface ?? '—'}
          </span>
        </InfoRow>
        <Separator />
        <InfoRow label="Public IP">
          <span className="font-mono text-sm tabular-nums text-foreground">
            {wan?.ip ?? '—'}
          </span>
        </InfoRow>
        <Separator />
        <InfoRow label="Uptime">
          <span className="font-mono text-sm tabular-nums text-foreground">
            {formatUptime(wan?.uptime_seconds)}
          </span>
        </InfoRow>
      </CardContent>
    </Card>
  )
}

function ActiveHeroCard({
  active,
  killSwitch,
  onToggleKill,
  killPending,
}: {
  active?: Conn
  killSwitch: boolean
  onToggleKill: (next: boolean) => void
  killPending: boolean
}) {
  return (
    <Card className="lg:col-span-2">
      <CardContent className="flex h-full flex-col gap-4 p-5">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 space-y-1">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Active route
            </p>
            {active ? (
              <div className="flex items-center gap-2.5">
                <StatusDot status={active.status} />
                <span className="truncate text-lg font-semibold tracking-tight text-foreground">
                  {active.name}
                </span>
                <TypeBadge type={active.type} />
              </div>
            ) : (
              <div className="flex items-center gap-2.5">
                <StatusDot status="down" />
                <span className="text-lg font-semibold tracking-tight text-destructive">
                  No active connection
                </span>
              </div>
            )}
            {active?.location ? (
              <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                <MapPin className="h-3.5 w-3.5" />
                <span>{active.location}</span>
              </div>
            ) : null}
          </div>
          {active ? (
            <div className="shrink-0 text-right">
              <LatencyBadge ms={active.latency_ms} className="text-base" />
              <p className="mt-1 text-[11px] text-muted-foreground">
                {timeAgo(active.last_check)}
              </p>
            </div>
          ) : null}
        </div>

        {active?.endpoint ? (
          <div className="rounded-md border border-border/70 bg-muted/40 px-3 py-2">
            <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              Endpoint
            </p>
            <p className="mt-0.5 truncate font-mono text-sm text-foreground">
              {active.endpoint}
            </p>
          </div>
        ) : null}

        <div className="mt-auto flex items-center justify-between rounded-md border border-border/70 bg-card px-3 py-2.5">
          <div className="flex items-center gap-2.5">
            <div
              className={cn(
                'flex h-8 w-8 items-center justify-center rounded-md',
                killSwitch
                  ? 'bg-warning/15 text-warning'
                  : 'bg-muted text-muted-foreground',
              )}
            >
              {killSwitch ? (
                <ShieldAlert className="h-4 w-4" />
              ) : (
                <Shield className="h-4 w-4" />
              )}
            </div>
            <div className="leading-tight">
              <p className="text-sm font-medium text-foreground">Kill-switch</p>
              <p className="text-xs text-muted-foreground">
                {killSwitch
                  ? 'Blocking non-tunnel traffic'
                  : 'Allow fallback to WAN'}
              </p>
            </div>
          </div>
          <Switch
            checked={killSwitch}
            disabled={killPending}
            onCheckedChange={onToggleKill}
            aria-label="Toggle kill-switch"
          />
        </div>
      </CardContent>
    </Card>
  )
}

type StatTone = 'default' | 'success' | 'warning' | 'muted'

const toneClasses: Record<StatTone, string> = {
  default: 'bg-primary/15 text-primary',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  muted: 'bg-muted text-muted-foreground',
}

function StatCard({
  icon: Icon,
  label,
  value,
  hint,
  tone,
}: {
  icon: typeof Zap
  label: string
  value: string
  hint: string
  tone: StatTone
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div
          className={cn(
            'flex h-10 w-10 shrink-0 items-center justify-center rounded-lg',
            toneClasses[tone],
          )}
        >
          <Icon className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">{label}</p>
          <p className="truncate text-lg font-semibold tracking-tight text-foreground">
            {value}
          </p>
          <p className="truncate text-[11px] text-muted-foreground">{hint}</p>
        </div>
      </CardContent>
    </Card>
  )
}

function InfoRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </div>
  )
}

function DashboardSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-3">
        <Skeleton className="h-44" />
        <Skeleton className="h-44 lg:col-span-2" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-20" />
        ))}
      </div>
      <Skeleton className="h-64" />
    </div>
  )
}
