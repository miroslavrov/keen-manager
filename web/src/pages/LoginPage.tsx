import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Lock, ShieldCheck } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent } from '@/components/ui/card'
import { useToast } from '@/components/ui/toast'
import { api } from '@/lib/api'
import { useT } from '@/i18n'

export function LoginPage() {
  const t = useT()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { toast } = useToast()
  const [password, setPassword] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)

  // If auth is disabled or already authenticated, skip the login screen.
  const { data: auth } = useQuery({ queryKey: ['auth'], queryFn: api.authState })
  React.useEffect(() => {
    if (auth && (!auth.enabled || auth.authenticated)) {
      navigate('/', { replace: true })
    }
  }, [auth, navigate])

  const login = useMutation({
    mutationFn: () => api.login(password),
    onSuccess: () => {
      queryClient.invalidateQueries()
      toast({ variant: 'success', title: t('login.signedIn') })
      navigate('/', { replace: true })
    },
    onError: () => {
      setError(t('login.error'))
    },
  })

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!password) {
      setError(t('login.required'))
      return
    }
    login.mutate()
  }

  return (
    <div className="app-grid-bg flex min-h-dvh items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/15 text-primary">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <div>
            <h1 className="text-lg font-semibold tracking-tight">
              {t('login.title')}
            </h1>
            <p className="text-sm text-muted-foreground">
              {t('login.subtitle')}
            </p>
          </div>
        </div>

        <Card>
          <CardContent className="pt-6">
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="password">{t('login.password')}</Label>
                <div className="relative">
                  <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    id="password"
                    type="password"
                    autoFocus
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="••••••••"
                    className="pl-9"
                    aria-invalid={!!error}
                  />
                </div>
                {error ? (
                  <p className="text-xs text-destructive">{error}</p>
                ) : null}
              </div>

              <Button
                type="submit"
                className="w-full"
                disabled={login.isPending}
              >
                {login.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : null}
                {login.isPending ? t('login.signingIn') : t('login.signIn')}
              </Button>
            </form>
          </CardContent>
        </Card>

        <p className="mt-6 text-center text-xs text-muted-foreground">
          {t('login.caption')}
        </p>
      </div>
    </div>
  )
}
