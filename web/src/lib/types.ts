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
  /** "native-interface" | "userspace-awg" | "transparent-proxy". */
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
}

export type NfqwsMode = 'MODE_AUTO' | 'MODE_LIST' | 'MODE_ALL'

export interface Nfqws {
  installed: boolean
  running: boolean
  version?: string
  mode?: NfqwsMode
}

export interface NfqwsConfig {
  raw: string
  mode: NfqwsMode
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
  target_conn_id: string
  target_name?: string
  target_iface?: string
  enabled: boolean
  applied: boolean
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
  platform: Platform
}

export interface Health {
  status: string
  version: string
  arch: string
  os: string
  uptime_seconds: number
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
