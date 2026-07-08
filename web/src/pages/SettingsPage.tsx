import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  Cpu,
  Info,
  KeyRound,
  Loader2,
  LogOut,
  Save,
  Server as ServerIcon,
} from 'lucide-react'

import { PageHeader } from '@/components/PageHeader'
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
  SelectValue,
} from '@/components/ui/select'
import { useTheme, type Theme } from '@/hooks/use-theme'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { formatUptime } from '@/lib/utils'
import { useT } from '@/i18n'
import type { Settings } from '@/lib/types'

// The backend PUT accepts a couple of fields not present in the Settings TS
// contract (a password to (re)set, and the auto-select cadence in minutes).
interface SettingsForm {
  port: number
  auth_enabled: boolean
  theme: Theme
  backup_on_change: boolean
  rollback_timeout_s: number
  kill_switch_default: boolean
  auto_select_interval_min: number
}

type SettingsPayload = Partial<Settings> & {
  password?: string
  auto_select_interval_min?: number
}

export function SettingsPage() {
  const t = useT()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const { theme, setTheme } = useTheme()

  const { data: settings, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: api.settings,
  })
  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    staleTime: 60_000,
  })

  const [form, setForm] = React.useState<SettingsForm | null>(null)
  const [password, setPassword] = React.useState('')
  const [dirty, setDirty] = React.useState(false)

  React.useEffect(() => {
    if (settings && !form) {
      setForm({
        port: settings.port,
        auth_enabled: settings.auth_enabled,
        theme: settings.theme,
        backup_on_change: settings.backup_on_change,
        rollback_timeout_s: settings.rollback_timeout_s,
        kill_switch_default: settings.kill_switch_default,
        auto_select_interval_min: 15,
      })
    }
  }, [settings, form])

  const update = <K extends keyof SettingsForm>(
    key: K,
    value: SettingsForm[K],
  ) => {
    setForm((prev) => (prev ? { ...prev, [key]: value } : prev))
    setDirty(true)
  }

  const saveMutation = useMutation({
    mutationFn: () => {
      if (!form) return Promise.resolve({ ok: true })
      const body: SettingsPayload = {
        port: form.port,
        auth_enabled: form.auth_enabled,
        theme: form.theme,
        backup_on_change: form.backup_on_change,
        rollback_timeout_s: form.rollback_timeout_s,
        kill_switch_default: form.kill_switch_default,
        auto_select_interval_min: form.auto_select_interval_min,
      }
      if (password.trim()) body.password = password
      return api.saveSettings(body as Partial<Settings>)
    },
    onSuccess: () => {
      if (form) setTheme(form.theme)
      queryClient.invalidateQueries({ queryKey: ['settings'] })
      queryClient.invalidateQueries({ queryKey: ['auth'] })
      setPassword('')
      setDirty(false)
      toast({
        variant: 'success',
        title: t('settings.saved'),
        description: t('settings.savedDesc'),
      })
    },
    onError: () => toast({ variant: 'error', title: t('settings.saveError') }),
  })

  const logoutMutation = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: () => {
      queryClient.clear()
      navigate('/login', { replace: true })
    },
  })

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('settings.title')}
        description={t('settings.desc')}
        actions={
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
            {t('common.saveChanges')}
          </Button>
        }
      />

      {isLoading || !form ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-96" />
          <Skeleton className="h-96" />
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {/* Web UI */}
          <Card>
            <CardHeader>
              <CardTitle>{t('settings.webInterface')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor="port">{t('settings.port')}</Label>
                <Input
                  id="port"
                  type="number"
                  min={1}
                  max={65535}
                  value={form.port}
                  onChange={(e) => update('port', Number(e.target.value))}
                  className="max-w-[160px] tabular-nums"
                />
                <p className="text-xs text-muted-foreground">
                  {t('settings.portHint')}
                </p>
              </div>

              <Separator />

              <div className="space-y-2">
                <Label htmlFor="theme">{t('settings.theme')}</Label>
                <Select
                  value={form.theme}
                  onValueChange={(v) => update('theme', v as Theme)}
                >
                  <SelectTrigger id="theme" className="max-w-[200px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="dark">{t('settings.themeDark')}</SelectItem>
                    <SelectItem value="light">{t('settings.themeLight')}</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {t('settings.themeCurrent', { theme })}
                </p>
              </div>

              <Separator />

              <ToggleRow
                label={t('settings.backupOnChange')}
                description={t('settings.backupOnChangeDesc')}
                checked={form.backup_on_change}
                onCheckedChange={(v) => update('backup_on_change', v)}
              />

              <div className="space-y-2">
                <Label htmlFor="rollback">{t('settings.rollbackTimeout')}</Label>
                <Input
                  id="rollback"
                  type="number"
                  min={0}
                  value={form.rollback_timeout_s}
                  onChange={(e) =>
                    update('rollback_timeout_s', Number(e.target.value))
                  }
                  className="max-w-[160px] tabular-nums"
                />
                <p className="text-xs text-muted-foreground">
                  {t('settings.rollbackTimeoutHint')}
                </p>
              </div>

              <Separator />

              <div className="space-y-2">
                <Label htmlFor="auto-select">
                  {t('settings.autoSelectInterval')}
                </Label>
                <Input
                  id="auto-select"
                  type="number"
                  min={0}
                  value={form.auto_select_interval_min}
                  onChange={(e) =>
                    update('auto_select_interval_min', Number(e.target.value))
                  }
                  className="max-w-[160px] tabular-nums"
                />
                <p className="text-xs text-muted-foreground">
                  {t('settings.autoSelectIntervalHint')}
                </p>
              </div>

              <Separator />

              <ToggleRow
                label={t('settings.killSwitchDefault')}
                description={t('settings.killSwitchDefaultDesc')}
                checked={form.kill_switch_default}
                onCheckedChange={(v) => update('kill_switch_default', v)}
              />
            </CardContent>
          </Card>

          {/* Auth + system column */}
          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>{t('settings.authentication')}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-5">
                <ToggleRow
                  label={t('settings.requirePassword')}
                  description={t('settings.requirePasswordDesc')}
                  checked={form.auth_enabled}
                  onCheckedChange={(v) => update('auth_enabled', v)}
                />

                <div className="space-y-2">
                  <Label htmlFor="password">
                    {form.auth_enabled
                      ? t('settings.setChangePassword')
                      : t('settings.password')}
                  </Label>
                  <div className="relative max-w-sm">
                    <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      id="password"
                      type="password"
                      value={password}
                      onChange={(e) => {
                        setPassword(e.target.value)
                        setDirty(true)
                      }}
                      placeholder={t('settings.passwordPlaceholder')}
                      autoComplete="new-password"
                      disabled={!form.auth_enabled}
                      className="pl-9"
                    />
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {t('settings.passwordHint')}
                  </p>
                </div>

                {settings?.auth_enabled ? (
                  <>
                    <Separator />
                    <Button
                      variant="outline"
                      onClick={() => logoutMutation.mutate()}
                      disabled={logoutMutation.isPending}
                      className="gap-1.5 border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
                    >
                      {logoutMutation.isPending ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <LogOut className="h-4 w-4" />
                      )}
                      {t('settings.signOut')}
                    </Button>
                  </>
                ) : null}
              </CardContent>
            </Card>

            <PlatformCard
              platform={settings?.platform}
              health={health}
            />
          </div>
        </div>
      )}
    </div>
  )
}

function PlatformCard({
  platform,
  health,
}: {
  platform?: Settings['platform']
  health?: {
    version: string
    arch: string
    os: string
    uptime_seconds: number
  }
}) {
  const t = useT()
  return (
    <Card>
      <CardHeader className="flex-row items-center gap-2 space-y-0">
        <Cpu className="h-4 w-4 text-muted-foreground" />
        <CardTitle>{t('settings.platform')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <InfoRow
          icon={Cpu}
          label={t('settings.architecture')}
          value={platform?.arch}
          mono
        />
        <Separator />
        <InfoRow
          icon={ServerIcon}
          label={t('settings.osVersion')}
          value={platform?.os_version}
        />
        <Separator />
        <InfoRow
          icon={Info}
          label={t('settings.entwarePath')}
          value={platform?.entware_path}
          mono
        />
        <Separator />
        <InfoRow
          icon={Info}
          label={t('settings.daemonVersion')}
          value={health ? `v${health.version}` : undefined}
          mono
        />
        <Separator />
        <InfoRow
          icon={Info}
          label={t('settings.kernelOs')}
          value={health?.os}
        />
        <Separator />
        <InfoRow
          icon={Activity}
          label={t('settings.daemonUptime')}
          value={
            health?.uptime_seconds !== undefined
              ? formatUptime(health.uptime_seconds)
              : undefined
          }
          mono
        />
      </CardContent>
    </Card>
  )
}

function InfoRow({
  icon: Icon,
  label,
  value,
  mono,
}: {
  icon: typeof Info
  label: string
  value?: string
  mono?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </span>
      <span
        className={
          mono
            ? 'font-mono text-sm tabular-nums text-foreground'
            : 'text-sm text-foreground'
        }
      >
        {value ?? '—'}
      </span>
    </div>
  )
}

function ToggleRow({
  label,
  description,
  checked,
  onCheckedChange,
}: {
  label: string
  description: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-0.5">
        <Label>{label}</Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={onCheckedChange}
        aria-label={label}
      />
    </div>
  )
}
