// Lightweight i18n: two typed dictionaries, a dotted-path lookup, and a React
// context exposing the current language + a t() function. No external deps.
//
// Design notes:
//  - t(key) walks a dotted path; on a miss it falls back to English, then to the
//    raw key — so a missing translation degrades gracefully and never throws.
//  - Language is persisted to localStorage and auto-detected from the browser on
//    first load. Switching is instant (context re-render), no page reload.
import * as React from 'react'

import { en } from './en'
import { ru } from './ru'

export type Lang = 'en' | 'ru'
export const LANGS: Lang[] = ['en', 'ru']

const DICTS: Record<Lang, unknown> = { en, ru }
const STORAGE_KEY = 'keen:lang'

function detectInitial(): Lang {
  if (typeof window !== 'undefined') {
    try {
      const saved = window.localStorage.getItem(STORAGE_KEY)
      if (saved === 'en' || saved === 'ru') return saved
    } catch {
      /* localStorage may be unavailable (private mode) — ignore */
    }
    const nav = window.navigator?.language?.toLowerCase() ?? ''
    if (nav.startsWith('ru')) return 'ru'
  }
  return 'en'
}

function lookup(dict: unknown, key: string): string | undefined {
  let cur: unknown = dict
  for (const part of key.split('.')) {
    if (cur && typeof cur === 'object' && part in (cur as Record<string, unknown>)) {
      cur = (cur as Record<string, unknown>)[part]
    } else {
      return undefined
    }
  }
  return typeof cur === 'string' ? cur : undefined
}

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s
  return s.replace(/\{(\w+)\}/g, (_m, k: string) =>
    k in vars ? String(vars[k]) : `{${k}}`,
  )
}

export type TFunc = (
  key: string,
  vars?: Record<string, string | number>,
) => string

interface LangContextValue {
  lang: Lang
  setLang: (l: Lang) => void
  toggle: () => void
  t: TFunc
}

const LangContext = React.createContext<LangContextValue | null>(null)

export function LanguageProvider({ children }: { children: React.ReactNode }) {
  const [lang, setLangState] = React.useState<Lang>(detectInitial)

  React.useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = lang
    }
  }, [lang])

  const setLang = React.useCallback((l: Lang) => {
    setLangState(l)
    try {
      window.localStorage.setItem(STORAGE_KEY, l)
    } catch {
      /* ignore persistence failures */
    }
  }, [])

  const toggle = React.useCallback(() => {
    setLangState((prev) => {
      const next: Lang = prev === 'en' ? 'ru' : 'en'
      try {
        window.localStorage.setItem(STORAGE_KEY, next)
      } catch {
        /* ignore */
      }
      return next
    })
  }, [])

  const t = React.useCallback<TFunc>(
    (key, vars) => {
      const val = lookup(DICTS[lang], key) ?? lookup(DICTS.en, key) ?? key
      return interpolate(val, vars)
    },
    [lang],
  )

  const value = React.useMemo<LangContextValue>(
    () => ({ lang, setLang, toggle, t }),
    [lang, setLang, toggle, t],
  )

  return <LangContext.Provider value={value}>{children}</LangContext.Provider>
}

export function useLang(): LangContextValue {
  const ctx = React.useContext(LangContext)
  if (!ctx) {
    throw new Error('useLang must be used within a LanguageProvider')
  }
  return ctx
}

export function useT(): TFunc {
  return useLang().t
}
