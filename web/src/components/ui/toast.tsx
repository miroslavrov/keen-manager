import * as React from 'react'
import { createPortal } from 'react-dom'
import { CircleCheck, CircleX, Info, TriangleAlert, X } from 'lucide-react'

import { cn } from '@/lib/utils'

export type ToastVariant = 'default' | 'success' | 'error' | 'warning'

export interface ToastItem {
  id: string
  title: string
  description?: string
  variant?: ToastVariant
  duration?: number
}

interface ToastContextValue {
  toasts: ToastItem[]
  toast: (t: Omit<ToastItem, 'id'>) => string
  dismiss: (id: string) => void
}

const ToastContext = React.createContext<ToastContextValue | null>(null)

let counter = 0
function nextId() {
  counter += 1
  return `t-${Date.now()}-${counter}`
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = React.useState<ToastItem[]>([])
  const timers = React.useRef<Record<string, ReturnType<typeof setTimeout>>>({})

  const dismiss = React.useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
    const timer = timers.current[id]
    if (timer) {
      clearTimeout(timer)
      delete timers.current[id]
    }
  }, [])

  const toast = React.useCallback(
    (t: Omit<ToastItem, 'id'>) => {
      const id = nextId()
      const duration = t.duration ?? 4200
      setToasts((prev) => [...prev, { ...t, id }])
      timers.current[id] = setTimeout(() => dismiss(id), duration)
      return id
    },
    [dismiss],
  )

  React.useEffect(() => {
    const store = timers.current
    return () => {
      Object.values(store).forEach(clearTimeout)
    }
  }, [])

  return (
    <ToastContext.Provider value={{ toasts, toast, dismiss }}>
      {children}
      <ToastViewport toasts={toasts} dismiss={dismiss} />
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = React.useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within <ToastProvider>')
  return ctx
}

const variantStyles: Record<
  ToastVariant,
  { icon: React.ReactNode; ring: string }
> = {
  default: {
    icon: <Info className="h-4 w-4 text-muted-foreground" />,
    ring: 'border-border',
  },
  success: {
    icon: <CircleCheck className="h-4 w-4 text-success" />,
    ring: 'border-success/40',
  },
  error: {
    icon: <CircleX className="h-4 w-4 text-destructive" />,
    ring: 'border-destructive/40',
  },
  warning: {
    icon: <TriangleAlert className="h-4 w-4 text-warning" />,
    ring: 'border-warning/40',
  },
}

function ToastViewport({
  toasts,
  dismiss,
}: {
  toasts: ToastItem[]
  dismiss: (id: string) => void
}) {
  if (typeof document === 'undefined') return null
  return createPortal(
    <div className="pointer-events-none fixed bottom-0 right-0 z-[100] flex w-full max-w-sm flex-col gap-2 p-4">
      {toasts.map((t) => {
        const v = variantStyles[t.variant ?? 'default']
        return (
          <div
            key={t.id}
            role="status"
            aria-live="polite"
            className={cn(
              'pointer-events-auto flex items-start gap-3 rounded-lg border bg-card p-3.5 shadow-lg animate-in slide-in-from-right-4 fade-in-0',
              v.ring,
            )}
          >
            <div className="mt-0.5 shrink-0">{v.icon}</div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium text-foreground">
                {t.title}
              </div>
              {t.description ? (
                <div className="mt-0.5 break-words text-xs text-muted-foreground">
                  {t.description}
                </div>
              ) : null}
            </div>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              className="shrink-0 rounded-sm text-muted-foreground opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Dismiss notification"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        )
      })}
    </div>,
    document.body,
  )
}
