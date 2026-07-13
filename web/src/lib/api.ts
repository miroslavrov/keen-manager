// Typed API client for keen-manager.
//
// Design goals:
//  - Same-origin fetch against /api.
//  - 401 -> broadcast an "unauthorized" event so the app can redirect to /login.
//  - Graceful degradation: when a GET fails (network error, 404, non-OK, or the
//    backend simply hasn't implemented an endpoint yet) we fall back to realistic
//    mock data so the UI never crashes and stays fully browsable. This is the
//    USE_MOCKS behavior. Mutations also degrade to a synthetic { ok:true } so the
//    UI flow can be exercised without a live daemon.

import * as mocks from './mocks'
import type {
  AppState,
  AuthState,
  Bypass,
  Conn,
  ConnDetail,
  ConnType,
  DomainCheck,
  Failover,
  Health,
  InterfacesView,
  ListResolveResult,
  LogResponse,
  LogService,
  Nfqws,
  NfqwsConf,
  NfqwsConfig,
  NfqwsImportResult,
  NfqwsList,
  Ok,
  PresetCatalog,
  RouteDetail,
  RouteEntry,
  Server,
  Settings,
  Sub,
  Traffic,
} from './types'

const BASE = '/api'

// Mocks are a DEVELOPMENT/TEST convenience ONLY. In a production build the UI
// must reflect the real daemon: real parsed servers, real empty states, real
// errors — never fabricated data (fake servers hide a broken subscription and
// mislead the operator). import.meta.env.DEV is true under the vite dev server
// and vitest, false in the shipped bundle, so mock fallback is compiled out of
// the binary the router runs.
const USE_MOCKS = import.meta.env.DEV

/** Fired when the API returns 401 so the router can send the user to /login. */
export const UNAUTHORIZED_EVENT = 'keen:unauthorized'

class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized')
    this.name = 'UnauthorizedError'
  }
}

function emitUnauthorized() {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT))
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  signal?: AbortSignal
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, signal } = opts
  const res = await fetch(`${BASE}${path}`, {
    method,
    signal,
    credentials: 'same-origin',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    emitUnauthorized()
    throw new UnauthorizedError()
  }
  // Read the body once; reuse it for both the error path and the success parse.
  const text = await res.text()
  if (!res.ok) {
    // The daemon renders failures as {"error": "..."} (writeErr). Surface that
    // real reason to the caller so pages can show *why* an action failed (e.g.
    // an activation that rolled back because the tunnel wouldn't carry traffic)
    // instead of a generic "failed: 500".
    let detail = ''
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: string; message?: string }
        detail = parsed.error || parsed.message || text
      } catch {
        detail = text
      }
    }
    throw new Error(detail || `${method} ${path} failed: ${res.status}`)
  }
  // 204 / empty body tolerance
  if (!text) return {} as T
  return JSON.parse(text) as T
}

/**
 * Wrap a GET with a mock fallback. A genuine 401 still propagates (so the login
 * flow works); everything else resolves to the provided mock so the UI degrades
 * gracefully to a browsable state.
 */
async function withMock<T>(
  fn: () => Promise<T>,
  fallback: T | (() => T),
): Promise<T> {
  try {
    return await fn()
  } catch (err) {
    if (err instanceof UnauthorizedError) throw err
    // Production: surface the real failure to react-query (pages render their
    // empty/error state, contained by the per-route ErrorBoundary) instead of
    // papering over it with fake data.
    if (!USE_MOCKS) throw err
    return typeof fallback === 'function' ? (fallback as () => T)() : fallback
  }
}

/** Mutations degrade to a synthetic ok result so flows are exercisable offline. */
async function withOk<T extends object>(
  fn: () => Promise<T>,
  fallback: T | (() => T),
): Promise<T> {
  try {
    return await fn()
  } catch (err) {
    if (err instanceof UnauthorizedError) throw err
    if (!USE_MOCKS) throw err
    return typeof fallback === 'function' ? (fallback as () => T)() : fallback
  }
}

export const api = {
  // ---- system / auth ----
  health: () =>
    withMock<Health>(() => request('/health'), () => mocks.mockHealth),

  authState: () =>
    withMock<AuthState>(() => request('/auth'), () => mocks.mockAuth),

  login: (password: string) =>
    request<Ok>('/login', { method: 'POST', body: { password } }),

  logout: () =>
    withOk<Ok>(() => request('/logout', { method: 'POST' }), { ok: true }),

  // ---- aggregate state ----
  state: () => withMock<AppState>(() => request('/state'), () => mocks.mockState),

  traffic: () =>
    withMock<Traffic>(() => request('/traffic'), () => mocks.mockTraffic),

  // ---- connections ----
  connections: () =>
    withMock<Conn[]>(() => request('/connections'), () => mocks.mockConnections),

  connection: (id: string) =>
    withMock<ConnDetail>(
      () => request(`/connections/${id}`),
      () =>
        mocks.mockConnDetails[id] ?? {
          ...(mocks.mockConnections.find((c) => c.id === id) ??
            mocks.mockConnections[0]),
        },
    ),

  createConnection: (body: {
    type: ConnType
    name: string
    awg_conf?: string
    share_link?: string
  }) =>
    withOk<Conn>(
      () => request('/connections', { method: 'POST', body }),
      () => ({
        id: `new-${Date.now()}`,
        type: body.type,
        name: body.name,
        enabled: true,
        status: 'checking',
        active: false,
      }),
    ),

  updateConnection: (id: string, body: Partial<Conn>) =>
    withOk<Conn>(
      () => request(`/connections/${id}`, { method: 'PUT', body }),
      () => ({
        ...(mocks.mockConnections.find((c) => c.id === id) ??
          mocks.mockConnections[0]),
        ...body,
      }),
    ),

  deleteConnection: (id: string) =>
    withOk<Ok>(
      () => request(`/connections/${id}`, { method: 'DELETE' }),
      { ok: true },
    ),

  connectionAction: (
    id: string,
    action: 'up' | 'down' | 'activate' | 'test',
  ) =>
    withOk<Ok>(
      () => request(`/connections/${id}/${action}`, { method: 'POST' }),
      { ok: true },
    ),

  // ---- subscriptions ----
  subscriptions: () =>
    withMock<Sub[]>(
      () => request('/subscriptions'),
      () => mocks.mockSubscriptions,
    ),

  createSubscription: (body: { name: string; url: string }) =>
    withOk<Sub>(
      () => request('/subscriptions', { method: 'POST', body }),
      () => ({
        id: `sub-${Date.now()}`,
        name: body.name,
        url: body.url,
        host: (() => {
          try {
            return new URL(body.url).host
          } catch {
            return body.url
          }
        })(),
        protocol: 'mixed',
        server_count: 0,
        auto_select_best: false,
        enabled: true,
      }),
    ),

  updateSubscription: (
    id: string,
    body: Partial<{
      name: string
      enabled: boolean
      auto_select_best: boolean
      update_interval_hours: number
    }>,
  ) =>
    withOk<Sub>(
      () => request(`/subscriptions/${id}`, { method: 'PUT', body }),
      () =>
        ({
          ...(mocks.mockSubscriptions.find((s) => s.id === id) ?? mocks.mockSubscriptions[0]),
          ...body,
        }) as Sub,
    ),

  refreshSubscription: (id: string) =>
    withOk<Sub>(
      () => request(`/subscriptions/${id}/refresh`, { method: 'POST' }),
      () =>
        mocks.mockSubscriptions.find((s) => s.id === id) ??
        mocks.mockSubscriptions[0],
    ),

  deleteSubscription: (id: string) =>
    withOk<Ok>(
      () => request(`/subscriptions/${id}`, { method: 'DELETE' }),
      { ok: true },
    ),

  subscriptionServers: (id: string) =>
    withMock<Server[]>(
      () => request(`/subscriptions/${id}/servers`),
      () => mocks.mockServers[id] ?? [],
    ),

  selectBest: (id: string) =>
    withOk<Ok & { selected_id?: string }>(
      () => request(`/subscriptions/${id}/select-best`, { method: 'POST' }),
      () => ({ ok: true, selected_id: mocks.mockServers[id]?.[0]?.id }),
    ),

  // ---- router interfaces (live from KeeneticOS RCI) ----
  interfaces: () =>
    withMock<InterfacesView>(
      () => request('/interfaces'),
      () => mocks.mockInterfaces,
    ),

  // ---- routes / "Маршруты" ----
  routes: () =>
    withMock<RouteEntry[]>(() => request('/routes'), () => mocks.mockRoutes),

  routePresets: () =>
    withMock<PresetCatalog>(
      () => request('/routes/presets'),
      () => mocks.mockPresetCatalog,
    ),

  createRoute: (body: {
    name?: string
    preset_id?: string
    domains?: string[]
    subnets?: string[]
    // A route needs one target: a keen-manager connection (target_conn_id) OR a
    // router interface picked from the live interface list (target_iface).
    target_conn_id?: string
    target_iface?: string
  }) =>
    withOk<RouteEntry>(
      () => request('/routes', { method: 'POST', body }),
      () => ({
        id: `route-${Date.now()}`,
        name: body.name ?? body.preset_id ?? 'Custom route',
        preset_id: body.preset_id,
        domain_count: body.domains?.length ?? 0,
        subnet_count: body.subnets?.length ?? 0,
        target_conn_id: body.target_conn_id,
        target_iface: body.target_iface,
        enabled: true,
        applied: false,
      }),
    ),

  // Full membership for opening a rule in the editor (list view carries counts).
  getRoute: (id: string) =>
    withMock<RouteDetail>(
      () => request(`/routes/${id}`),
      () => {
        const r =
          mocks.mockRoutes.find((x) => x.id === id) ?? mocks.mockRoutes[0]
        return { ...r, domains: [], subnets: [] }
      },
    ),

  updateRoute: (
    id: string,
    body: {
      name?: string
      domains?: string[]
      subnets?: string[]
      target_conn_id?: string
      target_iface?: string
    },
  ) =>
    withOk<RouteEntry>(
      () => request(`/routes/${id}`, { method: 'PUT', body }),
      () => ({
        id,
        name: body.name ?? 'Route',
        domain_count: body.domains?.length ?? 0,
        subnet_count: body.subnets?.length ?? 0,
        target_conn_id: body.target_conn_id,
        target_iface: body.target_iface,
        enabled: true,
        applied: false,
      }),
    ),

  toggleRoute: (id: string, enabled: boolean) =>
    withOk<Ok>(
      () => request(`/routes/${id}/toggle`, { method: 'PUT', body: { enabled } }),
      { ok: true },
    ),

  deleteRoute: (id: string) =>
    withOk<Ok>(() => request(`/routes/${id}`, { method: 'DELETE' }), {
      ok: true,
    }),

  // ---- remote list resolution (v2fly / plain / hosts) ----
  resolveList: (url: string, attr?: string) =>
    withOk<ListResolveResult>(
      () => request('/lists/resolve', { method: 'POST', body: { url, attr } }),
      () => ({ domains: [], skipped: [], sources: [], truncated: false, skipped_n: 0 }),
    ),

  // ---- nfqws2 ----
  nfqws: () => withMock<Nfqws>(() => request('/nfqws'), () => mocks.mockNfqws),

  nfqwsAction: (
    action: 'start' | 'stop' | 'restart' | 'reload' | 'install',
  ) =>
    withOk<Ok>(
      () => request('/nfqws/action', { method: 'POST', body: { action } }),
      { ok: true },
    ),

  nfqwsConfig: () =>
    withMock<NfqwsConfig>(
      () => request('/nfqws/config'),
      () => mocks.mockNfqwsConfig,
    ),

  saveNfqwsConfig: (body: Partial<NfqwsConfig>) =>
    withOk<Ok>(
      () => request('/nfqws/config', { method: 'PUT', body }),
      { ok: true },
    ),

  nfqwsConfigStructured: () =>
    withMock<NfqwsConf>(
      () => request('/nfqws/config/structured'),
      () => mocks.mockNfqwsConf,
    ),

  saveNfqwsConfigStructured: (body: Partial<NfqwsConf>) =>
    withOk<Ok>(
      () => request('/nfqws/config/structured', { method: 'PUT', body }),
      { ok: true },
    ),

  nfqwsLists: () =>
    withMock<string[]>(() => request('/nfqws/lists'), () => mocks.mockLists),

  nfqwsList: (name: string) =>
    withMock<NfqwsList>(
      () => request(`/nfqws/lists/${encodeURIComponent(name)}`),
      () => ({ name, content: mocks.mockListContent[name] ?? '' }),
    ),

  saveNfqwsList: (name: string, content: string) =>
    withOk<Ok>(
      () =>
        request(`/nfqws/lists/${encodeURIComponent(name)}`, {
          method: 'PUT',
          body: { content },
        }),
      { ok: true },
    ),

  // Import a remote domain list into the hostlists, auto-splitting a large set
  // across numbered sibling files (user.list, user2.list, …) server-side.
  importNfqwsList: (
    base: string,
    url: string,
    attr: string | undefined,
    mode: 'append' | 'replace',
  ) =>
    withOk<NfqwsImportResult>(
      () =>
        request('/nfqws/lists/import', {
          method: 'POST',
          body: { base, url, attr, mode },
        }),
      () => mocks.mockImportResult(base, mode),
    ),

  checkDomain: (domain: string) =>
    withOk<DomainCheck>(
      () => request('/nfqws/check-domain', { method: 'POST', body: { domain } }),
      () => mocks.mockDomainCheck(domain),
    ),

  // ---- DPI bypass (routable tpws interface) ----
  bypass: () =>
    withMock<Bypass>(() => request('/bypass'), () => mocks.mockBypass),

  saveBypass: (
    body: Partial<{ enabled: boolean; strategy: string; port: number }>,
  ) =>
    withOk<Bypass>(
      () => request('/bypass', { method: 'PUT', body }),
      () => ({ ...mocks.mockBypass, ...body }),
    ),

  // ---- failover ----
  failover: () =>
    withMock<Failover>(() => request('/failover'), () => mocks.mockFailover),

  saveFailover: (body: Failover) =>
    withOk<Ok>(
      () => request('/failover', { method: 'PUT', body }),
      { ok: true },
    ),

  // ---- settings ----
  settings: () =>
    withMock<Settings>(() => request('/settings'), () => mocks.mockSettings),

  saveSettings: (body: Partial<Settings>) =>
    withOk<Ok>(
      () => request('/settings', { method: 'PUT', body }),
      { ok: true },
    ),

  // Factory reset — wipe ALL configuration + secrets back to defaults and tear
  // down the managed tunnel/interfaces. Destructive; gate behind a confirm.
  resetSettings: () =>
    withOk<Ok>(
      () => request('/settings/reset', { method: 'POST' }),
      { ok: true },
    ),

  // ---- master connector switch (whole VPN egress on/off) ----
  setConnector: (enabled: boolean) =>
    withOk<Ok>(
      () => request('/connector', { method: 'POST', body: { enabled } }),
      { ok: true },
    ),

  // ---- logs ----
  logs: (service: LogService, lines: number) =>
    withMock<LogResponse>(
      () => request(`/logs?service=${service}&lines=${lines}`),
      () => mocks.mockLogs(service, lines),
    ),
}

export { UnauthorizedError }
