import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  FileText,
  Link2,
  Loader2,
  Play,
  RefreshCw,
  RotateCw,
  Save,
  Search,
  ShieldCheck,
  ShieldOff,
  SlidersHorizontal,
  Square,
  XCircle,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
import { EmptyState } from '@/components/EmptyState'
import { StatusDot } from '@/components/StatusDot'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useT } from '@/i18n'
import type { DomainCheck, NfqwsConf, NfqwsImportResult, NfqwsMode } from '@/lib/types'

// nfqws2 mode macro <-> Select value. The structured conf stores the active
// mode as a macro in NFQWS_EXTRA_ARGS (e.g. "$MODE_AUTO").
const MODE_MACROS: Record<NfqwsMode, string> = {
  MODE_AUTO: '$MODE_AUTO',
  MODE_LIST: '$MODE_LIST',
  MODE_ALL: '$MODE_ALL',
}
function macroToMode(macro: string): NfqwsMode {
  const m = (macro || '').toUpperCase()
  if (m.includes('MODE_LIST')) return 'MODE_LIST'
  if (m.includes('MODE_ALL')) return 'MODE_ALL'
  return 'MODE_AUTO'
}

type NfqwsServiceAction = 'start' | 'stop' | 'restart' | 'reload' | 'install'

export function BypassPage() {
  const t = useT()
  const { data: nfqws, isLoading } = useQuery({
    queryKey: ['nfqws'],
    queryFn: api.nfqws,
    refetchInterval: 8000,
  })

  return (
    <div className="space-y-6">
      <PageHeader title={t('bypass.title')} description={t('bypass.desc')} />

      {isLoading ? (
        <Skeleton className="h-28" />
      ) : (
        <ServiceControlCard
          installed={!!nfqws?.installed}
          running={!!nfqws?.running}
          version={nfqws?.version}
          mode={nfqws?.mode}
          kernelReady={nfqws?.kernel_ready}
          missingModules={nfqws?.missing_modules}
        />
      )}

      <RoutableBypassCard />

      <Tabs defaultValue="config">
        <TabsList>
          <TabsTrigger value="config">{t('bypass.tabConfig')}</TabsTrigger>
          <TabsTrigger value="hostlists">{t('bypass.tabHostlists')}</TabsTrigger>
          <TabsTrigger value="check">{t('bypass.tabCheck')}</TabsTrigger>
        </TabsList>

        <TabsContent value="config">
          <ConfigSection />
        </TabsContent>
        <TabsContent value="hostlists">
          <HostlistsManager />
        </TabsContent>
        <TabsContent value="check">
          <DomainChecker />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// RoutableBypassCard exposes DPI bypass as ONE routable interface (the user's
// P1 request: nfqws "like Xray"). keen-manager runs a local tpws SOCKS proxy
// and registers a managed KeeneticOS Proxy interface pointing at it; chosen
// domains are then routed through it from the Routes page (target "DPI Bypass")
// — not a global inline NFQUEUE. The desync strategy is edited here; only the
// domains are picked on the Routes page (one shared source of domains).
function RoutableBypassCard() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const { data: bypass, isLoading } = useQuery({
    queryKey: ['bypass'],
    queryFn: api.bypass,
    refetchInterval: 8000,
  })

  const [strategy, setStrategy] = React.useState('')
  const [port, setPort] = React.useState('')
  const [dirty, setDirty] = React.useState(false)
  React.useEffect(() => {
    if (bypass && !dirty) {
      setStrategy(bypass.strategy ?? '')
      setPort(bypass.port ? String(bypass.port) : '')
    }
  }, [bypass, dirty])

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['bypass'] })
    queryClient.invalidateQueries({ queryKey: ['routes'] })
    queryClient.invalidateQueries({ queryKey: ['interfaces'] })
  }

  const toggleMutation = useMutation({
    mutationFn: (enabled: boolean) => api.saveBypass({ enabled }),
    onSuccess: (v) => {
      invalidate()
      toast({
        variant: 'success',
        title: v.enabled
          ? t('bypass.routableEnabled')
          : t('bypass.routableDisabled'),
      })
    },
    onError: (err: Error) =>
      toast({
        variant: 'error',
        title: t('bypass.routableError'),
        description: err.message,
      }),
  })

  const saveMutation = useMutation({
    mutationFn: () =>
      api.saveBypass({ strategy: strategy.trim(), port: Number(port) || 0 }),
    onSuccess: () => {
      setDirty(false)
      invalidate()
      toast({ variant: 'success', title: t('bypass.strategySaved') })
    },
    onError: (err: Error) =>
      toast({
        variant: 'error',
        title: t('bypass.routableError'),
        description: err.message,
      }),
  })

  if (isLoading) return <Skeleton className="h-40" />

  const enabled = !!bypass?.enabled
  const installed = !!bypass?.installed
  const running = !!bypass?.running

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div className="min-w-0 space-y-1">
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-4 w-4 text-emerald-400" />
            {t('bypass.routableTitle')}
          </CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('bypass.routableDesc')}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2 pl-3">
          <Label htmlFor="bypass-enabled" className="text-xs text-muted-foreground">
            {enabled ? t('bypass.on') : t('bypass.off')}
          </Label>
          <Switch
            id="bypass-enabled"
            checked={enabled}
            disabled={toggleMutation.isPending}
            onCheckedChange={(v) => toggleMutation.mutate(v)}
          />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={installed ? 'success' : 'warning'}>
            {installed ? t('bypass.tpwsInstalled') : t('bypass.tpwsMissing')}
          </Badge>
          {enabled ? (
            <Badge variant={running ? 'success' : 'muted'}>
              {running ? t('bypass.running') : t('bypass.stopped')}
            </Badge>
          ) : null}
          {bypass?.interface ? (
            <Badge variant="secondary" className="font-mono">
              {bypass.interface} · 127.0.0.1:{bypass.port}
            </Badge>
          ) : null}
        </div>

        {bypass?.note ? (
          <p className="text-xs text-muted-foreground">{bypass.note}</p>
        ) : null}

        <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
          <div className="space-y-1.5">
            <Label htmlFor="bypass-strategy">{t('bypass.strategyLabel')}</Label>
            <Input
              id="bypass-strategy"
              value={strategy}
              spellCheck={false}
              className="font-mono text-xs"
              placeholder="--split-tls=sni --disorder"
              onChange={(e) => {
                setStrategy(e.target.value)
                setDirty(true)
              }}
            />
            <p className="text-xs text-muted-foreground">
              {t('bypass.strategyHint')}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bypass-port">{t('bypass.portLabel')}</Label>
            <Input
              id="bypass-port"
              value={port}
              inputMode="numeric"
              className="font-mono text-xs"
              placeholder="10809"
              onChange={(e) => {
                setPort(e.target.value.replace(/[^0-9]/g, ''))
                setDirty(true)
              }}
            />
          </div>
        </div>

        <div className="flex items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">
            {t('bypass.routableRoutesHint')}
          </p>
          <Button
            size="sm"
            disabled={!dirty || saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            {saveMutation.isPending ? (
              <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Save className="mr-1.5 h-3.5 w-3.5" />
            )}
            {t('bypass.saveStrategy')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

function ServiceControlCard({
  installed,
  running,
  version,
  mode,
  kernelReady,
  missingModules,
}: {
  installed: boolean
  running: boolean
  version?: string
  mode?: NfqwsMode
  kernelReady?: boolean
  missingModules?: string[]
}) {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const [pending, setPending] = React.useState<NfqwsServiceAction | null>(null)

  // A running daemon whose NFQUEUE kernel modules are absent is up but inert —
  // surface it prominently so "running" isn't mistaken for "working".
  const kernelInert = installed && running && kernelReady === false

  const actionMutation = useMutation({
    mutationFn: (action: NfqwsServiceAction) => api.nfqwsAction(action),
    onMutate: (action) => setPending(action),
    onSuccess: (_d, action) => {
      queryClient.invalidateQueries({ queryKey: ['nfqws'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      const titles: Record<NfqwsServiceAction, string> = {
        start: t('bypass.starting'),
        stop: t('bypass.stopping'),
        restart: t('bypass.restarting'),
        reload: t('bypass.reloading'),
        install: t('bypass.installing'),
      }
      toast({ variant: 'success', title: titles[action] })
    },
    onError: () =>
      toast({ variant: 'error', title: t('bypass.actionError') }),
    onSettled: () => setPending(null),
  })

  const busy = (a: NfqwsServiceAction) =>
    actionMutation.isPending && pending === a

  return (
    <Card>
      <CardContent className="space-y-4 p-5">
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="flex items-center gap-3">
          <div
            className={cn(
              'flex h-11 w-11 items-center justify-center rounded-lg',
              !installed
                ? 'bg-muted text-muted-foreground'
                : running
                  ? 'bg-success/15 text-success'
                  : 'bg-warning/15 text-warning',
            )}
          >
            {installed ? (
              <StatusDot
                status={running ? 'up' : 'down'}
                className="h-3 w-3"
              />
            ) : (
              <ShieldOff className="h-5 w-5" />
            )}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold text-foreground">nfqws2</h3>
              {!installed ? (
                <Badge variant="muted">{t('bypass.notInstalled')}</Badge>
              ) : running ? (
                <Badge variant="success">{t('bypass.running')}</Badge>
              ) : (
                <Badge variant="warning">{t('bypass.stopped')}</Badge>
              )}
            </div>
            <p className="mt-0.5 font-mono text-xs text-muted-foreground">
              {version ?? (installed ? t('bypass.versionUnknown') : t('bypass.notPresent'))}
              {mode ? ` · ${mode}` : ''}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {!installed ? (
            <Button
              size="sm"
              onClick={() => actionMutation.mutate('install')}
              disabled={busy('install')}
              className="gap-1.5"
            >
              {busy('install') ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Download className="h-3.5 w-3.5" />
              )}
              {t('bypass.install')}
            </Button>
          ) : (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => actionMutation.mutate('start')}
                disabled={running || busy('start')}
                className="gap-1.5"
              >
                {busy('start') ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Play className="h-3.5 w-3.5" />
                )}
                {t('bypass.start')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => actionMutation.mutate('stop')}
                disabled={!running || busy('stop')}
                className="gap-1.5"
              >
                {busy('stop') ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Square className="h-3.5 w-3.5" />
                )}
                {t('bypass.stop')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => actionMutation.mutate('restart')}
                disabled={busy('restart')}
                className="gap-1.5"
              >
                {busy('restart') ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RotateCw className="h-3.5 w-3.5" />
                )}
                {t('bypass.restart')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => actionMutation.mutate('reload')}
                disabled={busy('reload')}
                className="gap-1.5"
              >
                {busy('reload') ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
                {t('bypass.reload')}
              </Button>
            </>
          )}
        </div>
        </div>

        {kernelInert ? (
          <div className="flex items-start gap-2.5 rounded-md border border-warning/30 bg-warning/5 px-3 py-2.5">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">
                {t('bypass.kernelMissing')}
              </p>
              <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                {t('bypass.kernelMissingDesc', {
                  modules: (missingModules ?? []).join(', ') || 'nfnetlink_queue, xt_NFQUEUE',
                })}
              </p>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

// ConfigSection wraps the typed form and the raw editor in sub-tabs: the
// structured Form is primary; the raw nfqws2.conf editor is kept under
// "Advanced" for power users (parity with nfqws-keenetic-web).
function ConfigSection() {
  const t = useT()
  return (
    <Tabs defaultValue="form" className="space-y-4">
      <TabsList>
        <TabsTrigger value="form" className="gap-1.5">
          <SlidersHorizontal className="h-3.5 w-3.5" />
          {t('bypass.subForm')}
        </TabsTrigger>
        <TabsTrigger value="raw" className="gap-1.5">
          <FileText className="h-3.5 w-3.5" />
          {t('bypass.subAdvanced')}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="form">
        <StructuredConfigEditor />
      </TabsContent>
      <TabsContent value="raw">
        <ConfigEditor />
      </TabsContent>
    </Tabs>
  )
}

function StructuredConfigEditor() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const { data: conf, isLoading } = useQuery({
    queryKey: ['nfqws-config-structured'],
    queryFn: api.nfqwsConfigStructured,
  })

  const [form, setForm] = React.useState<NfqwsConf | null>(null)
  const [dirty, setDirty] = React.useState(false)

  React.useEffect(() => {
    if (conf) {
      setForm(conf)
      setDirty(false)
    }
  }, [conf])

  const set = <K extends keyof NfqwsConf>(key: K, value: NfqwsConf[K]) => {
    setForm((prev) => (prev ? { ...prev, [key]: value } : prev))
    setDirty(true)
  }

  const saveMutation = useMutation({
    mutationFn: () => api.saveNfqwsConfigStructured(form ?? {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nfqws-config-structured'] })
      queryClient.invalidateQueries({ queryKey: ['nfqws-config'] })
      queryClient.invalidateQueries({ queryKey: ['nfqws'] })
      setDirty(false)
      toast({
        variant: 'success',
        title: t('bypass.configSaved'),
        description: t('bypass.configSavedDesc'),
      })
    },
    onError: () => toast({ variant: 'error', title: t('bypass.configSaveError') }),
  })

  if (isLoading || !form) {
    return <Skeleton className="h-96" />
  }

  const modeLabels: Record<NfqwsMode, string> = {
    MODE_AUTO: t('bypass.modeAuto'),
    MODE_LIST: t('bypass.modeList'),
    MODE_ALL: t('bypass.modeAll'),
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle>{t('bypass.configTitle')}</CardTitle>
          <p className="text-xs text-muted-foreground">{t('bypass.formHint')}</p>
        </div>
        <Button
          size="sm"
          onClick={() => saveMutation.mutate()}
          disabled={!dirty || saveMutation.isPending}
          className="gap-1.5"
        >
          {saveMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t('common.save')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Mode & ports */}
        <section className="space-y-4">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t('bypass.sectionMode')}
          </h4>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2 sm:col-span-3 sm:max-w-md">
              <Label>{t('bypass.modeLabel')}</Label>
              <Select
                value={macroToMode(form.nfqws_extra_args)}
                onValueChange={(v) =>
                  set('nfqws_extra_args', MODE_MACROS[v as NfqwsMode])
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(modeLabels) as NfqwsMode[]).map((m) => (
                    <SelectItem key={m} value={m}>
                      {modeLabels[m]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <FieldInput
              label={t('bypass.fieldTcpPorts')}
              value={form.tcp_ports}
              onChange={(v) => set('tcp_ports', v)}
              placeholder="80,443"
              mono
            />
            <FieldInput
              label={t('bypass.fieldUdpPorts')}
              value={form.udp_ports}
              onChange={(v) => set('udp_ports', v)}
              placeholder="443,50000-50100"
              mono
            />
            <FieldInput
              label={t('bypass.fieldIspInterface')}
              value={form.isp_interface}
              onChange={(v) => set('isp_interface', v)}
              placeholder={t('bypass.fieldIspInterfacePlaceholder')}
              mono
            />
          </div>
        </section>

        <Separator />

        {/* Strategy args */}
        <section className="space-y-4">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t('bypass.sectionStrategy')}
          </h4>
          <FieldArgs
            label={t('bypass.fieldArgsTcp')}
            value={form.nfqws_args}
            onChange={(v) => set('nfqws_args', v)}
            placeholder={t('bypass.argsPlaceholder')}
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <FieldArgs
              label={t('bypass.fieldArgsQuic')}
              value={form.nfqws_args_quic}
              onChange={(v) => set('nfqws_args_quic', v)}
            />
            <FieldArgs
              label={t('bypass.fieldArgsUdp')}
              value={form.nfqws_args_udp}
              onChange={(v) => set('nfqws_args_udp', v)}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <FieldArgs
              label={t('bypass.fieldBaseArgs')}
              value={form.nfqws_base_args}
              onChange={(v) => set('nfqws_base_args', v)}
            />
            <FieldArgs
              label={t('bypass.fieldCustomArgs')}
              value={form.nfqws_args_custom}
              onChange={(v) => set('nfqws_args_custom', v)}
            />
          </div>
        </section>

        <Separator />

        {/* Advanced */}
        <section className="space-y-4">
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t('bypass.sectionAdvanced')}
          </h4>
          <div className="grid gap-4 sm:grid-cols-3">
            <FieldInput
              label={t('bypass.fieldPolicyName')}
              value={form.policy_name}
              onChange={(v) => set('policy_name', v)}
              mono
            />
            <FieldNumber
              label={t('bypass.fieldNfqueue')}
              value={form.nfqueue_num}
              onChange={(v) => set('nfqueue_num', v)}
            />
            <FieldNumber
              label={t('bypass.fieldLogLevel')}
              value={form.log_level}
              onChange={(v) => set('log_level', v)}
            />
          </div>
          <div className="flex items-center justify-between gap-4 rounded-md border border-border/70 px-3 py-2.5">
            <div className="space-y-0.5">
              <Label>{t('bypass.fieldIpv6')}</Label>
              <p className="text-xs text-muted-foreground">
                {t('bypass.fieldIpv6Desc')}
              </p>
            </div>
            <Switch
              checked={form.ipv6_enabled}
              onCheckedChange={(v) => set('ipv6_enabled', v)}
              aria-label={t('bypass.fieldIpv6')}
            />
          </div>
        </section>
      </CardContent>
    </Card>
  )
}

function FieldInput({
  label,
  value,
  onChange,
  placeholder,
  mono,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        className={mono ? 'font-mono text-xs' : undefined}
      />
    </div>
  )
}

function FieldNumber({
  label,
  value,
  onChange,
}: {
  label: string
  value: number
  onChange: (v: number) => void
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Input
        type="number"
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="tabular-nums"
      />
    </div>
  )
}

function FieldArgs({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        className="min-h-[72px] font-mono text-xs leading-relaxed"
      />
    </div>
  )
}

function ConfigEditor() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const { data: config, isLoading } = useQuery({
    queryKey: ['nfqws-config'],
    queryFn: api.nfqwsConfig,
  })

  const [raw, setRaw] = React.useState('')
  const [mode, setMode] = React.useState<NfqwsMode>('MODE_AUTO')
  const [dirty, setDirty] = React.useState(false)

  React.useEffect(() => {
    if (config) {
      setRaw(config.raw)
      setMode(config.mode)
      setDirty(false)
    }
  }, [config])

  const saveMutation = useMutation({
    mutationFn: () => api.saveNfqwsConfig({ raw, mode }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nfqws-config'] })
      queryClient.invalidateQueries({ queryKey: ['nfqws'] })
      setDirty(false)
      toast({
        variant: 'success',
        title: t('bypass.configSaved'),
        description: t('bypass.configSavedDesc'),
      })
    },
    onError: () => toast({ variant: 'error', title: t('bypass.configSaveError') }),
  })

  if (isLoading) {
    return <Skeleton className="h-96" />
  }

  const modeLabels: Record<NfqwsMode, string> = {
    MODE_AUTO: t('bypass.modeAuto'),
    MODE_LIST: t('bypass.modeList'),
    MODE_ALL: t('bypass.modeAll'),
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle>{t('bypass.configTitle')}</CardTitle>
          <p className="text-xs text-muted-foreground">
            {t('bypass.configHint')}
          </p>
        </div>
        <Button
          size="sm"
          onClick={() => saveMutation.mutate()}
          disabled={!dirty || saveMutation.isPending}
          className="gap-1.5"
        >
          {saveMutation.isPending ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t('common.save')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-2 sm:max-w-md">
          <Label htmlFor="nfqws-mode">{t('bypass.modeLabel')}</Label>
          <Select
            value={mode}
            onValueChange={(v) => {
              setMode(v as NfqwsMode)
              setDirty(true)
            }}
          >
            <SelectTrigger id="nfqws-mode">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(Object.keys(modeLabels) as NfqwsMode[]).map((m) => (
                <SelectItem key={m} value={m}>
                  {modeLabels[m]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="nfqws-raw">{t('bypass.rawConfigLabel')}</Label>
          <Textarea
            id="nfqws-raw"
            value={raw}
            onChange={(e) => {
              setRaw(e.target.value)
              setDirty(true)
            }}
            spellCheck={false}
            className="min-h-[340px] font-mono text-xs leading-relaxed"
          />
        </div>
      </CardContent>
    </Card>
  )
}

function HostlistsManager() {
  const t = useT()
  const queryClient = useQueryClient()
  const { toast } = useToast()

  const { data: lists, isLoading } = useQuery({
    queryKey: ['nfqws-lists'],
    queryFn: api.nfqwsLists,
  })

  const [selected, setSelected] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (lists && lists.length > 0 && selected === null) {
      setSelected(lists[0])
    }
  }, [lists, selected])

  const { data: listData, isLoading: listLoading } = useQuery({
    queryKey: ['nfqws-list', selected],
    queryFn: () => api.nfqwsList(selected as string),
    enabled: !!selected,
  })

  const [content, setContent] = React.useState('')
  const [dirty, setDirty] = React.useState(false)
  const [importOpen, setImportOpen] = React.useState(false)

  React.useEffect(() => {
    if (listData) {
      setContent(listData.content)
      setDirty(false)
    }
  }, [listData])

  // After a server-side import the hostlist family is already written (and
  // split across user.list / user2.list / …), so refresh the index and open the
  // base file rather than merging text client-side.
  const handleImported = (res: NfqwsImportResult) => {
    queryClient.invalidateQueries({ queryKey: ['nfqws-lists'] })
    queryClient.invalidateQueries({ queryKey: ['nfqws-list', res.base] })
    queryClient.invalidateQueries({ queryKey: ['nfqws'] })
    queryClient.invalidateQueries({ queryKey: ['state'] })
    setSelected(res.base)
    setDirty(false)
  }

  const saveMutation = useMutation({
    mutationFn: () => api.saveNfqwsList(selected as string, content),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nfqws-list', selected] })
      setDirty(false)
      toast({
        variant: 'success',
        title: t('bypass.hostlistSaved'),
        description: t('bypass.hostlistSavedDesc', { name: selected ?? '' }),
      })
    },
    onError: () => toast({ variant: 'error', title: t('bypass.hostlistSaveError') }),
  })

  if (isLoading) {
    return <Skeleton className="h-96" />
  }

  if (!lists || lists.length === 0) {
    return (
      <EmptyState
        icon={FileText}
        title={t('bypass.emptyListsTitle')}
        description={t('bypass.emptyListsDesc')}
      />
    )
  }

  const entryCount = content
    .split('\n')
    .filter((l) => l.trim() && !l.trim().startsWith('#')).length

  return (
    <div className="grid gap-4 lg:grid-cols-[220px_1fr]">
      {/* List selector */}
      <Card className="h-fit">
        <CardContent className="p-2">
          <nav className="space-y-0.5">
            {lists.map((name) => (
              <button
                key={name}
                type="button"
                onClick={() => setSelected(name)}
                className={cn(
                  'flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors',
                  selected === name
                    ? 'bg-accent font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
                )}
              >
                <FileText className="h-4 w-4 shrink-0" />
                <span className="truncate font-mono text-xs">{name}</span>
              </button>
            ))}
          </nav>
        </CardContent>
      </Card>

      {/* Editor */}
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div className="space-y-1">
            <CardTitle className="font-mono">{selected}</CardTitle>
            <p className="text-xs text-muted-foreground">
              {t('bypass.activeEntries', { count: entryCount })}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setImportOpen(true)}
              className="gap-1.5"
            >
              <Link2 className="h-3.5 w-3.5" />
              {t('bypass.importList')}
            </Button>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate()}
              disabled={!dirty || saveMutation.isPending}
              className="gap-1.5"
            >
              {saveMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              {t('common.save')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {listLoading ? (
            <Skeleton className="h-80" />
          ) : (
            <Textarea
              value={content}
              onChange={(e) => {
                setContent(e.target.value)
                setDirty(true)
              }}
              spellCheck={false}
              className="min-h-[360px] font-mono text-xs leading-relaxed"
              placeholder={t('bypass.hostlistPlaceholder')}
            />
          )}
        </CardContent>
      </Card>

      <ImportListDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        listName={selected ?? ''}
        onImported={handleImported}
      />
    </div>
  )
}

// ImportListDialog fetches a remote domain-list URL (v2fly / plain / hosts),
// flattening include: directives and @attribute tags server-side, then writes
// the result into the hostlist family — auto-splitting a large set across
// numbered files (user.list, user2.list, …) of up to 300 domains each.
function ImportListDialog({
  open,
  onOpenChange,
  listName,
  onImported,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  listName: string
  onImported: (res: NfqwsImportResult) => void
}) {
  const t = useT()
  const { toast } = useToast()
  const [url, setUrl] = React.useState('')
  const [attr, setAttr] = React.useState('')
  const [mode, setMode] = React.useState<'append' | 'replace'>('append')

  const importMutation = useMutation({
    mutationFn: () =>
      api.importNfqwsList(listName, url.trim(), attr.trim() || undefined, mode),
    onSuccess: (res) => {
      if (!res.files || res.files.length === 0 || res.total === 0) {
        toast({ variant: 'error', title: t('bypass.importNoDomains') })
        return
      }
      onImported(res)
      const extra: string[] = [
        t('bypass.importDoneFiles', {
          names: res.files.map((f) => f.name).join(', '),
        }),
      ]
      if (res.skipped_n > 0)
        extra.push(t('bypass.importDoneSkipped', { count: res.skipped_n }))
      if (res.truncated) extra.push(t('bypass.importDoneTrunc'))
      toast({
        variant: 'success',
        title: t('bypass.importDone', {
          count: res.total,
          files: res.files.length,
        }),
        description: extra.join(' ') || undefined,
      })
      setUrl('')
      setAttr('')
      onOpenChange(false)
    },
    onError: () => toast({ variant: 'error', title: t('bypass.importFailed') }),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('bypass.importListTitle', { name: listName })}</DialogTitle>
          <DialogDescription>{t('bypass.importListDesc')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="import-list-url">{t('bypass.importUrlLabel')}</Label>
            <Input
              id="import-list-url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={t('bypass.importUrlPlaceholder')}
              className="font-mono text-xs"
              spellCheck={false}
              autoFocus
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="import-list-attr">{t('bypass.importAttrLabel')}</Label>
              <Input
                id="import-list-attr"
                value={attr}
                onChange={(e) => setAttr(e.target.value)}
                placeholder={t('bypass.importAttrPlaceholder')}
              />
            </div>
            <div className="space-y-2">
              <Label>{t('bypass.modeLabel')}</Label>
              <Select
                value={mode}
                onValueChange={(v) => setMode(v as 'append' | 'replace')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="append">
                    {t('bypass.importModeAppend')}
                  </SelectItem>
                  <SelectItem value="replace">
                    {t('bypass.importModeReplace')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('bypass.importChunkNote')}
          </p>
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => importMutation.mutate()}
            disabled={!url.trim() || importMutation.isPending}
            className="gap-1.5"
          >
            {importMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Download className="h-4 w-4" />
            )}
            {importMutation.isPending ? t('bypass.importRunning') : t('bypass.importRun')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DomainChecker() {
  const t = useT()
  const { toast } = useToast()
  const [domain, setDomain] = React.useState('')
  const [result, setResult] = React.useState<DomainCheck | null>(null)

  const checkMutation = useMutation({
    mutationFn: (d: string) => api.checkDomain(d),
    onSuccess: (res) => setResult(res),
    onError: () => toast({ variant: 'error', title: t('bypass.checkError') }),
  })

  const onCheck = () => {
    const d = domain.trim()
    if (!d) return
    setResult(null)
    checkMutation.mutate(d)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('bypass.checkTitle')}</CardTitle>
        <p className="text-xs text-muted-foreground">{t('bypass.checkHint')}</p>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onCheck()
              }}
              placeholder={t('bypass.domainPlaceholder')}
              className="pl-9 font-mono text-sm"
              spellCheck={false}
            />
          </div>
          <Button
            onClick={onCheck}
            disabled={!domain.trim() || checkMutation.isPending}
            className="gap-1.5"
          >
            {checkMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Search className="h-4 w-4" />
            )}
            {t('bypass.check')}
          </Button>
        </div>

        {result ? (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <ReachabilityTile
                label={t('bypass.directPath')}
                ok={result.direct_ok}
                okText={t('bypass.reachableDirect')}
                failText={t('bypass.blockedUnreachable')}
              />
              <ReachabilityTile
                label={t('bypass.throughBypass')}
                ok={result.bypass_ok}
                okText={t('bypass.reachableViaBypass')}
                failText={t('bypass.stillUnreachable')}
              />
            </div>
            {result.note ? (
              <div className="rounded-md border border-border/70 bg-muted/40 px-3 py-2.5">
                <p className="text-xs leading-relaxed text-muted-foreground">
                  <span className="font-medium text-foreground">
                    {t('bypass.note')}{' '}
                  </span>
                  {result.note}
                </p>
              </div>
            ) : null}
          </div>
        ) : (
          <div className="rounded-md border border-dashed border-border/70 px-4 py-8 text-center">
            <p className="text-xs text-muted-foreground">
              {t('bypass.checkEmptyHint')}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ReachabilityTile({
  label,
  ok,
  okText,
  failText,
}: {
  label: string
  ok: boolean
  okText: string
  failText: string
}) {
  return (
    <div
      className={cn(
        'flex items-center gap-3 rounded-md border px-4 py-3',
        ok
          ? 'border-success/30 bg-success/5'
          : 'border-destructive/30 bg-destructive/5',
      )}
    >
      {ok ? (
        <CheckCircle2 className="h-5 w-5 shrink-0 text-success" />
      ) : (
        <XCircle className="h-5 w-5 shrink-0 text-destructive" />
      )}
      <div className="min-w-0">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p
          className={cn(
            'text-sm font-medium',
            ok ? 'text-success' : 'text-destructive',
          )}
        >
          {ok ? okText : failText}
        </p>
      </div>
    </div>
  )
}
