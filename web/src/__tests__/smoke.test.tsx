import { describe, it, expect, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import App from '@/App'
import { LanguageProvider, type Lang } from '@/i18n'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { ThemeProvider } from '@/hooks/use-theme'
import { EventsProvider } from '@/hooks/use-events'
import { ToastProvider } from '@/components/ui/toast'
import { TooltipProvider } from '@/components/ui/tooltip'
import { en } from '@/i18n/en'
import { ru } from '@/i18n/ru'

// The exact set of authenticated routes reachable from the sidebar. This is the
// regression guard for the "open Failover and every tab goes blank" bug: if any
// page throws during render, its RouteBoundary (or the app boundary) renders the
// error fallback into <main>, which the assertions below catch.
//
// It is ALSO the bilingual guard: every route is rendered in both languages and
// must show its language-appropriate title, so a page that forgot to go through
// i18n (hardcoded English) fails the RU pass.
const ROUTES = [
  '/',
  '/connections',
  '/subscriptions',
  '/bypass',
  '/failover',
  '/logs',
  '/settings',
]

// Per-route page title in each language — used to prove the page rendered AND
// that it honoured the active language.
const TITLE: Record<string, { en: string; ru: string }> = {
  '/': { en: en.dashboard.title, ru: ru.dashboard.title },
  '/connections': { en: en.connections.title, ru: ru.connections.title },
  '/subscriptions': { en: en.subscriptions.title, ru: ru.subscriptions.title },
  '/bypass': { en: en.bypass.title, ru: ru.bypass.title },
  '/failover': { en: en.failover.title, ru: ru.failover.title },
  '/logs': { en: en.logs.title, ru: ru.logs.title },
  '/settings': { en: en.settings.title, ru: ru.settings.title },
}

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

afterEach(() => {
  try {
    window.localStorage.clear()
  } catch {
    /* ignore */
  }
})

for (const lang of ['en', 'ru'] as Lang[]) {
  describe(`[${lang}] every tab renders without tripping an error boundary`, () => {
    for (const route of ROUTES) {
      it(`renders ${route} in ${lang} with page content and no error fallback`, async () => {
        window.localStorage.setItem('keen:lang', lang)
        const { container } = renderAt(route)
        await settle()

        const main = container.querySelector('main')
        expect(main, `no <main> rendered for ${route}`).not.toBeNull()
        const text = main?.textContent ?? ''

        // Route content must not be the per-route or app error fallback.
        expect(text, `error fallback shown on ${route} (${lang})`).not.toContain(
          en.err.title,
        )
        expect(text, `error fallback shown on ${route} (${lang})`).not.toContain(
          ru.err.title,
        )
        // The page must have rendered meaningful content.
        expect(
          text.length,
          `no content rendered in <main> for ${route} (${lang})`,
        ).toBeGreaterThan(40)
        // And it must show its title in the ACTIVE language — the i18n guard.
        expect(
          text,
          `page title not localized to ${lang} on ${route}`,
        ).toContain(TITLE[route][lang])
      })
    }
  })
}

describe('routing edge cases', () => {
  it('renders an unknown route as the 404 page, not a crash', async () => {
    const { container } = renderAt('/this-route-does-not-exist')
    await settle()
    const text = container.textContent ?? ''
    expect(text).toContain(en.notfound.title)
    expect(text).not.toContain(en.err.title)
  })
})
