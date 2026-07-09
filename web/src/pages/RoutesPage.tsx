import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Cloud,
  Code2,
  Download,
  Gamepad2,
  Loader2,
  Network,
  PlayCircle,
  Plus,
  Search,
  ShieldBan,
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

  // Connection targets: only native-interface connections (AmneziaWG) can back a
  // router-native dns-proxy route. Xray carries traffic transparently.
  const connOptions = React.useMemo<TargetOption[]>(
    () =>
      (connections ?? [])
        .filter((c) => c.type === 'awg')
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

  const allValues = React.useMemo(
    () => [...connOptions, ...ifaceOptions].map((o) => o.value),
    [connOptions, ifaceOptions],
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
        ifaceOptions={ifaceOptions}
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
  ifaceOptions,
  value,
  onChange,
  dnsAvailable,
  note,
}: {
  connOptions: TargetOption[]
  ifaceOptions: TargetOption[]
  value: string
  onChange: (v: string) => void
  dnsAvailable: boolean
  note?: string
}) {
  const t = useT()
  const hasAny = connOptions.length > 0 || ifaceOptions.length > 0
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
          {!dnsAvailable ? (
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
              {connOptions.length > 0 && ifaceOptions.length > 0 ? (
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
          />
        ))}
      </div>

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
}: {
  route: RouteEntry
  onToggle: (enabled: boolean) => void
  onDelete: () => void
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
