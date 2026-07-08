import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'

import { api, UNAUTHORIZED_EVENT } from '@/lib/api'

/**
 * Gate the app behind auth when the daemon requires it.
 *  - Reads /api/auth. If auth is enabled and the user isn't authenticated,
 *    redirect to /login.
 *  - Also reacts to the global UNAUTHORIZED_EVENT (fired on any 401) so an
 *    expired session anywhere sends the user back to login.
 */
export function RequireAuth({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['auth'],
    queryFn: api.authState,
    staleTime: 30_000,
  })

  React.useEffect(() => {
    const handler = () => navigate('/login', { replace: true })
    window.addEventListener(UNAUTHORIZED_EVENT, handler)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, handler)
  }, [navigate])

  React.useEffect(() => {
    if (data && data.enabled && !data.authenticated) {
      navigate('/login', { replace: true })
    }
  }, [data, navigate])

  if (isLoading) {
    return (
      <div className="flex h-dvh items-center justify-center bg-background">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Connecting to keen-manager…
        </div>
      </div>
    )
  }

  return <>{children}</>
}
