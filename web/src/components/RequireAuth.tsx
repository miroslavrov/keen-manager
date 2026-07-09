import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'

import { api, UNAUTHORIZED_EVENT } from '@/lib/api'

/**
 * Gate the app behind auth when the daemon requires it.
 *
 *  - Reads /api/auth. If auth is enabled and the user isn't authenticated, the
 *    protected tree is NEVER rendered — we show a redirecting state and bounce
 *    to /login. This is a HARD gate: previously children were rendered while a
 *    redirect was merely scheduled in an effect, which let protected pages flash
 *    (and, with a stale auth cache, occasionally stay) without a password.
 *  - The auth query is always revalidated fresh (no stale window) so enabling a
 *    password in Settings, a logout, or an expired session take effect
 *    immediately rather than after a cache-stale delay.
 *  - Also reacts to the global UNAUTHORIZED_EVENT (fired on any 401) so an
 *    expired session anywhere sends the user back to login.
 */
export function RequireAuth({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['auth'],
    queryFn: api.authState,
    // Auth state is cheap and security-critical: never serve it stale, and
    // re-check on mount and when the tab regains focus.
    staleTime: 0,
    gcTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: true,
    retry: false,
  })

  React.useEffect(() => {
    const handler = () => navigate('/login', { replace: true })
    window.addEventListener(UNAUTHORIZED_EVENT, handler)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, handler)
  }, [navigate])

  // A login is required when the daemon has auth enabled and this client is not
  // authenticated. Until /api/auth resolves, `data` is undefined and we hold on
  // the loading state rather than optimistically rendering the app.
  const mustLogin = !!data && data.enabled && !data.authenticated

  React.useEffect(() => {
    if (mustLogin) {
      navigate('/login', { replace: true })
    }
  }, [mustLogin, navigate])

  // Loading the auth state, or actively redirecting to /login: render a neutral
  // holding screen. Crucially we do NOT render `children` here, so no protected
  // page mounts (or fires its queries) before auth is confirmed.
  if (isLoading || mustLogin || !data) {
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
