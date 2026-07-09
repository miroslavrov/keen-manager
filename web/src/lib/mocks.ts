// Realistic mock data. Used as a graceful fallback whenever a live API call
// fails or returns non-OK, so the UI stays fully browsable in development and
// in screenshots. Nothing here should look like placeholder lorem ipsum.

import type {
  AppState,
  AuthState,
  Conn,
  ConnDetail,
  DomainCheck,
  Failover,
  Health,
  LogResponse,
  Nfqws,
  NfqwsConfig,
  Server,
  Settings,
  Sub,
} from './types'

const now = Date.now()
const iso = (secAgo: number) => new Date(now - secAgo * 1000).toISOString()

export const mockConnections: Conn[] = [
  {
    id: 'awg-nl-ams',
    type: 'awg',
    name: 'Amnezia NL-1',
    enabled: true,
    status: 'up',
    active: true,
    location: 'Netherlands, Amsterdam',
    endpoint: '109.163.239.98:51820',
    latency_ms: 42,
    last_check: iso(9),
    fallback_to: 'xray-de-fra',
  },
  {
    id: 'xray-de-fra',
    type: 'xray',
    name: 'vless-reality DE',
    enabled: true,
    status: 'up',
    active: false,
    location: 'Germany, Frankfurt',
    endpoint: '45.132.207.14:443',
    latency_ms: 68,
    last_check: iso(11),
    subscription_id: 'sub-oceanlink',
    fallback_to: 'awg-se-sto',
  },
  {
    id: 'awg-se-sto',
    type: 'awg',
    name: 'Amnezia SE-1',
    enabled: true,
    status: 'degraded',
    active: false,
    location: 'Sweden, Stockholm',
    endpoint: '193.138.218.74:51820',
    latency_ms: 187,
    last_check: iso(14),
    fallback_to: 'xray-fi-hel',
  },
  {
    id: 'xray-fi-hel',
    type: 'xray',
    name: 'trojan FI',
    enabled: true,
    status: 'up',
    active: false,
    location: 'Finland, Helsinki',
    endpoint: '95.216.44.19:8443',
    latency_ms: 74,
    last_check: iso(12),
    subscription_id: 'sub-oceanlink',
  },
  {
    id: 'xray-us-nyc',
    type: 'xray',
    name: 'vmess US-East',
    enabled: true,
    status: 'down',
    active: false,
    location: 'United States, New York',
    endpoint: '162.55.90.201:2087',
    latency_ms: undefined,
    last_check: iso(20),
    subscription_id: 'sub-skyroute',
  },
  {
    id: 'awg-lab',
    type: 'awg',
    name: 'Self-hosted Lab',
    enabled: false,
    status: 'disabled',
    active: false,
    location: 'Local, Home DC',
    endpoint: '10.8.0.1:51820',
    latency_ms: undefined,
    last_check: iso(3600),
  },
]

export const mockNfqws: Nfqws = {
  installed: true,
  running: true,
  version: 'nfqws2 v1.4.2 (entware)',
  mode: 'MODE_AUTO',
}

export const mockFailover: Failover = {
  enabled: true,
  chain: ['awg-nl-ams', 'xray-de-fra', 'awg-se-sto', 'direct'],
  current_index: 0,
  check_interval_s: 30,
  failure_threshold: 3,
  auto_return: true,
  probe_target: 'https://www.gstatic.com/generate_204',
  history: [
    {
      time: iso(240),
      from: 'xray-de-fra',
      to: 'awg-nl-ams',
      reason: 'Primary recovered — auto-return',
    },
    {
      time: iso(1820),
      from: 'awg-nl-ams',
      to: 'xray-de-fra',
      reason: '3 consecutive probe failures (handshake stalled)',
    },
    {
      time: iso(7400),
      from: 'awg-se-sto',
      to: 'awg-nl-ams',
      reason: 'Manual switch to best location',
    },
  ],
}

export const mockState: AppState = {
  active_connection_id: 'awg-nl-ams',
  connections: mockConnections,
  nfqws: mockNfqws,
  failover: mockFailover,
  wan: {
    interface: 'eth3 (PPPoE)',
    ip: '188.170.82.41',
    uptime_seconds: 412_338,
  },
  kill_switch: false,
}

export const mockConnDetails: Record<string, ConnDetail> = {
  'awg-nl-ams': {
    ...mockConnections[0],
    protocol: 'AmneziaWG',
    handshake_age_s: 24,
    rx_bytes: 8_643_221_004,
    tx_bytes: 1_204_889_311,
    config_preview: `[Interface]
Address = 10.13.13.2/32
DNS = 1.1.1.1
Jc = 4
Jmin = 40
Jmax = 70
S1 = 86
S2 = 122
H1 = 1088686601

[Peer]
PublicKey = 3xO8v…redacted…u1Qk=
Endpoint = 109.163.239.98:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25`,
  },
  'xray-de-fra': {
    ...mockConnections[1],
    protocol: 'VLESS + REALITY',
    rx_bytes: 2_118_004_552,
    tx_bytes: 402_113_900,
    config_preview: `{
  "protocol": "vless",
  "address": "45.132.207.14",
  "port": 443,
  "id": "b1d9…redacted…4a2f",
  "flow": "xtls-rprx-vision",
  "security": "reality",
  "sni": "www.microsoft.com",
  "fp": "chrome"
}`,
  },
}

export const mockSubscriptions: Sub[] = [
  {
    id: 'sub-oceanlink',
    name: 'OceanLink Premium',
    url: 'https://oceanlink.example/sub/9f3c2a1b',
    host: 'oceanlink.example',
    protocol: 'mixed',
    server_count: 18,
    last_update: iso(3600),
    update_interval_hours: 12,
    userinfo: {
      used_bytes: 214_748_364_800,
      total_bytes: 536_870_912_000,
      expire: iso(-60 * 60 * 24 * 47),
    },
    auto_select_best: true,
  },
  {
    id: 'sub-skyroute',
    name: 'SkyRoute',
    url: 'https://panel.skyroute.example/api/v1/client/subscribe?token=3b1f',
    host: 'panel.skyroute.example',
    protocol: 'vmess',
    server_count: 9,
    last_update: iso(86400),
    update_interval_hours: 24,
    userinfo: {
      used_bytes: 96_636_764_160,
      total_bytes: 107_374_182_400,
      expire: iso(-60 * 60 * 24 * 6),
    },
    auto_select_best: false,
  },
]

export const mockServers: Record<string, Server[]> = {
  'sub-oceanlink': [
    {
      id: 'ol-de-fra',
      name: 'DE Frankfurt Reality',
      location: 'Germany, Frankfurt',
      address: '45.132.207.14',
      port: 443,
      protocol: 'vless-reality',
      latency_ms: 68,
      status: 'up',
      active: true,
    },
    {
      id: 'ol-fi-hel',
      name: 'FI Helsinki',
      location: 'Finland, Helsinki',
      address: '95.216.44.19',
      port: 8443,
      protocol: 'trojan',
      latency_ms: 74,
      status: 'up',
      active: false,
    },
    {
      id: 'ol-nl-ams',
      name: 'NL Amsterdam',
      location: 'Netherlands, Amsterdam',
      address: '185.65.135.230',
      port: 443,
      protocol: 'vless-ws',
      latency_ms: 51,
      status: 'up',
      active: false,
    },
    {
      id: 'ol-uk-lon',
      name: 'UK London',
      location: 'United Kingdom, London',
      address: '51.89.201.7',
      port: 2053,
      protocol: 'vless-grpc',
      latency_ms: 133,
      status: 'degraded',
      active: false,
    },
    {
      id: 'ol-us-lax',
      name: 'US Los Angeles',
      location: 'United States, Los Angeles',
      address: '104.234.90.16',
      port: 443,
      protocol: 'vless-reality',
      latency_ms: 214,
      status: 'up',
      active: false,
    },
  ],
  'sub-skyroute': [
    {
      id: 'sr-us-nyc',
      name: 'US New York',
      location: 'United States, New York',
      address: '162.55.90.201',
      port: 2087,
      protocol: 'vmess-ws-tls',
      latency_ms: undefined,
      status: 'down',
      active: false,
    },
    {
      id: 'sr-sg-sin',
      name: 'SG Singapore',
      location: 'Singapore',
      address: '128.199.180.44',
      port: 443,
      protocol: 'vmess-ws-tls',
      latency_ms: 246,
      status: 'up',
      active: true,
    },
    {
      id: 'sr-jp-tok',
      name: 'JP Tokyo',
      location: 'Japan, Tokyo',
      address: '160.16.208.99',
      port: 8443,
      protocol: 'vmess-grpc',
      latency_ms: 198,
      status: 'up',
      active: false,
    },
  ],
}

export const mockNfqwsConfig: NfqwsConfig = {
  mode: 'MODE_AUTO',
  raw: `# nfqws2 configuration (keen-manager managed)
# Mode is controlled from the UI segmented control.

NFQWS_ARGS="--dpi-desync=fake,split2 --dpi-desync-ttl=6 --dpi-desync-fooling=badseq"
NFQWS_ARGS_QUIC="--dpi-desync=fake --dpi-desync-repeats=6"

# Ports intercepted
TCP_PORTS=80,443
UDP_PORTS=443,50000-50100

# Hostlist mode: use auto + user lists, honor exclude + ipset
HOSTLIST_AUTO=1
HOSTLIST_NOAUTO=exclude.list
HOSTLIST=user.list

# Frag/desync tuning
DESYNC_MARK=0x40000000
FLOW_OFFLOAD=none`,
}

export const mockLists: string[] = [
  'user.list',
  'auto.list',
  'exclude.list',
  'ipset.list',
]

export const mockListContent: Record<string, string> = {
  'user.list': `# Domains you always want bypassed
youtube.com
googlevideo.com
discord.com
discordapp.com
x.com
twitter.com
instagram.com
cdninstagram.com`,
  'auto.list': `# Auto-learned from blocked-detection (managed by nfqws2)
rutracker.org
notion.so
medium.com
signal.org
telegram.org`,
  'exclude.list': `# Never bypass these (kept direct)
local.lan
gosuslugi.ru
mos.ru`,
  'ipset.list': `# IP / CIDR bypass targets
149.154.160.0/20
91.108.4.0/22
2001:67c:4e8::/48`,
}

export const mockHealth: Health = {
  status: 'ok',
  version: '0.4.1',
  arch: 'mipsel',
  os: 'KeeneticOS 4.2.3 (NDMS)',
  uptime_seconds: 412_902,
}

export const mockAuth: AuthState = {
  enabled: false,
  authenticated: true,
}

export const mockSettings: Settings = {
  port: 47115,
  auth_enabled: false,
  theme: 'dark',
  backup_on_change: true,
  rollback_timeout_s: 90,
  kill_switch_default: false,
  auto_select_interval_min: 30,
  platform: {
    arch: 'mipsel',
    os_version: 'KeeneticOS 4.2.3',
    entware_path: '/opt',
  },
}

export const mockDomainCheck = (domain: string): DomainCheck => ({
  domain,
  direct_ok: false,
  bypass_ok: true,
  note: 'Blocked by ISP DPI on direct path; reachable through nfqws2 desync.',
})

const LOG_SAMPLES: Record<string, string[]> = {
  'keen-manager': [
    'INFO  [state] active connection = awg-nl-ams (up, 42ms)',
    'INFO  [health] probe https://www.gstatic.com/generate_204 -> 204 in 41ms',
    'INFO  [failover] primary healthy, index=0, no action',
    'WARN  [health] xray-us-nyc probe timeout after 5000ms (attempt 2/3)',
    'INFO  [sub] OceanLink Premium refreshed: 18 servers, 214.7GB/512GB used',
    'DEBUG [route] default route bound to nwg0 (awg-nl-ams)',
    'INFO  [nfqws] reloaded strategy set, 132 hostlist entries active',
    'ERROR [xray-us-nyc] dial tcp 162.55.90.201:2087: connect: connection refused',
  ],
  xray: [
    'INFO  transport/internet: dialing to tcp:45.132.207.14:443',
    'INFO  proxy/vless/outbound: tunneling request to www.microsoft.com:443',
    'WARN  app/proxyman/outbound: REALITY handshake retry (spiderX mismatch)',
    'INFO  app/dispatcher: taking detour [proxy] for [tcp:youtube.com:443]',
  ],
  nfqws2: [
    'nfqws: desync profile MODE_AUTO loaded (fake,split2 ttl=6)',
    'nfqws: hostlist reload — user.list(8) auto.list(5) exclude.list(3)',
    'nfqws: packet mark 0x40000000 bound to queue 200',
    'nfqws: QUIC desync active on udp/443',
  ],
  awg: [
    'awg: interface nwg0 up, peer 109.163.239.98:51820',
    'awg: handshake completed in 0.24s',
    'awg: tx 1.2GB rx 8.6GB, last handshake 24s ago',
    'awg: junk packets Jc=4 S1=86 S2=122 applied',
  ],
}

export function mockLogs(service: string, lines: number): LogResponse {
  const base = LOG_SAMPLES[service] ?? LOG_SAMPLES['keen-manager']
  const out: string[] = []
  const stamp = (i: number) =>
    new Date(now - (lines - i) * 1400).toISOString().replace('T', ' ').slice(0, 19)
  for (let i = 0; i < lines; i++) {
    const msg = base[i % base.length]
    out.push(`${stamp(i)}  ${msg}`)
  }
  return { service, lines: out }
}

export function mockLogLine(service: string): string {
  const base = LOG_SAMPLES[service] ?? LOG_SAMPLES['keen-manager']
  const msg = base[Math.floor(Math.random() * base.length)]
  const stamp = new Date().toISOString().replace('T', ' ').slice(0, 19)
  return `${stamp}  ${msg}`
}
