import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import App from './App'
import { LanguageProvider } from '@/i18n'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { ThemeProvider } from '@/hooks/use-theme'
import { EventsProvider } from '@/hooks/use-events'
import { ToastProvider } from '@/components/ui/toast'
import { TooltipProvider } from '@/components/ui/tooltip'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 5_000,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <LanguageProvider>
        {/* App-level catch-all: even a provider/mount crash shows a recoverable
            screen instead of a blank page. Per-route boundaries (see App.tsx)
            keep the shell alive when a single page fails. */}
        <ErrorBoundary variant="app">
          <ThemeProvider>
            <ToastProvider>
              <TooltipProvider delayDuration={200}>
                <BrowserRouter>
                  <EventsProvider>
                    <App />
                  </EventsProvider>
                </BrowserRouter>
              </TooltipProvider>
            </ToastProvider>
          </ThemeProvider>
        </ErrorBoundary>
      </LanguageProvider>
    </QueryClientProvider>
  </React.StrictMode>,
)
