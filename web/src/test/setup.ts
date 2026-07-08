import '@testing-library/jest-dom/vitest'
import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'

afterEach(() => cleanup())

// jsdom lacks several browser APIs Radix UI and the app touch during render.
// Without these, a component throws at mount, which would falsely trip an
// ErrorBoundary and mask what the smoke test actually verifies. Polyfill the
// minimum surface.
if (typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false
    },
  })) as unknown as typeof window.matchMedia
}

class NoopObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
;(globalThis as unknown as { ResizeObserver?: unknown }).ResizeObserver ??=
  NoopObserver
;(globalThis as unknown as { IntersectionObserver?: unknown }).IntersectionObserver ??=
  NoopObserver

const proto = Element.prototype as unknown as Record<string, unknown>
proto.scrollIntoView ??= () => {}
proto.hasPointerCapture ??= () => false
proto.setPointerCapture ??= () => {}
proto.releasePointerCapture ??= () => {}

// Force every network call to fail so the typed api client degrades to its
// realistic mock data (see src/lib/api.ts) — no daemon needed.
globalThis.fetch = vi.fn(() =>
  Promise.reject(new Error('network disabled in tests')),
) as unknown as typeof fetch
