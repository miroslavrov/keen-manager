import * as React from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { UNAUTHORIZED_EVENT } from '@/lib/api'

export interface LogEvent {
  service: string
  line: string
}

type LogListener = (evt: LogEvent) => void

interface EventsContextValue {
  connected: boolean
  /** Subscribe to live "log" SSE events. Returns an unsubscribe function. */
  subscribeLogs: (fn: LogListener) => () => void
}

const EventsContext = React.createContext<EventsContextValue | null>(null)

/**
 * Connects to the /api/events SSE stream.
 *  - "state" events invalidate the aggregate state query so the UI refreshes.
 *  - "log" events are fanned out to registered listeners (used by /logs).
 * If the stream can't be established (backend not present), we simply stay
 * disconnected — the rest of the app keeps working from polling + mocks.
 */
export function EventsProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient()
  const [connected, setConnected] = React.useState(false)
  const listeners = React.useRef<Set<LogListener>>(new Set())

  const subscribeLogs = React.useCallback((fn: LogListener) => {
    listeners.current.add(fn)
    return () => {
      listeners.current.delete(fn)
    }
  }, [])

  React.useEffect(() => {
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') {
      return
    }

    let es: EventSource | null = null
    let closedByUs = false
    let retry: ReturnType<typeof setTimeout> | undefined

    const connect = () => {
      try {
        es = new EventSource('/api/events', { withCredentials: true })
      } catch {
        return
      }

      es.onopen = () => setConnected(true)

      es.addEventListener('state', () => {
        queryClient.invalidateQueries({ queryKey: ['state'] })
        queryClient.invalidateQueries({ queryKey: ['connections'] })
      })

      es.addEventListener('log', (e) => {
        try {
          const data = JSON.parse((e as MessageEvent).data) as LogEvent
          listeners.current.forEach((fn) => fn(data))
        } catch {
          /* ignore malformed frames */
        }
      })

      es.onerror = () => {
        setConnected(false)
        es?.close()
        if (!closedByUs) {
          // Backend may not implement SSE; retry slowly, don't hammer.
          retry = setTimeout(connect, 8000)
        }
      }
    }

    const onUnauthorized = () => {
      closedByUs = true
      es?.close()
    }
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)

    connect()

    return () => {
      closedByUs = true
      window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
      if (retry) clearTimeout(retry)
      es?.close()
    }
  }, [queryClient])

  return (
    <EventsContext.Provider value={{ connected, subscribeLogs }}>
      {children}
    </EventsContext.Provider>
  )
}

export function useEvents() {
  const ctx = React.useContext(EventsContext)
  if (!ctx) throw new Error('useEvents must be used within <EventsProvider>')
  return ctx
}
