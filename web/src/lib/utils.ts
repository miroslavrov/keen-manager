import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Format a byte count into a human string (e.g. 1.42 GB). */
export function formatBytes(bytes?: number, decimals = 1): string {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) return '—'
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(k)),
    sizes.length - 1,
  )
  const value = bytes / Math.pow(k, i)
  return `${value.toFixed(i === 0 ? 0 : decimals)} ${sizes[i]}`
}

/** Format a duration in seconds to a compact uptime string (e.g. 3d 4h 12m). */
export function formatUptime(seconds?: number): string {
  if (seconds === undefined || seconds === null || Number.isNaN(seconds))
    return '—'
  if (seconds < 1) return '0s'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const parts: string[] = []
  if (d) parts.push(`${d}d`)
  if (h) parts.push(`${h}h`)
  if (m && parts.length < 2) parts.push(`${m}m`)
  if (!d && !h && parts.length < 2) parts.push(`${s}s`)
  return parts.slice(0, 2).join(' ') || '0s'
}

/** Relative time from an ISO timestamp (e.g. "12s ago", "3m ago"). */
export function timeAgo(iso?: string): string {
  if (!iso) return '—'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '—'
  const diff = Math.max(0, Date.now() - then)
  const s = Math.floor(diff / 1000)
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

/** Seconds-ago helper for handshake/last-check style numbers. */
export function secondsAgoLabel(seconds?: number): string {
  if (seconds === undefined || seconds === null || Number.isNaN(seconds))
    return '—'
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${Math.floor(seconds)}s ago`
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  return `${h}h ago`
}

/** Format a date-only ISO string, tolerant of missing values. */
export function formatDate(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/** Latency threshold bucket used for color coding. */
export type LatencyLevel = 'good' | 'ok' | 'bad' | 'unknown'
export function latencyLevel(ms?: number): LatencyLevel {
  if (ms === undefined || ms === null || Number.isNaN(ms)) return 'unknown'
  if (ms < 80) return 'good'
  if (ms < 200) return 'ok'
  return 'bad'
}

/** Percent (0–100), clamped. */
export function pct(used?: number, total?: number): number {
  if (!total || total <= 0 || used === undefined) return 0
  return Math.min(100, Math.max(0, (used / total) * 100))
}
