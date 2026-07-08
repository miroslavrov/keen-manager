import * as React from 'react'
import { Outlet } from 'react-router-dom'

import { Sidebar } from '@/components/Sidebar'
import { TopBar } from '@/components/TopBar'
import { Sheet, SheetContent } from '@/components/ui/sheet'

/** Application shell: fixed sidebar (desktop) + sheet (mobile) + top bar. */
export function AppLayout() {
  const [mobileOpen, setMobileOpen] = React.useState(false)

  return (
    <div className="flex h-dvh w-full overflow-hidden bg-background">
      {/* Desktop sidebar */}
      <div className="hidden lg:block">
        <Sidebar />
      </div>

      {/* Mobile sidebar */}
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-64 p-0">
          <Sidebar onNavigate={() => setMobileOpen(false)} />
        </SheetContent>
      </Sheet>

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar onOpenSidebar={() => setMobileOpen(true)} />
        <main className="app-grid-bg flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1400px] animate-fade-in px-4 py-6 sm:px-6 lg:px-8">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
