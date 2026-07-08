import type { ReactNode } from 'react'
import { Route, Routes, useLocation } from 'react-router-dom'

import { AppLayout } from '@/components/AppLayout'
import { RequireAuth } from '@/components/RequireAuth'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { DashboardPage } from '@/pages/DashboardPage'
import { ConnectionsPage } from '@/pages/ConnectionsPage'
import { SubscriptionsPage } from '@/pages/SubscriptionsPage'
import { BypassPage } from '@/pages/BypassPage'
import { FailoverPage } from '@/pages/FailoverPage'
import { LogsPage } from '@/pages/LogsPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { LoginPage } from '@/pages/LoginPage'
import { NotFoundPage } from '@/pages/NotFoundPage'

// RouteBoundary wraps each page in its own error boundary keyed by pathname, so
// a crash in one page is contained to the content area (the sidebar/top bar
// stay usable) and navigating to another route clears the error automatically.
function RouteBoundary({ children }: { children: ReactNode }) {
  const { pathname } = useLocation()
  return (
    <ErrorBoundary variant="page" resetKey={pathname}>
      {children}
    </ErrorBoundary>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route
          path="/"
          element={
            <RouteBoundary>
              <DashboardPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/connections"
          element={
            <RouteBoundary>
              <ConnectionsPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/subscriptions"
          element={
            <RouteBoundary>
              <SubscriptionsPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/bypass"
          element={
            <RouteBoundary>
              <BypassPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/failover"
          element={
            <RouteBoundary>
              <FailoverPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/logs"
          element={
            <RouteBoundary>
              <LogsPage />
            </RouteBoundary>
          }
        />
        <Route
          path="/settings"
          element={
            <RouteBoundary>
              <SettingsPage />
            </RouteBoundary>
          }
        />
      </Route>
      {/* Public catch-all: unknown URLs render the 404 page regardless of auth
          state (it lives outside the RequireAuth gate on purpose). */}
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
