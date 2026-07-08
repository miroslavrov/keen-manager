import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import App from '@/App'
import { LanguageProvider } from '@/i18n'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { ThemeProvider } from '@/hooks/use-theme'
import { EventsProvider } from '@/hooks/use-events'
import { ToastProvider } from '@/components/ui/toast'
import { TooltipProvider } from '@/components/ui/tooltip'
import { en } from '@/i18n/en'

// The exact set of authenticated routes reachable from the sidebar. This is the
// regression guard for the "open Failover and every tab goes blank" bug: if any
// page throws during render, its RouteBoundary (or the app boundary) renders the
// error fallback into <main>, which the assertions below catch.
const ROUTES = [
  '/',
  '/connections',
  '/subscriptions',
  '/bypass',
  '/failover',
  '/logs',
  '/settings',
]

function renderAt(route: string) {
  // Mirror the provider nesting from main.tsx, but with MemoryRouter so we can
  // mount a specific route. retry:false so the mock fallback resolves once.
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <LanguageProvider>
        <ErrorBoundary variant="app">
          <ThemeProvider>
            <ToastProvider>
              <TooltipProvider delayDuration={0}>
                <MemoryRouter initialEntries={[route]}>
                  <EventsProvider>
                    <App />
                  </EventsProvider>
                </MemoryRouter>
              </TooltipProvider>
            </ToastProvider>
          </ThemeProvider>
        </ErrorBoundary>
      </LanguageProvider>
    </QueryClientProvider>,
  )
}

// Let react-query resolve the mock fallbacks and any post-load re-render settle,
// so a crash that happens after data arrives is still caught.
const settle = () => new Promise((r) => setTimeout(r, 250))

describe('every tab renders without tripping an error boundary', () => {
  for (const route of ROUTES) {
    it(`renders ${route} with page content and no error fallback`, async () => {
      const { container } = renderAt(route)
      await settle()

      const main = container.querySelector('main')
      expect(main, `no <main> rendered for ${route}`).not.toBeNull()
      const text = main?.textContent ?? ''

      // Route content must not be the per-route or app error fallback.
      expect(text, `error fallback shown on ${route}`).not.toContain(en.err.title)
      expect(text, `app error fallback shown on ${route}`).not.toContain(
        en.err.appDesc,
      )
      // And the page must have actually rendered meaningful content.
      expect(
        text.length,
        `no content rendered in <main> for ${route}`,
      ).toBeGreaterThan(40)
    })
  }

  it('renders an unknown route as the 404 page, not a crash', async () => {
    const { container } = renderAt('/this-route-does-not-exist')
    await settle()
    const text = container.textContent ?? ''
    expect(text).toContain(en.notfound.title)
    expect(text).not.toContain(en.err.title)
  })
})
