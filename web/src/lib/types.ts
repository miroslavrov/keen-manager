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

export interface ConnDetail extends Conn {
  config_preview?: string
  handshake_age_s?: number
  rx_bytes?: number
  tx_bytes?: number
  protocol?: string
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
