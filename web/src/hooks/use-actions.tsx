import { useMutation, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { useToast } from '@/components/ui/toast'
import { useT } from '@/i18n'

/**
 * Shared connection action mutations (up / down / activate / test) with
 * consistent toasts and cache invalidation. Kept out of the pages so the
 * Dashboard and Connections views behave identically.
 */
export function useConnectionActions() {
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const t = useT()

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['state'] })
    queryClient.invalidateQueries({ queryKey: ['connections'] })
  }

  const action = useMutation({
    mutationFn: ({
      id,
      action,
    }: {
      id: string
      name?: string
      action: 'up' | 'down' | 'activate' | 'test'
    }) => api.connectionAction(id, action),
    onSuccess: (_data, vars) => {
      invalidate()
      const name = vars.name ?? t('actions.connection')
      const messages: Record<typeof vars.action, string> = {
        up: t('actions.bringingUp', { name }),
        down: t('actions.takingDown', { name }),
        activate: t('actions.setActive', { name }),
        test: t('actions.testing', { name }),
      }
      toast({ variant: 'success', title: messages[vars.action] })
    },
    onError: (err) => {
      // Surface the daemon's real reason (e.g. an activation that rolled back
      // because the tunnel wouldn't carry traffic through DPI) instead of a
      // generic "action failed" — the backend now returns the probe target and
      // failure cause, and the API client threads it through as err.message.
      const detail = err instanceof Error && err.message ? err.message : ''
      toast({
        variant: 'error',
        title: t('actions.failedTitle'),
        description: detail || t('actions.failedDesc'),
      })
    },
  })

  return {
    up: (id: string, name?: string) =>
      action.mutate({ id, name, action: 'up' }),
    down: (id: string, name?: string) =>
      action.mutate({ id, name, action: 'down' }),
    activate: (id: string, name?: string) =>
      action.mutate({ id, name, action: 'activate' }),
    test: (id: string, name?: string) =>
      action.mutate({ id, name, action: 'test' }),
    pending: action.isPending,
    pendingVars: action.variables,
  }
}
