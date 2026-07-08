import { useMutation, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { useToast } from '@/components/ui/toast'

/**
 * Shared connection action mutations (up / down / activate / test) with
 * consistent toasts and cache invalidation. Kept out of the pages so the
 * Dashboard and Connections views behave identically.
 */
export function useConnectionActions() {
  const queryClient = useQueryClient()
  const { toast } = useToast()

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
      const label = vars.name ? `“${vars.name}”` : 'connection'
      const messages: Record<typeof vars.action, string> = {
        up: `Bringing ${label} up`,
        down: `Taking ${label} down`,
        activate: `${label} set as active route`,
        test: `Testing ${label}…`,
      }
      toast({ variant: 'success', title: messages[vars.action] })
    },
    onError: (_e, vars) => {
      toast({
        variant: 'error',
        title: 'Action failed',
        description: `Could not ${vars.action} the connection.`,
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
