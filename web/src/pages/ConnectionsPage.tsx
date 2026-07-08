import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  ArrowDownCircle,
  ArrowUpCircle,
  Gauge,
  Loader2,
  MapPin,
  MoreHorizontal,
  Plus,
  Radio,
  Signal,
  Trash2,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { StatusDot } from '@/components/StatusDot'
import { StatusBadge } from '@/components/StatusBadge'
import { TypeBadge } from '@/components/TypeBadge'
import { LatencyBadge } from '@/components/LatencyBadge'
import { CopyButton } from '@/components/CopyButton'
import { ConfirmDialog, useConfirm } from '@/components/ConfirmDialog'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useConnectionActions } from '@/hooks/use-actions'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn, formatBytes, secondsAgoLabel, timeAgo } from '@/lib/utils'
import { useT } from '@/i18n'
import type { Conn, ConnType } from '@/lib/types'

const NONE_VALUE = '__none__'

export function ConnectionsPage() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const actions = useConnectionActions()

  const [addOpen, setAddOpen] = React.useState(false)
  const [detailId, setDetailId] = React.useState<string | null>(null)
  const deleteConfirm = useConfirm<Conn>()

  const { data: connections, isLoading } = useQuery({
    queryKey: ['connections'],
    queryFn: api.connections,
    refetchInterval: 5000,
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['connections'] })
    queryClient.invalidateQueries({ queryKey: ['state'] })
  }

  const updateMutation = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<Conn> }) =>
      api.updateConnection(id, body),
    onSuccess: () => invalidate(),
    onError: () =>
      toast({ variant: 'error', title: t('connections.updateError') }),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteConnection(id),
    onSuccess: (_d, id) => {
      const name = connections?.find((c) => c.id === id)?.name
      invalidate()
      deleteConfirm.close()
      toast({
        variant: 'success',
        title: t('connections.deleted'),
        description: name
          ? t('connections.deletedDesc', { name })
          : undefined,
      })
    },
    onError: () =>
      toast({ variant: 'error', title: t('connections.deleteError') }),
  })

  const list = connections ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('connections.title')}
        description={t('connections.desc')}
        actions={
          <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1.5">
            <Plus className="h-4 w-4" />
            {t('connections.addConnection')}
          </Button>
        }
      />

      {isLoading ? (
        <div className="space-y-2.5">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-[76px]" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <EmptyState
          icon={Activity}
          title={t('connections.emptyTitle')}
          description={t('connections.emptyDesc')}
          action={
            <Button size="sm" onClick={() => setAddOpen(true)}>
              {t('connections.addConnection')}
            </Button>
          }
        />
      ) : (
        <div className="space-y-2.5">
          {list.map((conn) => (
            <ConnectionRow
              key={conn.id}
              conn={conn}
              allConnections={list}
              actions={actions}
              onOpenDetail={() => setDetailId(conn.id)}
              onToggleEnabled={(enabled) =>
                updateMutation.mutate({ id: conn.id, body: { enabled } })
              }
              onSetFallback={(fallback_to) =>
                updateMutation.mutate({ id: conn.id, body: { fallback_to } })
              }
              onDelete={() => deleteConfirm.ask(conn)}
            />
          ))}
        </div>
      )}

      <AddConnectionDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        onCreated={invalidate}
      />

      <ConnectionDetailSheet
        id={detailId}
        onOpenChange={(open) => {
          if (!open) setDetailId(null)
        }}
      />

      <ConfirmDialog
        open={deleteConfirm.open}
        onOpenChange={deleteConfirm.setOpen}
        destructive
        title={t('connections.deleteTitle')}
        description={
          deleteConfirm.payload
            ? t('connections.deleteDesc', { name: deleteConfirm.payload.name })
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

function ConnectionRow({
  conn,
  allConnections,
  actions,
  onOpenDetail,
  onToggleEnabled,
  onSetFallback,
  onDelete,
}: {
  conn: Conn
  allConnections: Conn[]
  actions: ReturnType<typeof useConnectionActions>
  onOpenDetail: () => void
  onToggleEnabled: (enabled: boolean) => void
  onSetFallback: (fallbackTo: string) => void
  onDelete: () => void
}) {
  const t = useT()
  const pendingThis =
    actions.pending && actions.pendingVars?.id === conn.id
  const fallbackName =
    conn.fallback_to === 'direct'
      ? t('common.directWan')
      : allConnections.find((c) => c.id === conn.fallback_to)?.name

  return (
    <Card
      className={cn(
        'transition-colors',
        conn.active && 'border-primary/50 ring-1 ring-inset ring-primary/15',
      )}
    >
      <CardContent className="flex flex-col gap-3 p-4 lg:flex-row lg:items-center">
        {/* Identity */}
        <button
          type="button"
          onClick={onOpenDetail}
          className="flex min-w-0 flex-1 items-center gap-3 text-left"
        >
          <StatusDot status={conn.status} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium text-foreground">
                {conn.name}
              </span>
              <TypeBadge type={conn.type} />
              {conn.active ? (
                <span className="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
                  <Radio className="h-3 w-3" />
                  {t('common.active')}
                </span>
              ) : null}
            </div>
            <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
              <MapPin className="h-3 w-3 shrink-0" />
              <span className="truncate">
                {conn.location ?? t('common.unknownLocation')}
              </span>
            </div>
          </div>
        </button>

        {/* Endpoint */}
        <div className="flex min-w-0 items-center gap-1 lg:w-56">
          {conn.endpoint ? (
            <>
              <code className="truncate rounded bg-muted/60 px-2 py-1 font-mono text-xs text-muted-foreground">
                {conn.endpoint}
              </code>
              <CopyButton
                value={conn.endpoint}
                label={t('connections.copyEndpoint')}
              />
            </>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          )}
        </div>

        {/* Metrics */}
        <div className="flex items-center gap-4 lg:w-40 lg:justify-end">
          <LatencyBadge ms={conn.latency_ms} />
          <span className="hidden text-xs text-muted-foreground sm:inline">
            {conn.enabled ? timeAgo(conn.last_check) : t('common.disabled')}
          </span>
        </div>

        {/* Controls */}
        <div className="flex items-center justify-between gap-2 lg:justify-end">
          <div className="flex items-center gap-1.5">
            <Switch
              checked={conn.enabled}
              onCheckedChange={onToggleEnabled}
              aria-label={t(
                conn.enabled ? 'connections.disableName' : 'connections.enableName',
                { name: conn.name },
              )}
            />
          </div>

          {!conn.active ? (
            <Button
              variant="outline"
              size="sm"
              disabled={!conn.enabled || pendingThis}
              onClick={() => actions.activate(conn.id, conn.name)}
              className="gap-1.5"
            >
              {pendingThis && actions.pendingVars?.action === 'activate' ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Gauge className="h-3.5 w-3.5" />
              )}
              {t('connections.activate')}
            </Button>
          ) : (
            <span className="hidden text-xs font-medium text-primary sm:inline">
              {t('common.routingTraffic')}
            </span>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('connections.connectionActions')}
              >
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuLabel>{conn.name}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={() => actions.up(conn.id, conn.name)}
                disabled={!conn.enabled}
              >
                <ArrowUpCircle className="h-4 w-4" />
                {t('connections.bringUp')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => actions.down(conn.id, conn.name)}
              >
                <ArrowDownCircle className="h-4 w-4" />
                {t('connections.takeDown')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => actions.test(conn.id, conn.name)}
                disabled={!conn.enabled}
              >
                <Signal className="h-4 w-4" />
                {t('connections.testReachability')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <div className="px-2 py-1.5">
                <p className="mb-1.5 text-[11px] font-medium text-muted-foreground">
                  {t('connections.fallbackTarget')}
                </p>
                <Select
                  value={conn.fallback_to ?? NONE_VALUE}
                  onValueChange={(v) =>
                    onSetFallback(v === NONE_VALUE ? '' : v)
                  }
                >
                  <SelectTrigger className="h-8 text-xs">
                    <SelectValue placeholder={t('common.none')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE_VALUE}>
                      {t('common.none')}
                    </SelectItem>
                    <SelectItem value="direct">
                      {t('common.directWan')}
                    </SelectItem>
                    {allConnections
                      .filter((c) => c.id !== conn.id)
                      .map((c) => (
                        <SelectItem key={c.id} value={c.id}>
                          {c.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </div>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={onDelete}
                className="text-destructive focus:text-destructive"
              >
                <Trash2 className="h-4 w-4" />
                {t('common.delete')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </CardContent>

      {fallbackName ? (
        <div className="border-t border-border/60 px-4 py-1.5">
          <span className="text-[11px] text-muted-foreground">
            {t('connections.fallsBackTo')}{' '}
            <span className="font-medium text-foreground">{fallbackName}</span>
          </span>
        </div>
      ) : null}
    </Card>
  )
}

function AddConnectionDialog({
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
  const [type, setType] = React.useState<ConnType>('awg')
  const [name, setName] = React.useState('')
  const [awgConf, setAwgConf] = React.useState('')
  const [shareLink, setShareLink] = React.useState('')

  const reset = () => {
    setType('awg')
    setName('')
    setAwgConf('')
    setShareLink('')
  }

  const createMutation = useMutation({
    mutationFn: () =>
      api.createConnection({
        type,
        name: name.trim(),
        awg_conf: type === 'awg' ? awgConf : undefined,
        share_link: type === 'xray' ? shareLink.trim() : undefined,
      }),
    onSuccess: () => {
      onCreated()
      toast({
        variant: 'success',
        title: t('connections.added'),
        description: t('connections.addedDesc', { name: name.trim() }),
      })
      reset()
      onOpenChange(false)
    },
    onError: () =>
      toast({ variant: 'error', title: t('connections.addError') }),
  })

  const canSubmit =
    name.trim().length > 0 &&
    (type === 'awg' ? awgConf.trim().length > 0 : shareLink.trim().length > 0)

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset()
        onOpenChange(o)
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('connections.addTitle')}</DialogTitle>
          <DialogDescription>{t('connections.addDesc')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="conn-name">{t('connections.name')}</Label>
            <Input
              id="conn-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('connections.namePlaceholder')}
              autoFocus
            />
          </div>

          <Tabs value={type} onValueChange={(v) => setType(v as ConnType)}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="awg">AmneziaWG</TabsTrigger>
              <TabsTrigger value="xray">Xray</TabsTrigger>
            </TabsList>

            <TabsContent value="awg" className="space-y-2">
              <Label htmlFor="awg-conf">{t('connections.configLabel')}</Label>
              <Textarea
                id="awg-conf"
                value={awgConf}
                onChange={(e) => setAwgConf(e.target.value)}
                placeholder={
                  '[Interface]\nPrivateKey = …\nAddress = 10.13.13.2/32\n\n[Peer]\nPublicKey = …\nEndpoint = host:51820\nAllowedIPs = 0.0.0.0/0'
                }
                className="min-h-[220px] font-mono text-xs leading-relaxed"
                spellCheck={false}
              />
              <p className="text-xs text-muted-foreground">
                {t('connections.configHintBefore')}{' '}
                <span className="font-mono">.conf</span>
                {t('connections.configHintAfter')}
              </p>
            </TabsContent>

            <TabsContent value="xray" className="space-y-2">
              <Label htmlFor="share-link">{t('connections.shareLink')}</Label>
              <Input
                id="share-link"
                value={shareLink}
                onChange={(e) => setShareLink(e.target.value)}
                placeholder="vless:// · vmess:// · trojan:// · ss://"
                className="font-mono text-xs"
                spellCheck={false}
              />
              <p className="text-xs text-muted-foreground">
                {t('connections.shareLinkHint')}
              </p>
            </TabsContent>
          </Tabs>
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
            {t('connections.addConnection')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ConnectionDetailSheet({
  id,
  onOpenChange,
}: {
  id: string | null
  onOpenChange: (open: boolean) => void
}) {
  const t = useT()
  const { data: detail, isLoading } = useQuery({
    queryKey: ['connection', id],
    queryFn: () => api.connection(id as string),
    enabled: !!id,
  })

  return (
    <Sheet open={!!id} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full gap-0 p-0 sm:max-w-lg">
        <SheetHeader className="border-b border-border px-5 py-4">
          <SheetTitle className="flex items-center gap-2">
            {detail ? (
              <>
                <StatusDot status={detail.status} />
                {detail.name}
              </>
            ) : (
              t('connections.detailFallback')
            )}
          </SheetTitle>
          <SheetDescription>
            {detail?.protocol ?? t('connections.detailDescFallback')}
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 space-y-5 overflow-y-auto px-5 py-5">
          {isLoading || !detail ? (
            <div className="space-y-3">
              <Skeleton className="h-24" />
              <Skeleton className="h-40" />
            </div>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <TypeBadge type={detail.type} />
                <StatusBadge status={detail.status} />
                {detail.active ? (
                  <span className="inline-flex items-center gap-1 rounded-full bg-primary/15 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-primary">
                    <Radio className="h-3 w-3" />
                    {t('common.active')}
                  </span>
                ) : null}
              </div>

              <div className="grid grid-cols-2 gap-3">
                <MetricTile
                  label={t('connections.latency')}
                  value={
                    detail.latency_ms !== undefined
                      ? `${Math.round(detail.latency_ms)} ms`
                      : '—'
                  }
                />
                <MetricTile
                  label={t('connections.handshake')}
                  value={secondsAgoLabel(detail.handshake_age_s)}
                />
                <MetricTile
                  label={t('connections.received')}
                  value={formatBytes(detail.rx_bytes)}
                />
                <MetricTile
                  label={t('connections.transmitted')}
                  value={formatBytes(detail.tx_bytes)}
                />
              </div>

              {detail.endpoint ? (
                <div className="space-y-1.5">
                  <p className="text-xs font-medium text-muted-foreground">
                    {t('connections.endpoint')}
                  </p>
                  <div className="flex items-center gap-1">
                    <code className="flex-1 truncate rounded bg-muted/60 px-2 py-1.5 font-mono text-xs text-foreground">
                      {detail.endpoint}
                    </code>
                    <CopyButton value={detail.endpoint} />
                  </div>
                </div>
              ) : null}

              {detail.config_preview ? (
                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <p className="text-xs font-medium text-muted-foreground">
                      {t('connections.configPreview')}
                    </p>
                    <CopyButton
                      value={detail.config_preview}
                      label={t('connections.copyConfig')}
                    />
                  </div>
                  <pre className="max-h-80 overflow-auto rounded-md border border-border bg-muted/40 p-3 font-mono text-xs leading-relaxed text-foreground">
                    {detail.config_preview}
                  </pre>
                </div>
              ) : null}

              {detail.last_check ? (
                <>
                  <Separator />
                  <p className="text-xs text-muted-foreground">
                    {t('connections.lastChecked', {
                      time: timeAgo(detail.last_check),
                    })}
                  </p>
                </>
              ) : null}
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function MetricTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border/70 bg-card px-3 py-2.5">
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </p>
      <p className="mt-0.5 font-mono text-sm tabular-nums text-foreground">
        {value}
      </p>
    </div>
  )
}
