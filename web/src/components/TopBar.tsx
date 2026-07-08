import * as React from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Menu, Moon, Power, RefreshCw, ShieldAlert, Sun } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { StatusPill } from '@/components/StatusPill'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { LanguageToggle } from '@/components/LanguageToggle'
import { useTheme } from '@/hooks/use-theme'
import { useEvents } from '@/hooks/use-events'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { useT } from '@/i18n'

interface TopBarProps {
  onOpenSidebar: () => void
}

/** Top app bar: mobile nav trigger, global status, kill-switch, theme, refresh. */
export function TopBar({ onOpenSidebar }: TopBarProps) {
  const t = useT()
  const queryClient = useQueryClient()
  const { theme, toggleTheme } = useTheme()
  const { connected } = useEvents()
  const { toast } = useToast()
  const [confirmKill, setConfirmKill] = React.useState(false)

  const { data: state, isLoading } = useQuery({
    queryKey: ['state'],
    queryFn: api.state,
    refetchInterval: 8000,
  })

  const killActive = state?.kill_switch ?? false

  const killMutation = useMutation({
    mutationFn: (next: boolean) =>
      api.saveSettings({ kill_switch_default: next }),
    onSuccess: (_data, next) => {
      queryClient.setQueryData(['state'], (prev: typeof state) =>
        prev ? { ...prev, kill_switch: next } : prev,
      )
      queryClient.invalidateQueries({ queryKey: ['state'] })
      toast({
        variant: next ? 'warning' : 'success',
        title: next
          ? t('topbar.killswitchEngaged')
          : t('topbar.killswitchReleased'),
        description: next
          ? t('topbar.killswitchEngagedDesc')
          : t('topbar.killswitchReleasedDesc'),
      })
    },
  })

  const refresh = () => {
    queryClient.invalidateQueries()
    toast({
      title: t('topbar.refreshing'),
      description: t('topbar.refreshingDesc'),
    })
  }

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <Button
        variant="ghost"
        size="icon"
        className="lg:hidden"
        onClick={onOpenSidebar}
        aria-label="Open navigation"
      >
        <Menu className="h-5 w-5" />
      </Button>

      <StatusPill state={state} loading={isLoading} />

      <div className="ml-auto flex items-center gap-1.5">
        <div className="mr-1 hidden items-center gap-1.5 sm:flex">
          <span
            className={cn(
              'h-1.5 w-1.5 rounded-full',
              connected ? 'bg-success' : 'bg-muted-foreground/40',
            )}
          />
          <span className="text-[11px] font-medium text-muted-foreground">
            {connected ? t('common.live') : t('common.polling')}
          </span>
        </div>

        <LanguageToggle />

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={refresh}
              aria-label={t('common.refresh')}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.refresh')}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={toggleTheme}
              aria-label={theme === 'dark' ? t('topbar.lightTheme') : t('topbar.darkTheme')}
            >
              {theme === 'dark' ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {theme === 'dark' ? t('topbar.lightTheme') : t('topbar.darkTheme')}
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={killActive ? 'destructive' : 'outline'}
              size="sm"
              className={cn(
                'gap-1.5',
                !killActive &&
                  'border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive',
              )}
              onClick={() => setConfirmKill(true)}
            >
              {killActive ? (
                <ShieldAlert className="h-4 w-4" />
              ) : (
                <Power className="h-4 w-4" />
              )}
              <span className="hidden sm:inline">
                {killActive ? t('topbar.killswitchOn') : t('topbar.killswitch')}
              </span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {killActive
              ? t('topbar.killswitchTipOn')
              : t('topbar.killswitchTipOff')}
          </TooltipContent>
        </Tooltip>
      </div>

      <ConfirmDialog
        open={confirmKill}
        onOpenChange={setConfirmKill}
        destructive={!killActive}
        title={
          killActive
            ? t('topbar.killswitchReleaseTitle')
            : t('topbar.killswitchEngageTitle')
        }
        description={
          killActive
            ? t('topbar.killswitchReleaseDesc')
            : t('topbar.killswitchEngageDesc')
        }
        confirmLabel={
          killActive
            ? t('topbar.killswitchReleaseConfirm')
            : t('topbar.killswitchEngageConfirm')
        }
        loading={killMutation.isPending}
        onConfirm={() => {
          killMutation.mutate(!killActive)
          setConfirmKill(false)
        }}
      />
    </header>
  )
}
