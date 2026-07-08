import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CheckCircle2,
  Download,
  FileText,
  Loader2,
  Play,
  RefreshCw,
  RotateCw,
  Save,
  Search,
  ShieldOff,
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
import type { DomainCheck, NfqwsMode } from '@/lib/types'

const MODE_LABELS: Record<NfqwsMode, string> = {
  MODE_AUTO: 'Auto (learned + user lists)',
  MODE_LIST: 'List (hostlists only)',
  MODE_ALL: 'All traffic',
}

type NfqwsServiceAction = 'start' | 'stop' | 'restart' | 'reload' | 'install'

export function BypassPage() {
  const { data: nfqws, isLoading } = useQuery({
    queryKey: ['nfqws'],
    queryFn: api.nfqws,
    refetchInterval: 8000,
  })

  return (
    <div className="space-y-6">
      <PageHeader
        title="DPI Bypass"
        description="nfqws2 deep-packet-inspection circumvention — service, strategy, and hostlists."
      />

      {isLoading ? (
        <Skeleton className="h-28" />
      ) : (
        <ServiceControlCard installed={!!nfqws?.installed} running={!!nfqws?.running} version={nfqws?.version} mode={nfqws?.mode} />
      )}

      <Tabs defaultValue="config">
        <TabsList>
          <TabsTrigger value="config">Config</TabsTrigger>
          <TabsTrigger value="hostlists">Hostlists</TabsTrigger>
          <TabsTrigger value="check">Domain check</TabsTrigger>
        </TabsList>

        <TabsContent value="config">
          <ConfigEditor />
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

function ServiceControlCard({
  installed,
  running,
  version,
  mode,
}: {
  installed: boolean
  running: boolean
  version?: string
  mode?: NfqwsMode
}) {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const [pending, setPending] = React.useState<NfqwsServiceAction | null>(null)

  const actionMutation = useMutation({
    mutationFn: (action: NfqwsServiceAction) => api.nfqwsAction(action),
    onMutate: (action) => setPending(action),
    onSuccess: (_d, action) => {
      queryClient.invalidateQueries({ queryKey: ['nfqws'] })
      queryClient.invalidateQueries({ queryKey: ['state'] })
      const titles: Record<NfqwsServiceAction, string> = {
        start: 'Starting nfqws2…',
        stop: 'Stopping nfqws2…',
        restart: 'Restarting nfqws2…',
        reload: 'Reloading strategy…',
        install: 'Installing nfqws2…',
      }
      toast({ variant: 'success', title: titles[action] })
    },
    onError: () =>
      toast({ variant: 'error', title: 'Service action failed' }),
    onSettled: () => setPending(null),
  })

  const busy = (a: NfqwsServiceAction) =>
    actionMutation.isPending && pending === a

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 p-5 md:flex-row md:items-center md:justify-between">
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
                <Badge variant="muted">Not installed</Badge>
              ) : running ? (
                <Badge variant="success">Running</Badge>
              ) : (
                <Badge variant="warning">Stopped</Badge>
              )}
            </div>
            <p className="mt-0.5 font-mono text-xs text-muted-foreground">
              {version ?? (installed ? 'version unknown' : 'not present')}
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
              Install
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
                Start
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
                Stop
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
                Restart
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
                Reload
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function ConfigEditor() {
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
        title: 'Configuration saved',
        description: 'Reload the service to apply changes.',
      })
    },
    onError: () => toast({ variant: 'error', title: 'Could not save config' }),
  })

  if (isLoading) {
    return <Skeleton className="h-96" />
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle>Strategy configuration</CardTitle>
          <p className="text-xs text-muted-foreground">
            Raw nfqws2 arguments and desync mode.
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
          Save
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-2 sm:max-w-md">
          <Label htmlFor="nfqws-mode">Bypass mode</Label>
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
              {(Object.keys(MODE_LABELS) as NfqwsMode[]).map((m) => (
                <SelectItem key={m} value={m}>
                  {MODE_LABELS[m]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Separator />

        <div className="space-y-2">
          <Label htmlFor="nfqws-raw">Raw configuration</Label>
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

  React.useEffect(() => {
    if (listData) {
      setContent(listData.content)
      setDirty(false)
    }
  }, [listData])

  const saveMutation = useMutation({
    mutationFn: () => api.saveNfqwsList(selected as string, content),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nfqws-list', selected] })
      setDirty(false)
      toast({
        variant: 'success',
        title: 'Hostlist saved',
        description: `${selected} updated.`,
      })
    },
    onError: () => toast({ variant: 'error', title: 'Could not save hostlist' }),
  })

  if (isLoading) {
    return <Skeleton className="h-96" />
  }

  if (!lists || lists.length === 0) {
    return (
      <EmptyState
        icon={FileText}
        title="No hostlists"
        description="nfqws2 has no managed hostlists on this device."
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
              {entryCount} active {entryCount === 1 ? 'entry' : 'entries'}
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
            Save
          </Button>
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
              placeholder="# one domain or CIDR per line"
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function DomainChecker() {
  const { toast } = useToast()
  const [domain, setDomain] = React.useState('')
  const [result, setResult] = React.useState<DomainCheck | null>(null)

  const checkMutation = useMutation({
    mutationFn: (d: string) => api.checkDomain(d),
    onSuccess: (res) => setResult(res),
    onError: () => toast({ variant: 'error', title: 'Domain check failed' }),
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
        <CardTitle>Domain reachability</CardTitle>
        <p className="text-xs text-muted-foreground">
          Probe a hostname on both the direct path and through the nfqws2 desync
          engine.
        </p>
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
              placeholder="youtube.com"
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
            Check
          </Button>
        </div>

        {result ? (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <ReachabilityTile
                label="Direct path"
                ok={result.direct_ok}
                okText="Reachable directly"
                failText="Blocked / unreachable"
              />
              <ReachabilityTile
                label="Through nfqws2"
                ok={result.bypass_ok}
                okText="Reachable via bypass"
                failText="Still unreachable"
              />
            </div>
            {result.note ? (
              <div className="rounded-md border border-border/70 bg-muted/40 px-3 py-2.5">
                <p className="text-xs leading-relaxed text-muted-foreground">
                  <span className="font-medium text-foreground">Note. </span>
                  {result.note}
                </p>
              </div>
            ) : null}
          </div>
        ) : (
          <div className="rounded-md border border-dashed border-border/70 px-4 py-8 text-center">
            <p className="text-xs text-muted-foreground">
              Enter a domain to run a reachability probe.
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
