import { Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/AppLayout'
import { RequireAuth } from '@/components/RequireAuth'
import { DashboardPage } from '@/pages/DashboardPage'
import { ConnectionsPage } from '@/pages/ConnectionsPage'
import { SubscriptionsPage } from '@/pages/SubscriptionsPage'
import { BypassPage } from '@/pages/BypassPage'
import { FailoverPage } from '@/pages/FailoverPage'
import { LogsPage } from '@/pages/LogsPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { LoginPage } from '@/pages/LoginPage'
import { NotFoundPage } from '@/pages/NotFoundPage'

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
        <Route path="/" element={<DashboardPage />} />
        <Route path="/connections" element={<ConnectionsPage />} />
        <Route path="/subscriptions" element={<SubscriptionsPage />} />
        <Route path="/bypass" element={<BypassPage />} />
        <Route path="/failover" element={<FailoverPage />} />
        <Route path="/logs" element={<LogsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      {/* Public catch-all: unknown URLs render the 404 page regardless of auth
          state (it lives outside the RequireAuth gate on purpose). */}
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
