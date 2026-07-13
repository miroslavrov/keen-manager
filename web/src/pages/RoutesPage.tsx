import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Cloud,
  Code2,
  Download,
  Gamepad2,
  Globe,
  Loader2,
  Network,
  Pencil,
  PlayCircle,
  Plus,
  Search,
  ShieldBan,
  ShieldCheck,
  Sparkles,
  Trash2,
  Users,
  Waypoints,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { ConfirmDialog, useConfirm } from '@/components/ConfirmDialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useT } from '@/i18n'
import type { Preset, RouteEntry } from '@/lib/types'

// A route target is encoded as "conn:<id>" (a keen-manager connection) or
// "iface:<name>" (a router interface picked from the live KeeneticOS list), so a
// single dropdown can offer both. decodeTarget turns it back into the request
// body the daemon expects (target_conn_id XOR target_iface).
function decodeTarget(v: string): { target_conn_id?: string; target_iface?: string } {
  if (v.startsWith('iface:')) return { target_iface: v.slice('iface:'.length) }
  if (v.startsWith('conn:')) return { target_conn_id: v.slice('conn:'.length) }
  if (v.startsWith('sub:')) return { target_conn_id: 'sub:' + v.slice('sub:'.length) }
  return {}
}

// Category visual metadata: a lucide glyph + a tint. Kept literal (not computed)
// so Tailwind's content scanner keeps every class in the bundle.
const CATEGORY: Record<
  string,
  { icon: LucideIcon; tile: string; labelKey: string }
> = {
  social: { icon: Users, tile: 'bg-sky-500/15 text-sky-400', labelKey: 'routes.catSocial' },
  media: { icon: PlayCircle, tile: 'bg-rose-500/15 text-rose-400', labelKey: 'routes.catMedia' },
  ai: { icon: Sparkles, tile: 'bg-violet-500/15 text-violet-400', labelKey: 'routes.catAi' },
  gaming: { icon: Gamepad2, tile: 'bg-emerald-500/15 text-emerald-400', labelKey: 'routes.catGaming' },
  developer: { icon: Code2, tile: 'bg-amber-500/15 text-amber-400', labelKey: 'routes.catDeveloper' },
  cloud: { icon: Cloud, tile: 'bg-cyan-500/15 text-cyan-400', labelKey: 'routes.catCloud' },
  block: { icon: ShieldBan, tile: 'bg-red-500/15 text-red-400', labelKey: 'routes.catBlock' },
  custom: { icon: Waypoints, tile: 'bg-slate-500/15 text-slate-400', labelKey: 'routes.catCustom' },
}

function catMeta(category?: string) {
  return CATEGORY[category ?? 'custom'] ?? CATEGORY.custom
}

/** A compact service tile: a monogram on a category tint. Brand-neutral, clean,
 * and dependency-free — the service NAME carries recognition. */
function ServiceTile({
  name,
  category,
  className,
}: {
  name: string
  category?: string
  className?: string
}) {
  const meta = catMeta(category)
  const letter = (name.trim()[0] ?? '?').toUpperCase()
  return (
    <div
      className={cn(
        'flex items-center justify-center rounded-lg font-semibold',
        meta.tile,
        className,
      )}
    >
      {letter}
    </div>
  )
}

/** One selectable route target — a keen-manager connection or a live router
 * interface, unified behind a prefixed value ("conn:…" / "iface:…"). */
interface TargetOption {
  value: string
  label: string
  hint?: string
  /** For interfaces: whether it is currently up on the router. */
  live?: boolean
}

export function RoutesPage() {
  const t = useT()
  const { data: connections } = useQuery({
    queryKey: ['connections'],
    queryFn: api.connections,
    refetchInterval: 8000,
  })
  // Router interfaces pulled live from KeeneticOS over RCI (GET /api/interfaces)
  // — the "interfaces from the router" dropdown the user asked for.
  const { data: ifaceData } = useQuery({
    queryKey: ['interfaces'],
    queryFn: api.interfaces,
    refetchInterval: 15000,
  })
  // DPI-bypass exit point (tpws → managed Proxy interface). When enabled it is
  // an additional route target, so a route can send chosen domains through DPI
  // bypass exactly like a VPN tunnel — one shared source of domains with Routes.
  const { data: bypass } = useQuery({
    queryKey: ['bypass'],
    queryFn: api.bypass,
    refetchInterval: 15000,
  })
  // Subscriptions: a route can target an entire subscription ("sub:<id>"), so
  // any server from it (the active one) carries the routed traffic.
  const { data: subscriptions } = useQuery({
    queryKey: ['subscriptions'],
    queryFn: api.subscriptions,
    refetchInterval: 30000,
  })

  // AmneziaWG connections back a router-native dns-proxy route (via their native
  // WireguardN interface).
  const connOptions = React.useMemo<TargetOption[]>(
    () =>
      (connections ?? [])
        .filter((c) => c.type === 'awg')
        .map((c) => ({ value: `conn:${c.id}`, label: c.name, hint: c.location })),
    [connections],
  )

  // Xray connections are also valid targets now: the daemon compiles the route
  // into the active Xray config as split-tunnel routing (selected services via
  // the tunnel, everything else direct). Same "conn:<id>" encoding.
  const xrayOptions = React.useMemo<TargetOption[]>(
    () =>
      (connections ?? [])
        .filter((c) => c.type === 'xray')
        .map((c) => ({ value: `conn:${c.id}`, label: c.name, hint: c.location })),
    [connections],
  )

  // Interface targets: routable WireGuard interfaces read live from the router,
  // minus any a listed keen-manager connection already represents (shown as the
  // connection instead, so the same tunnel isn't offered twice).
  const connIds = React.useMemo(
    () => new Set((connections ?? []).map((c) => c.id)),
    [connections],
  )
  const ifaceOptions = React.useMemo<TargetOption[]>(
    () =>
      (ifaceData?.interfaces ?? [])
        .filter((i) => i.routable)
        .filter((i) => !(i.managed_conn_id && connIds.has(i.managed_conn_id)))
        .map((i) => ({
          value: `iface:${i.name}`,
          label: i.label || i.name,
          hint: i.name,
          live: i.up,
        })),
    [ifaceData, connIds],
  )

  // The DPI-bypass interface is offered as a single reserved target
  // ("conn:bypass") whenever the feature is enabled — decodeTarget maps it to
  // target_conn_id="bypass", which the daemon resolves to the managed tpws
  // Proxy interface (pending until it exists, like an Xray route).
  const bypassOptions = React.useMemo<TargetOption[]>(
    () =>
      bypass?.enabled
        ? [
            {
              value: 'conn:bypass',
              label: t('routes.bypassTarget'),
              hint: t('routes.bypassTargetHint'),
            },
          ]
        : [],
    [bypass, t],
  )

  // Subscription targets: route through any server in a subscription.
  // Encoded as "sub:<id>" — the daemon resolves it to the active member's
  // interface, re-evaluated on every activate/failover.
  const subOptions = React.useMemo<TargetOption[]>(
    () =>
      (subscriptions ?? [])
        .filter((s) => s.enabled)
        .map((s) => ({
          value: `sub:${s.id}`,
          label: s.name,
          hint: `${s.server_count} servers`,
        })),
    [subscriptions],
  )

  const allValues = React.useMemo(
    () =>
      [...connOptions, ...xrayOptions, ...subOptions, ...ifaceOptions, ...bypassOptions].map(
        (o) => o.value,
      ),
    [connOptions, xrayOptions, subOptions, ifaceOptions, bypassOptions],
  )
  const hasTarget = allValues.length > 0
  const [target, setTarget] = React.useState<string>('')

  // Keep a valid selection: default to the first available target and recover if
  // the current pick disappears (e.g. an interface went away on the router).
  React.useEffect(() => {
    if (allValues.length === 0) return
    if (!target || !allValues.includes(target)) setTarget(allValues[0])
  }, [allValues, target])

  const dnsAvailable = ifaceData?.dns_routing_available ?? true

  return (
    <div className="space-y-6">
      <PageHeader title={t('routes.title')} description={t('routes.desc')} />

      <TargetPicker
        connOptions={connOptions}
        xrayOptions={xrayOptions}
        subOptions={subOptions}
        ifaceOptions={ifaceOptions}
        bypassOptions={bypassOptions}
        value={target}
        onChange={setTarget}
        dnsAvailable={dnsAvailable}
        note={ifaceData?.note}
      />

      <Tabs defaultValue="catalog">
        <TabsList>
          <TabsTrigger value="catalog">{t('routes.tabCatalog')}</TabsTrigger>
          <TabsTrigger value="active">{t('routes.tabActive')}</TabsTrigger>
          <TabsTrigger value="custom">{t('routes.tabCustom')}</TabsTrigger>
        </TabsList>

        <TabsContent value="catalog">
          <CatalogTab target={target} hasTarget={hasTarget} />
        </TabsContent>
        <TabsContent value="active">
          <ActiveRoutesTab />
        </TabsContent>
        <TabsContent value="custom">
          <CustomTab target={target} hasTarget={hasTarget} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function TargetPicker({
  connOptions,
  xrayOptions,
  subOptions,
  ifaceOptions,
  bypassOptions,
  value,
  onChange,
  dnsAvailable,
  note,
}: {
  connOptions: TargetOption[]
  xrayOptions: TargetOption[]
  subOptions: TargetOption[]
  ifaceOptions: TargetOption[]
  bypassOptions: TargetOption[]
  value: string
  onChange: (v: string) => void
  dnsAvailable: boolean
  note?: string
}) {
  const t = useT()
  const hasAny =
    connOptions.length > 0 ||
    xrayOptions.length > 0 ||
    subOptions.length > 0 ||
    ifaceOptions.length > 0 ||
    bypassOptions.length > 0
  // Native DNS routing only gates the AWG/interface (dns-proxy) path; Xray
  // routes are compiled into the Xray config and don't need it, so the warning
  // is suppressed when the only available targets are Xray tunnels.
  const dnsMatters = connOptions.length > 0 || ifaceOptions.length > 0
  return (
    <Card>
      <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">
            {t('routes.targetLabel')}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {t('routes.targetHint')}
          </p>
          {!dnsAvailable && dnsMatters ? (
            <p className="mt-1 text-xs text-warning">
              {t('routes.dnsUnavailable')}
            </p>
          ) : null}
          {note ? (
            <p className="mt-1 text-xs text-muted-foreground">{note}</p>
          ) : null}
        </div>
        {!hasAny ? (
          <div className="shrink-0 sm:text-right">
            <Badge variant="warning">{t('routes.noTargets')}</Badge>
            <p className="mt-1 max-w-xs text-xs text-muted-foreground">
              {t('routes.noTargetsHint')}
            </p>
          </div>
        ) : (
          <Select value={value} onValueChange={onChange}>
            <SelectTrigger className="w-full sm:w-80">
              <SelectValue placeholder={t('routes.targetPlaceholder')} />
            </SelectTrigger>
            <SelectContent>
              {connOptions.length > 0 ? (
                <SelectGroup>
                  <SelectLabel>{t('routes.groupConnections')}</SelectLabel>
                  {connOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                      {o.hint ? (
                        <span className="text-muted-foreground"> · {o.hint}</span>
                      ) : null}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null}
              {connOptions.length > 0 && xrayOptions.length > 0 ? (
                <SelectSeparator />
              ) : null}
              {xrayOptions.length > 0 ? (
                <SelectGroup>
                  <SelectLabel className="flex items-center gap-1.5">
                    <Cloud className="h-3.5 w-3.5" />
                    {t('routes.groupXray')}
                  </SelectLabel>
                  {xrayOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                      <span className="text-muted-foreground">
                        {' · '}
                        {o.hint || t('routes.xrayTargetHint')}
                      </span>
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null}
              {(connOptions.length > 0 || xrayOptions.length > 0) &&
              subOptions.length > 0 ? (
                <SelectSeparator />
              ) : null}
              {subOptions.length > 0 ? (
                <SelectGroup>
                  <SelectLabel className="flex items-center gap-1.5">
                    <Globe className="h-3.5 w-3.5" />
                    {t('routes.groupSubscriptions')}
                  </SelectLabel>
                  {subOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                      {o.hint ? (
                        <span className="text-muted-foreground"> · {o.hint}</span>
                      ) : null}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null}
              {(connOptions.length > 0 || xrayOptions.length > 0 || subOptions.length > 0) &&
              ifaceOptions.length > 0 ? (
                <SelectSeparator />
              ) : null}
              {ifaceOptions.length > 0 ? (
                <SelectGroup>
                  <SelectLabel className="flex items-center gap-1.5">
                    <Network className="h-3.5 w-3.5" />
                    {t('routes.groupInterfaces')}
                  </SelectLabel>
                  {ifaceOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                      <span className="font-mono text-muted-foreground">
                        {' · '}
                        {o.hint}
                      </span>
                      {o.live === false ? (
                        <span className="text-warning"> · {t('routes.ifaceDown')}</span>
                      ) : null}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null}
              {bypassOptions.length > 0 &&
              (connOptions.length > 0 ||
                xrayOptions.length > 0 ||
                ifaceOptions.length > 0) ? (
                <SelectSeparator />
              ) : null}
              {bypassOptions.length > 0 ? (
                <SelectGroup>
                  <SelectLabel className="flex items-center gap-1.5">
                    <ShieldCheck className="h-3.5 w-3.5" />
                    {t('routes.groupBypass')}
                  </SelectLabel>
                  {bypassOptions.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                      {o.hint ? (
                        <span className="text-muted-foreground">
                          {' · '}
                          {o.hint}
                        </span>
                      ) : null}
                    </SelectItem>
                  ))}
                </SelectGroup>
              ) : null}
            </SelectContent>
          </Select>
        )}
      </CardContent>
    </Card>
  )
}

function CatalogTab({
  target,
  hasTarget,
}: {
  target: string
  hasTarget: boolean
}) {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const { data: catalog, isLoading } = useQuery({
    queryKey: ['route-presets'],
    queryFn: api.routePresets,
    staleTime: 5 * 60_000,
  })

  const [query, setQuery] = React.useState('')
  const [selected, setSelected] = React.useState<Set<string>>(new Set())

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const createMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      const presets = catalog?.presets ?? []
      // Sequential so state.json mutations never race on the daemon.
      for (const id of ids) {
        const p = presets.find((x) => x.id === id)
        await api.createRoute({
          preset_id: id,
          name: p?.name,
          ...decodeTarget(target),
        })
      }
      return ids.length
    },
    onSuccess: (count) => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      setSelected(new Set())
      toast({
        variant: 'success',
        title: t('routes.created'),
        description: t('routes.selected', { count }),
      })
    },
    onError: () => toast({ variant: 'error', title: t('routes.createError') }),
  })

  const onCreate = () => {
    if (!hasTarget || !target) {
      toast({ variant: 'error', title: t('routes.selectTargetFirst') })
      return
    }
    if (selected.size === 0) return
    createMutation.mutate([...selected])
  }

  if (isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 9 }).map((_, i) => (
          <Skeleton key={i} className="h-[68px]" />
        ))}
      </div>
    )
  }

  const presets = catalog?.presets ?? []
  const categories = catalog?.categories ?? []
  const q = query.trim().toLowerCase()
  const filtered = q
    ? presets.filter(
        (p) =>
          p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q),
      )
    : presets

  return (
    <div className="space-y-5">
      {/* Search */}
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('routes.searchPlaceholder')}
          className="pl-9"
        />
      </div>

      {filtered.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t('routes.noResults', { query })}
        </p>
      ) : (
        categories.map((cat) => {
          const items = filtered.filter((p) => p.category === cat)
          if (items.length === 0) return null
          const meta = catMeta(cat)
          const CatIcon = meta.icon
          return (
            <section key={cat} className="space-y-2.5">
              <div className="flex items-center gap-2">
                <CatIcon className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold text-foreground">
                  {t(meta.labelKey)}
                </h3>
                <span className="text-xs text-muted-foreground">
                  {items.length}
                </span>
              </div>
              <div className="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
                {items.map((p) => (
                  <PresetCard
                    key={p.id}
                    preset={p}
                    selected={selected.has(p.id)}
                    onToggle={() => toggle(p.id)}
                  />
                ))}
              </div>
            </section>
          )
        })
      )}

      {/* Sticky selection action bar */}
      {selected.size > 0 ? (
        <div className="sticky bottom-4 z-10 flex items-center justify-between gap-3 rounded-lg border border-border bg-popover/95 px-4 py-3 shadow-lg backdrop-blur">
          <span className="text-sm font-medium text-foreground">
            {t('routes.selected', { count: selected.size })}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setSelected(new Set())}
            >
              {t('routes.clearSelection')}
            </Button>
            <Button
              size="sm"
              onClick={onCreate}
              disabled={createMutation.isPending}
              className="gap-1.5"
            >
              {createMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Plus className="h-4 w-4" />
              )}
              {t('routes.createSelectedN', { count: selected.size })}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function PresetCard({
  preset,
  selected,
  onToggle,
}: {
  preset: Preset
  selected: boolean
  onToggle: () => void
}) {
  const t = useT()
  const parts: string[] = []
  if (preset.has_subscription) parts.push(t('routes.remoteList'))
  if (preset.domain_count > 0)
    parts.push(t('routes.domains', { count: preset.domain_count }))
  if (preset.subnet_count > 0)
    parts.push(t('routes.subnets', { count: preset.subnet_count }))

  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={selected}
      className={cn(
        'group relative flex items-center gap-3 rounded-lg border p-3 text-left transition-colors',
        selected
          ? 'border-primary/60 bg-primary/5 ring-1 ring-inset ring-primary/20'
          : 'border-border bg-card hover:border-border/80 hover:bg-accent/40',
      )}
    >
      <ServiceTile
        name={preset.name}
        category={preset.category}
        className="h-9 w-9 shrink-0 text-sm"
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {preset.name}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {parts.join(' · ')}
        </p>
      </div>
      <span
        className={cn(
          'flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors',
          selected
            ? 'border-primary bg-primary text-primary-foreground'
            : 'border-border text-transparent group-hover:border-muted-foreground/50',
        )}
      >
        <Check className="h-3.5 w-3.5" />
      </span>
    </button>
  )
}

function ActiveRoutesTab() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const deleteConfirm = useConfirm<RouteEntry>()
  // Which route (if any) is open in the editor sheet.
  const [editId, setEditId] = React.useState<string | null>(null)

  const { data: routes, isLoading } = useQuery({
    queryKey: ['routes'],
    queryFn: api.routes,
    refetchInterval: 8000,
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['routes'] })
    queryClient.invalidateQueries({ queryKey: ['state'] })
  }

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.toggleRoute(id, enabled),
    onSuccess: () => {
      invalidate()
      toast({ variant: 'success', title: t('routes.toggled') })
    },
    onError: () => toast({ variant: 'error', title: t('routes.toggleError') }),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteRoute(id),
    onSuccess: () => {
      invalidate()
      deleteConfirm.close()
      toast({ variant: 'success', title: t('routes.deleted') })
    },
    onError: () => toast({ variant: 'error', title: t('routes.deleteError') }),
  })

  if (isLoading) {
    return (
      <div className="space-y-2.5">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-[68px]" />
        ))}
      </div>
    )
  }

  const list = routes ?? []
  if (list.length === 0) {
    return (
      <EmptyState
        icon={Waypoints}
        title={t('routes.emptyActiveTitle')}
        description={t('routes.emptyActiveDesc')}
      />
    )
  }

  return (
    <>
      <div className="space-y-2.5">
        {list.map((r) => (
          <RouteRow
            key={r.id}
            route={r}
            onToggle={(enabled) => toggleMutation.mutate({ id: r.id, enabled })}
            onDelete={() => deleteConfirm.ask(r)}
            onEdit={() => setEditId(r.id)}
          />
        ))}
      </div>

      <RouteEditorSheet
        id={editId}
        onOpenChange={(open) => {
          if (!open) setEditId(null)
        }}
      />

      <ConfirmDialog
        open={deleteConfirm.open}
        onOpenChange={deleteConfirm.setOpen}
        destructive
        title={t('routes.deleteTitle')}
        description={
          deleteConfirm.payload
            ? t('routes.deleteDesc', { name: deleteConfirm.payload.name })
            : undefined
        }
        confirmLabel={t('common.delete')}
        loading={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteConfirm.payload) deleteMutation.mutate(deleteConfirm.payload.id)
        }}
      />
    </>
  )
}

function RouteRow({
  route,
  onToggle,
  onDelete,
  onEdit,
}: {
  route: RouteEntry
  onToggle: (enabled: boolean) => void
  onDelete: () => void
  onEdit: () => void
}) {
  const t = useT()
  const counts: string[] = []
  if (route.domain_count > 0)
    counts.push(t('routes.domains', { count: route.domain_count }))
  if (route.subnet_count > 0)
    counts.push(t('routes.subnets', { count: route.subnet_count }))

  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        {/* The rule body is the primary click target — it opens the editor with
            the full domain list (the user asked to tap a rule to edit it). The
            switch and delete controls sit outside this button. */}
        <button
          type="button"
          onClick={onEdit}
          aria-label={t('routes.editRoute')}
          className="-m-1 flex min-w-0 flex-1 items-center gap-3 rounded-md p-1 text-left transition-colors hover:bg-accent/40"
        >
          <ServiceTile
            name={route.name}
            category={route.category}
            className="h-9 w-9 shrink-0 text-sm"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium text-foreground">
                {route.name}
              </span>
              {route.enabled ? (
                route.applied ? (
                  <Badge variant="success">{t('routes.applied')}</Badge>
                ) : (
                  <Badge variant="warning">{t('routes.pending')}</Badge>
                )
              ) : (
                <Badge variant="muted">{t('routes.notApplied')}</Badge>
              )}
            </div>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {counts.join(' · ')}
              {route.target_name ? (
                <>
                  {counts.length > 0 ? ' · ' : ''}
                  {t('routes.routeVia')}{' '}
                  <span className="text-foreground">{route.target_name}</span>
                  {route.target_iface ? (
                    <span className="font-mono"> ({route.target_iface})</span>
                  ) : null}
                </>
              ) : null}
            </p>
            {route.note ? (
              <p className="mt-1 text-xs text-warning">{route.note}</p>
            ) : null}
          </div>
        </button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onEdit}
          aria-label={t('routes.editRoute')}
          className="text-muted-foreground hover:text-foreground"
        >
          <Pencil className="h-4 w-4" />
        </Button>
        <Switch
          checked={route.enabled}
          onCheckedChange={onToggle}
          aria-label={route.name}
        />
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onDelete}
          aria-label={t('routes.deleteRoute')}
          className="text-muted-foreground hover:text-destructive"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </CardContent>
    </Card>
  )
}

/** Side-drawer editor for an existing route. Loads the rule's full membership
 * (GET /api/routes/{id}), lets the user edit the name, the domain list and the
 * subnet list, and saves via PUT /api/routes/{id} — which re-applies the rule
 * on the router with a clean teardown of the old form. */
function RouteEditorSheet({
  id,
  onOpenChange,
}: {
  id: string | null
  onOpenChange: (open: boolean) => void
}) {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const {
    data: detail,
    isLoading,
    isError,
  } = useQuery({
    queryKey: ['route', id],
    queryFn: () => api.getRoute(id as string),
    enabled: !!id,
  })

  const [name, setName] = React.useState('')
  const [domains, setDomains] = React.useState('')
  const [subnets, setSubnets] = React.useState('')

  // Seed the form each time a route's detail arrives.
  React.useEffect(() => {
    if (detail) {
      setName(detail.name)
      setDomains((detail.domains ?? []).join('\n'))
      setSubnets((detail.subnets ?? []).join('\n'))
    }
  }, [detail])

  const splitLines = (s: string) =>
    s
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#'))

  const dCount = splitLines(domains).length
  const sCount = splitLines(subnets).length

  const saveMutation = useMutation({
    mutationFn: () =>
      api.updateRoute(id as string, {
        name: name.trim() || undefined,
        domains: splitLines(domains),
        subnets: splitLines(subnets),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['route', id] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      toast({ variant: 'success', title: t('routes.editSaved') })
      onOpenChange(false)
    },
    onError: () => toast({ variant: 'error', title: t('routes.editError') }),
  })

  const onSave = () => {
    if (dCount === 0 && sCount === 0) {
      toast({ variant: 'error', title: t('routes.editEmpty') })
      return
    }
    saveMutation.mutate()
  }

  return (
    <Sheet open={!!id} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-lg"
      >
        <SheetHeader className="border-b border-border px-5 py-4">
          <SheetTitle>
            {detail
              ? t('routes.editTitle', { name: detail.name })
              : t('routes.editRoute')}
          </SheetTitle>
          <SheetDescription>{t('routes.editDesc')}</SheetDescription>
        </SheetHeader>

        {isLoading ? (
          <div className="flex items-center gap-2 p-5 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('routes.editLoading')}
          </div>
        ) : isError ? (
          <p className="p-5 text-sm text-destructive">
            {t('routes.editLoadError')}
          </p>
        ) : (
          <>
            <div className="flex-1 space-y-4 overflow-y-auto p-5">
              <div className="space-y-2">
                <Label htmlFor="edit-name">{t('routes.customName')}</Label>
                <Input
                  id="edit-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t('routes.customNamePlaceholder')}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-domains">
                  {t('routes.editDomainsLabel')}
                  <span className="ml-1 text-muted-foreground">· {dCount}</span>
                </Label>
                <Textarea
                  id="edit-domains"
                  value={domains}
                  onChange={(e) => setDomains(e.target.value)}
                  placeholder={t('routes.customDomainsPlaceholder')}
                  className="min-h-[240px] font-mono text-xs leading-relaxed"
                  spellCheck={false}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-subnets">
                  {t('routes.editSubnetsLabel')}
                  <span className="ml-1 text-muted-foreground">· {sCount}</span>
                </Label>
                <Textarea
                  id="edit-subnets"
                  value={subnets}
                  onChange={(e) => setSubnets(e.target.value)}
                  placeholder={t('routes.customSubnetsPlaceholder')}
                  className="min-h-[80px] font-mono text-xs leading-relaxed"
                  spellCheck={false}
                />
              </div>
              <p className="text-xs text-muted-foreground">
                {t('routes.chunkHint')}
              </p>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-border px-5 py-4">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button
                onClick={onSave}
                disabled={saveMutation.isPending}
                className="gap-1.5"
              >
                {saveMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Check className="h-4 w-4" />
                )}
                {t('routes.editSave')}
              </Button>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function CustomTab({
  target,
  hasTarget,
}: {
  target: string
  hasTarget: boolean
}) {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const [name, setName] = React.useState('')
  const [domains, setDomains] = React.useState('')
  const [subnets, setSubnets] = React.useState('')

  const splitLines = (s: string) =>
    s
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#'))

  const createMutation = useMutation({
    mutationFn: () =>
      api.createRoute({
        name: name.trim() || undefined,
        domains: splitLines(domains),
        subnets: splitLines(subnets),
        ...decodeTarget(target),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routes'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      setName('')
      setDomains('')
      setSubnets('')
      toast({ variant: 'success', title: t('routes.created') })
    },
    onError: () => toast({ variant: 'error', title: t('routes.createError') }),
  })

  const onCreate = () => {
    if (!hasTarget || !target) {
      toast({ variant: 'error', title: t('routes.selectTargetFirst') })
      return
    }
    if (splitLines(domains).length === 0 && splitLines(subnets).length === 0)
      return
    createMutation.mutate()
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <ListImporter
        onImported={(imported) =>
          setDomains((prev) => {
            const existing = new Set(splitLines(prev))
            const merged = [...splitLines(prev)]
            for (const d of imported) if (!existing.has(d)) merged.push(d)
            return merged.join('\n')
          })
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>{t('routes.customTitle')}</CardTitle>
          <p className="text-xs text-muted-foreground">{t('routes.chunkHint')}</p>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="route-name">{t('routes.customName')}</Label>
            <Input
              id="route-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('routes.customNamePlaceholder')}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="route-domains">{t('routes.customDomains')}</Label>
            <Textarea
              id="route-domains"
              value={domains}
              onChange={(e) => setDomains(e.target.value)}
              placeholder={t('routes.customDomainsPlaceholder')}
              className="min-h-[140px] font-mono text-xs leading-relaxed"
              spellCheck={false}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="route-subnets">{t('routes.customSubnets')}</Label>
            <Textarea
              id="route-subnets"
              value={subnets}
              onChange={(e) => setSubnets(e.target.value)}
              placeholder={t('routes.customSubnetsPlaceholder')}
              className="min-h-[80px] font-mono text-xs leading-relaxed"
              spellCheck={false}
            />
          </div>
          <Button
            onClick={onCreate}
            disabled={createMutation.isPending}
            className="w-full gap-1.5"
          >
            {createMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Plus className="h-4 w-4" />
            )}
            {t('routes.createCustom')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

/** Fetches a remote domain-list URL (v2fly / plain / hosts) and hands the
 * flattened domains to the parent so they can be added to a route or list. */
function ListImporter({ onImported }: { onImported: (domains: string[]) => void }) {
  const t = useT()
  const { toast } = useToast()
  const [url, setUrl] = React.useState('')
  const [attr, setAttr] = React.useState('')

  const importMutation = useMutation({
    mutationFn: () => api.resolveList(url.trim(), attr.trim() || undefined),
    onSuccess: (res) => {
      if (!res.domains || res.domains.length === 0) {
        toast({ variant: 'error', title: t('routes.importEmpty') })
        return
      }
      onImported(res.domains)
      const bits = [
        t('routes.imported', {
          count: res.domains.length,
          sources: res.sources?.length ?? 1,
        }),
        t('routes.appendToDomains'),
      ]
      if (res.skipped_n > 0)
        bits.push(t('routes.importedSkipped', { count: res.skipped_n }))
      if (res.truncated) bits.push(t('routes.importTruncated'))
      toast({
        variant: 'success',
        title: t('routes.imported', {
          count: res.domains.length,
          sources: res.sources?.length ?? 1,
        }),
        description: bits.slice(1).join(' '),
      })
    },
    onError: () => toast({ variant: 'error', title: t('routes.importError') }),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('routes.importTitle')}</CardTitle>
        <p className="text-xs text-muted-foreground">{t('routes.importDesc')}</p>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="import-url">URL</Label>
          <Input
            id="import-url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t('routes.importUrlPlaceholder')}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="import-attr">{t('routes.importAttr')}</Label>
          <Input
            id="import-attr"
            value={attr}
            onChange={(e) => setAttr(e.target.value)}
            placeholder={t('routes.importAttrPlaceholder')}
          />
        </div>
        <Button
          variant="outline"
          onClick={() => importMutation.mutate()}
          disabled={!url.trim() || importMutation.isPending}
          className="w-full gap-1.5"
        >
          {importMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Download className="h-4 w-4" />
          )}
          {importMutation.isPending ? t('routes.importing') : t('routes.importBtn')}
        </Button>
        <Separator />
        <p className="text-xs text-muted-foreground">{t('routes.importDesc')}</p>
      </CardContent>
    </Card>
  )
}
