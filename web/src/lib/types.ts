// Shared API types for keen-manager. Mirror the Go daemon's JSON contract.

export type ConnType = 'awg' | 'xray'

export type ConnStatus =
  | 'up'
  | 'down'
  | 'degraded'
  | 'checking'
  | 'disabled'

export interface Conn {
  id: string
  type: ConnType
  name: string
  enabled: boolean
  status: ConnStatus
  active: boolean
  location?: string
  endpoint?: string
  latency_ms?: number
  last_check?: string // ISO
  subscription_id?: string
  fallback_to?: string
}

/** How a connection surfaces on the router (answer to "why don't I see it in
 * the Keenetic UI?"). Mirrors engine.IntegrationView. */
export interface Integration {
  /** "native-interface" | "userspace-awg" | "keenetic-proxy" |
   * "transparent-proxy". */
  mode: string
  visible_in_router: boolean
  /** Native NDMS interface name once up (e.g. "Wireguard1"); empty otherwise. */
  interface?: string
  summary: string
  /** Only native interfaces can back a dns-proxy route (Routes target). */
  routable_target: boolean
}

export interface ConnDetail extends Conn {
  config_preview?: string
  handshake_age_s?: number
  rx_bytes?: number
  tx_bytes?: number
  protocol?: string
  integration?: Integration
}

export interface Server {
  id: string
  name: string
  location: string
  address: string
  port: number
  protocol: string
  latency_ms?: number
  status: ConnStatus
  active: boolean
}

export interface SubUserInfo {
  used_bytes: number
  total_bytes: number
  expire?: string // ISO
}

export interface Sub {
  id: string
  name: string
  url: string
  host: string
  protocol: string
  server_count: number
  last_update?: string
  update_interval_hours?: number
  userinfo?: SubUserInfo
  auto_select_best: boolean
  /** Subscription-stream on/off — the middle of the three egress toggle levels
   * (master connector → subscription stream → per-connection Enabled). False =
   * this subscription's servers are excluded from activation, select-best,
   * failover and auto-select. */
  enabled: boolean
}

export type NfqwsMode = 'MODE_AUTO' | 'MODE_LIST' | 'MODE_ALL'

export interface Nfqws {
  installed: boolean
  running: boolean
  version?: string
  mode?: NfqwsMode
  /** Whether the NFQUEUE kernel modules are loaded/loadable. */
  kernel_ready?: boolean
  /** Required kernel modules neither loaded nor on disk (empty when ready). */
  missing_modules?: string[]
  /** Honest "actually working" signal: installed && running && kernel_ready. */
  healthy?: boolean
}

export interface NfqwsConfig {
  raw: string
  mode: NfqwsMode
}

/** GET/PUT /api/bypass: the routable DPI-bypass feature. tpws (zapret's
 * socket-level desync proxy) is exposed as ONE managed KeeneticOS Proxy
 * interface; domains are routed through it per-service from the Routes page
 * (target "bypass"), like a VPN tunnel, instead of a global inline NFQUEUE.
 * Mirrors engine.BypassView. */
export interface Bypass {
  enabled: boolean
  /** tpws binary present on the device. */
  installed: boolean
  /** tpws daemon currently running (best-effort; false off-device). */
  running: boolean
  /** Managed Proxy interface name once registered (e.g. "Proxy1"). */
  interface?: string
  /** Local tpws SOCKS port the Proxy interface points at. */
  port: number
  /** tpws desync argument string (tuned on-device). */
  strategy: string
  /** True once a route can bind to the bypass exit point. */
  routable: boolean
  /** Reserved Routes target id for the bypass interface ("bypass"). */
  target: string
  note?: string
}

/** Structured nfqws2.conf, mirrors internal/nfqws.Conf. Only the fields the
 * form edits are typed; the round-trip parser preserves everything else. */
export interface NfqwsConf {
  isp_interface: string
  nfqws_base_args: string
  nfqws_args: string
  nfqws_args_quic: string
  nfqws_args_udp: string
  nfqws_args_ipset: string
  nfqws_args_custom: string
  /** Active mode macro, e.g. "$MODE_AUTO". */
  nfqws_extra_args: string
  tcp_ports: string
  udp_ports: string
  policy_name: string
  policy_exclude: number
  nfqueue_num: number
  log_level: number
  ipv6_enabled: boolean
}

/** Result of resolving a remote domain-list URL (v2fly / plain / hosts). */
export interface ListResolveResult {
  domains: string[]
  skipped: string[]
  sources: string[]
  truncated: boolean
  skipped_n: number
}

export interface NfqwsList {
  name: string
  content: string
}

/** One hostlist file written by an import, with its domain count. */
export interface NfqwsImportFile {
  name: string
  count: number
}

/** Result of importing a remote domain list into the nfqws2 hostlists. A large
 * set is auto-split across numbered sibling files (user.list, user2.list, …).
 * Mirrors engine.NfqwsImportView. */
export interface NfqwsImportResult {
  base: string
  mode: 'append' | 'replace'
  files: NfqwsImportFile[]
  total: number
  per_file: number
  truncated: boolean
  skipped_n: number
  sources: string[]
}

export interface DomainCheck {
  domain: string
  direct_ok: boolean
  bypass_ok: boolean
  note?: string
}

export interface FailoverEvent {
  time: string
  from: string
  to: string
  reason: string
}

export interface Failover {
  enabled: boolean
  chain: string[] // conn ids; last element is "direct"
  current_index: number
  check_interval_s: number
  failure_threshold: number
  auto_return: boolean
  probe_target: string
  history: FailoverEvent[]
  /** nfqws-bypass guard: when on and on the direct path, a dead nfqws2 bypass
   * fails over to nfqws_fallback_to (a conn id, or "direct"). */
  nfqws_guard?: boolean
  nfqws_fallback_to?: string
  nfqws_probe_domains?: string[]
}

// ----- Routes / "Маршруты" (per-service domain routing) -----

/** One configured service route. Mirrors engine.RouteView. */
export interface RouteEntry {
  id: string
  name: string
  preset_id?: string
  category?: string
  icon?: string
  domain_count: number
  subnet_count: number
  target_conn_id?: string
  target_name?: string
  target_iface?: string
  enabled: boolean
  applied: boolean
  note?: string
}

/** A RouteEntry plus its full domain/subnet membership. Returned by
 * GET /api/routes/{id} so a rule can be opened and edited. Mirrors
 * engine.RouteDetailView. */
export interface RouteDetail extends RouteEntry {
  domains: string[]
  subnets: string[]
}

/** One router interface as reported live by KeeneticOS over RCI
 * (GET /api/interfaces). Mirrors engine.InterfaceView. Powers the "pick a
 * router interface" dropdown so a route can bind to a real device interface,
 * not just a keen-manager connection. */
export interface RouterInterface {
  /** NDMS interface id, e.g. "Wireguard0" — the value a route binds to. */
  name: string
  /** Human-friendly description when set, else the name. */
  label: string
  description?: string
  /** NDMS transport type ("Wireguard", "Bridge", …). */
  type: string
  up: boolean
  connected: boolean
  address?: string
  security?: string
  /** Native WireGuard/AmneziaWG interface. */
  is_wireguard: boolean
  /** KeeneticOS "Proxy" interface (Proxy client component) — including the one
   * keen-manager registers for the Xray exit point. */
  is_proxy: boolean
  /** Can back a Routes dns-proxy route (a WireGuard or Proxy interface that
   * isn't the router's own bundled VPN server). */
  routable: boolean
  /** keen-manager connection that created this interface, when applicable. */
  managed_conn_id?: string
}

/** GET /api/interfaces: the router's interfaces plus whether the firmware
 * exposes the native DNS-routing stack a route needs. Mirrors
 * engine.InterfacesView. */
export interface InterfacesView {
  interfaces: RouterInterface[]
  dns_routing_available: boolean
  /** Human explanation when the list is empty/degraded (RCI unreachable, or
   * running off-device). */
  note?: string
}

/** One entry in the built-in service catalog. Mirrors engine.PresetView. */
export interface Preset {
  id: string
  name: string
  category: string
  icon?: string
  notice?: string
  domain_count: number
  subnet_count: number
  has_subscription: boolean
}

export interface PresetCatalog {
  categories: string[]
  presets: Preset[]
}

export interface Wan {
  interface: string
  ip: string
  uptime_seconds: number
}

export interface AppState {
  active_connection_id?: string
  connections: Conn[]
  nfqws: Nfqws
  failover: Failover
  wan: Wan
  kill_switch: boolean
  /** Master VPN-egress switch. False = the LAN is intentionally on the direct
   * path and the loops won't bring a tunnel up on their own. */
  connector_enabled: boolean
  /** Whether the ndm netfilter.d hook that reapplies routing rules is present. */
  hook_installed?: boolean
}

export interface IfaceTraffic {
  name: string
  rx_bytes: number
  tx_bytes: number
}

/** Snapshot of per-interface cumulative byte counters (GET /api/traffic). The
 * UI diffs successive snapshots to derive live throughput. */
export interface Traffic {
  at: string
  interfaces: IfaceTraffic[]
}

export interface Platform {
  arch: string
  os_version: string
  entware_path: string
}

export interface Settings {
  port: number
  auth_enabled: boolean
  theme: 'dark' | 'light'
  backup_on_change: boolean
  rollback_timeout_s: number
  kill_switch_default: boolean
  auto_select_interval_min: number
  /** How an active Xray connection is wired to the router:
   * "auto" (default) | "proxy" (one visible KeeneticOS Proxy connection) |
   * "tproxy" (legacy transparent-proxy capture). */
  xray_integration: string
  /** Xray loglevel written into the generated config. "warning" (default) |
   * "debug" (surfaces the tunnel's own failure reason on activation errors) |
   * "info" | "error" | "none". */
  xray_log_level: string
  /** Outbound MSS clamp (Xray sockopt tcpMaxSeg). 0 or negative = off (no
   * clamp) — the default; positive = clamp to exactly that MSS. A manual
   * per-ISP override for "handshake OK but no payload" on reduced-MTU / TSPU
   * WANs; XKeen never clamps, so keen-manager leaves it off unless set. */
  xray_mss_clamp: number
  platform: Platform
}

export interface Capabilities {
  firmware?: string
  native_awg2: boolean
  wireguard: boolean
  proxy_client: boolean
  dns_route: boolean
}

export interface Health {
  status: string
  version: string
  arch: string
  os: string
  uptime_seconds: number
  capabilities: Capabilities
}

export interface AuthState {
  enabled: boolean
  authenticated: boolean
}

export interface LogResponse {
  service: string
  lines: string[]
}

export type LogService = 'keen-manager' | 'xray' | 'nfqws2' | 'awg'

export interface Ok {
  ok: boolean
  [k: string]: unknown
}
