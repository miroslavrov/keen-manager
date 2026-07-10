// Package model defines the shared domain types used across keen-manager.
// It has no internal dependencies so every other package can import it without
// risking an import cycle.
package model

import "time"

// ConnType is the kind of a connection.
type ConnType string

const (
	ConnAWG  ConnType = "awg"  // AmneziaWG tunnel
	ConnXray ConnType = "xray" // Xray outbound (vless/vmess/trojan/ss)
)

// Status is a runtime health state.
type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDegraded Status = "degraded"
	StatusChecking Status = "checking"
	StatusDisabled Status = "disabled"
	StatusUnknown  Status = "unknown"
)

// Protocol is an Xray transport protocol parsed from a share link.
type Protocol string

const (
	ProtoVLESS  Protocol = "vless"
	ProtoVMess  Protocol = "vmess"
	ProtoTrojan Protocol = "trojan"
	ProtoSS     Protocol = "shadowsocks"
)

// NfqwsMode mirrors the NFQWS_EXTRA_ARGS macro in nfqws2.conf.
type NfqwsMode string

const (
	ModeAuto NfqwsMode = "MODE_AUTO"
	ModeList NfqwsMode = "MODE_LIST"
	ModeAll  NfqwsMode = "MODE_ALL"
)

// Server describes a single Xray endpoint (one location of a subscription, or a
// manually added server). Fields map directly onto an Xray outbound.
type Server struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Location string   `json:"location,omitempty"`
	Protocol Protocol `json:"protocol"`
	Address  string   `json:"address"`
	Port     int      `json:"port"`

	// Credentials (protocol dependent)
	UUID     string `json:"-"` // vless/vmess
	Password string `json:"-"` // trojan/ss
	AlterID  int    `json:"-"` // vmess
	Cipher   string `json:"-"` // vmess security (scy) / ss method

	// Stream / security
	Flow        string   `json:"-"` // xtls-rprx-vision
	Security    string   `json:"-"` // none | tls | reality
	Network     string   `json:"-"` // tcp | ws | grpc | http | httpupgrade | xhttp | kcp
	SNI         string   `json:"-"`
	Fingerprint string   `json:"-"` // uTLS fp
	PublicKey   string   `json:"-"` // reality pbk
	ShortID     string   `json:"-"` // reality sid
	SpiderX     string   `json:"-"` // reality spx
	Path        string   `json:"-"` // ws/grpc/xhttp path or serviceName
	Host        string   `json:"-"` // ws Host header / http host
	ALPN        []string `json:"-"`
	AllowInsecure bool   `json:"-"`

	Raw string `json:"-"` // original share link (never sent to the UI)
}

// AWGPeer is the [Peer] section of an AmneziaWG config.
type AWGPeer struct {
	PublicKey           string   `json:"public_key"`
	PresharedKey        string   `json:"preshared_key,omitempty"`
	Endpoint            string   `json:"endpoint"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive,omitempty"`
}

// AWGConfig is a parsed AmneziaWG (wg-quick style) configuration, including the
// obfuscation parameters that distinguish AWG from plain WireGuard.
type AWGConfig struct {
	PrivateKey string   `json:"private_key"`
	Address    []string `json:"address"`
	DNS        []string `json:"dns,omitempty"`
	MTU        int      `json:"mtu,omitempty"`
	ListenPort int      `json:"listen_port,omitempty"`
	Peer       AWGPeer  `json:"peer"`

	// AmneziaWG obfuscation (must match the server exactly)
	Jc   int `json:"jc"`
	Jmin int `json:"jmin"`
	Jmax int `json:"jmax"`
	S1   int `json:"s1"`
	S2   int `json:"s2"`
	H1   int64 `json:"h1"`
	H2   int64 `json:"h2"`
	H3   int64 `json:"h3"`
	H4   int64 `json:"h4"`

	// AWG v2 extended (optional; may be rejected by older Keenetic firmware)
	S3 int    `json:"s3,omitempty"`
	S4 int    `json:"s4,omitempty"`
	I1 string `json:"i1,omitempty"`
	I2 string `json:"i2,omitempty"`
	I3 string `json:"i3,omitempty"`
	I4 string `json:"i4,omitempty"`
	I5 string `json:"i5,omitempty"`
}

// Connection is a persisted VPN connection (AWG or Xray).
type Connection struct {
	ID             string     `json:"id"`
	Type           ConnType   `json:"type"`
	Name           string     `json:"name"`
	Enabled        bool       `json:"enabled"`
	SubscriptionID string     `json:"subscription_id,omitempty"`
	FallbackTo     string     `json:"fallback_to,omitempty"`
	Xray           *Server    `json:"xray,omitempty"`
	AWG            *AWGConfig `json:"awg,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// SubUserInfo is the quota metadata some panels return in headers.
type SubUserInfo struct {
	UploadBytes   int64      `json:"upload_bytes"`
	DownloadBytes int64      `json:"download_bytes"`
	UsedBytes     int64      `json:"used_bytes"`
	TotalBytes    int64      `json:"total_bytes"`
	Expire        *time.Time `json:"expire,omitempty"`
}

// Subscription is a remote list of servers (an Xray subscription URL).
type Subscription struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	URL            string       `json:"url"`
	Host           string       `json:"host"`
	ServerCount    int          `json:"server_count"`
	LastUpdate     *time.Time   `json:"last_update,omitempty"`
	UpdateInterval int          `json:"update_interval_hours,omitempty"`
	UserInfo       *SubUserInfo `json:"userinfo,omitempty"`
	AutoSelectBest bool         `json:"auto_select_best"`
	// Enabled is the subscription-stream on/off switch — the middle of the three
	// egress toggle levels the user asked to untangle: (1) the master connector
	// (State.TunnelPaused) is the whole VPN egress, (2) THIS flag turns a single
	// subscription's whole fleet on/off as a routing source, (3) Connection.Enabled
	// toggles one server. A disabled subscription's servers are ineligible for
	// activation, select-best, failover and auto-select (see engine.connEligible).
	// It is written "positive" (true = on) like Connection.Enabled; existing
	// pre-schema-2 state has no such field, so config.migrate enables every
	// subscription on upgrade to avoid silently disabling a user's fleet.
	Enabled bool `json:"enabled"`
	// ServerIDs references the Connection IDs created from this subscription.
	ServerIDs []string `json:"server_ids,omitempty"`
}

// ServiceRoute sends a set of domains (and/or subnets) through a specific
// connection's native interface using Keenetic's dns-proxy route + object-group
// stack — the router's "Маршруты/DNS" section. A route may be built from a
// built-in service preset (PresetID) or from a custom domain list.
type ServiceRoute struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PresetID string `json:"preset_id,omitempty"`
	Category string `json:"category,omitempty"`
	Icon     string `json:"icon,omitempty"`
	// Domains / Subnets are the effective, resolved membership (a preset's
	// lists are snapshotted here at creation so a route is self-contained).
	Domains []string `json:"domains,omitempty"`
	Subnets []string `json:"subnets,omitempty"`
	// TargetConnID is the keen-manager connection whose native interface
	// receives the routed traffic. It must resolve to a KeeneticOS native
	// interface (an AWG2 WireguardN); Xray connections route transparently and
	// are not valid dns-proxy targets. Optional when TargetIface is set.
	TargetConnID string `json:"target_conn_id,omitempty"`
	// TargetIface binds the route directly to a KeeneticOS interface by name
	// (e.g. "Wireguard0"), independent of any keen-manager connection — used
	// when the user routes through a router interface they picked from the live
	// interface list (including WireGuard interfaces created in the Keenetic UI
	// itself). Takes precedence over TargetConnID when set.
	TargetIface string `json:"target_iface,omitempty"`
	Enabled     bool   `json:"enabled"`
	// Groups are the object-group names created on the router for this route,
	// recorded so the exact set can be torn down or reconciled later.
	Groups    []string  `json:"groups,omitempty"`
	Applied   bool      `json:"applied,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// FailoverEvent records one automatic switch.
type FailoverEvent struct {
	Time   time.Time `json:"time"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
}

// Failover configures the fallback chain engine.
type Failover struct {
	Enabled          bool            `json:"enabled"`
	Chain            []string        `json:"chain"` // connection IDs; last element may be "direct"
	CurrentIndex     int             `json:"current_index"`
	CheckIntervalS   int             `json:"check_interval_s"`
	FailureThreshold int             `json:"failure_threshold"`
	AutoReturn       bool            `json:"auto_return"`
	ProbeTarget      string          `json:"probe_target"`
	History          []FailoverEvent `json:"history,omitempty"`

	// nfqws guard: when NfqwsGuard is on and the device is on the direct path
	// (no active tunnel), a dead/inert nfqws2 DPI-bypass — daemon down, NFQUEUE
	// modules missing, or a probe of NfqwsProbeDomains failing on the direct
	// path — drives a fallback to NfqwsFallbackTo (a connection ID of an AWG
	// tunnel that routes around DPI). This makes "bypass strategy died → fall
	// back to AWG" automatic. Empty NfqwsFallbackTo disables the action.
	NfqwsGuard        bool     `json:"nfqws_guard,omitempty"`
	NfqwsFallbackTo   string   `json:"nfqws_fallback_to,omitempty"`
	NfqwsProbeDomains []string `json:"nfqws_probe_domains,omitempty"`
}

// Bypass configures the DPI-bypass "routable interface" feature: keen-manager
// runs a local tpws (zapret's socket-level desync proxy) as a SOCKS server on
// 127.0.0.1:Port and registers ONE managed KeeneticOS Proxy interface pointing
// at it (State.ManagedBypassIface). Chosen domains are then routed through it
// per-service from the Routes page — exactly like a VPN tunnel — instead of a
// global inline NFQUEUE. The desync Strategy lives here (edited on the Bypass
// page → Advanced); the domain selection is Routes (a route targeting the
// reserved "bypass" target), so there is a single source of truth for domains.
type Bypass struct {
	// Enabled turns the routable bypass interface on: start tpws + register the
	// managed Proxy interface. Off stops tpws and retires the interface.
	Enabled bool `json:"enabled"`
	// Port is the local tpws SOCKS port (0 → the tpws package default, 10809;
	// distinct from the Xray SOCKS inbound on 10808 so both can coexist).
	Port int `json:"port,omitempty"`
	// Strategy is the free-form tpws desync argument string. It is device- and
	// ISP-specific and tuned on-device; empty means the tpws package default.
	Strategy string `json:"strategy,omitempty"`
	// Seeded records that the default Discord + YouTube routes (from the preset
	// catalog) were created once on first enable, so re-enabling doesn't
	// duplicate them and the user can freely delete them.
	Seeded bool `json:"seeded,omitempty"`
}

// Platform captures detected device facts (read-only, filled at runtime).
type Platform struct {
	Arch        string `json:"arch"`         // mipsle | mips | arm64 | ...
	OSVersion   string `json:"os_version"`   // KeeneticOS version if detected
	EntwarePath string `json:"entware_path"` // usually /opt
	Model       string `json:"model,omitempty"`
}

// Settings holds user-configurable options.
type Settings struct {
	Port             int    `json:"port"`
	AuthEnabled      bool   `json:"auth_enabled"`
	// PasswordHash is PBKDF2-HMAC-SHA256 (see engine/settings.go). It is
	// json:"-" so it never reaches the UI, state.json, or state backups; the
	// engine persists it in the 0600 vault (servers.json) and reinstates it in
	// memory at startup. Do NOT rely on it surviving via state.json.
	PasswordHash     string `json:"-"`
	Theme            string `json:"theme"` // dark | light
	BackupOnChange   bool   `json:"backup_on_change"`
	// RollbackTimeoutS is how long a fresh activation is probed end-to-end before
	// it is rolled back as failed. 0 means "use the built-in default" (see
	// engine.normalizeRollbackTimeout); very small positive values are clamped up
	// so a rollback can never fire before one probe attempt completes.
	RollbackTimeoutS int    `json:"rollback_timeout_s"`
	KillSwitchDefault bool  `json:"kill_switch_default"`
	// AutoSelectIntervalMin: how often to re-evaluate best location (0 = manual).
	AutoSelectIntervalMin int `json:"auto_select_interval_min"`
	// XrayIntegration selects how an active Xray connection is wired to the
	// router:
	//   ""/"auto" — use a KeeneticOS Proxy connection when the Proxy client
	//               component is present, else TPROXY (the default);
	//   "proxy"   — force the Proxy-connection path (one visible ProxyN → the
	//               local Xray SOCKS inbound; per-service routing via dns-proxy);
	//   "tproxy"  — force the legacy transparent-proxy capture (invisible in the
	//               router UI, in-Xray split routing).
	// See docs/XRAY-PROXY-PLAN.md.
	XrayIntegration string `json:"xray_integration,omitempty"`

	// XrayLogLevel is the loglevel keen-manager writes into the generated Xray
	// config ("warning" default; "debug"/"info"/"error"/"none" also valid). A
	// debug level makes Xray record the real handshake/dial detail in its own
	// error log, which the activation path tails to explain a failed bring-up.
	// Empty means the built-in default (warning).
	XrayLogLevel string `json:"xray_log_level,omitempty"`

	// XrayMSSClamp controls TCP_MAXSEG on Xray's server outbound (see
	// xray.Sockopt.TCPMaxSeg). It was added in session 15 as a candidate fix for
	// "handshake OK but no payload", but session 16's cross-check against the
	// XKeen canon found XKeen never clamps the MSS and the dead-tunnel bug
	// reproduced both before and after the clamp — so the clamp is now OFF by
	// default and is a manual, per-ISP override only. Semantics:
	//   0  → OFF (no clamp) — the default;
	//   <0 → also OFF (kept so an install that explicitly stored -1 still works);
	//   >0 → clamp to exactly this MSS (bytes).
	// Tunable on the Settings page (DefaultXrayMSS is the suggested value); the
	// diagnostic script's PMTU probe measures the real value to use.
	XrayMSSClamp int `json:"xray_mss_clamp,omitempty"`
}

// DefaultXrayMSS is the MSS the Settings UI suggests when a user chooses to turn
// the outbound clamp on. It is NOT applied automatically (the clamp defaults
// off — see Settings.XrayMSSClamp). 1380 (an ~1420-byte IP MTU) clears typical
// PPPoE (1492) plus TSPU/tunnel overhead while costing negligible throughput.
// Tune per-ISP from the diagnostic's PMTU probe.
const DefaultXrayMSS = 1380

// State is the full persisted document.
type State struct {
	Connections   []Connection   `json:"connections"`
	Subscriptions []Subscription `json:"subscriptions"`
	Routes        []ServiceRoute `json:"routes,omitempty"`
	Failover      Failover       `json:"failover"`
	Settings      Settings       `json:"settings"`
	Bypass        Bypass         `json:"bypass"`
	ActiveConnID  string         `json:"active_conn_id"`
	KillSwitch    bool           `json:"kill_switch"`
	Version       int            `json:"schema_version"`

	// TunnelPaused is the master "connector" switch for the VPN egress (Xray/AWG),
	// the single on/off the user asked for alongside the per-route (subnet)
	// toggles: when true the active tunnel is torn down and the LAN egresses
	// direct, and the background loops (failover, auto-select, nfqws guard,
	// boot reconcile) will NOT bring a tunnel up on their own. The zero value is
	// false (enabled), so existing installs need no migration. Any interactive
	// activation (activate a connection, select-best) clears it — turning the
	// connector explicitly back on. Distinct from per-connection Enabled (which
	// removes a server from the pool) and from KillSwitch (leak prevention).
	TunnelPaused bool `json:"tunnel_paused,omitempty"`
	// PausedConnID remembers which connection was active when the connector was
	// switched off, so switching it back on restores that exact tunnel. Cleared
	// once consumed.
	PausedConnID string `json:"paused_conn_id,omitempty"`

	// NativeIfaces maps a connection ID to the KeeneticOS native Wireguard
	// interface name (e.g. "Wireguard1") created for it via RCI import. It is
	// present only for AWG connections brought up on the native AWG2 path
	// (firmware >= 5.01.A.3); absent for the Entware userspace (awg-quick) path.
	// Persisted so the interface can be torn down or reconciled after a restart.
	NativeIfaces map[string]string `json:"native_ifaces,omitempty"`

	// ManagedProxyIface is the single KeeneticOS "Proxy" interface (e.g.
	// "Proxy0") keen-manager registers for the Xray proxy-connection model:
	// ProxyN → the local Xray SOCKS inbound (127.0.0.1:10808). It is shared by
	// every Xray connection (there is one exit point); switching server only
	// rewrites the Xray config, never this interface. Empty until the first
	// Xray activation in proxy mode creates it; persisted so it can be
	// reconciled or torn down after a restart. Only set on the native
	// Proxy-connection path (not the TPROXY fallback).
	ManagedProxyIface string `json:"managed_proxy_iface,omitempty"`

	// ManagedBypassIface is the single KeeneticOS "Proxy" interface keen-manager
	// registers for the DPI-bypass exit point: ProxyN → the local tpws SOCKS
	// listener (127.0.0.1:Bypass.Port). It is the exact analogue of
	// ManagedProxyIface but for the tpws desync proxy rather than Xray, and is
	// governed by the same anti-loop rule (never marked "use for internet
	// access" — it is a per-domain routing target only). Empty until the bypass
	// feature is first enabled; persisted so it can be reconciled/torn down after
	// a restart.
	ManagedBypassIface string `json:"managed_bypass_iface,omitempty"`
}

// NfqwsStatusView is the UI-facing status of the nfqws2 service.
type NfqwsStatusView struct {
	Installed bool      `json:"installed"`
	Running   bool      `json:"running"`
	Version   string    `json:"version,omitempty"`
	Mode      NfqwsMode `json:"mode,omitempty"`
	// KernelReady reports whether the netfilter modules nfqws2 needs
	// (nfnetlink_queue, xt_NFQUEUE) are loaded or loadable on the device.
	KernelReady bool `json:"kernel_ready"`
	// MissingModules lists any required kernel modules that are neither loaded
	// nor present on disk (empty when KernelReady).
	MissingModules []string `json:"missing_modules,omitempty"`
	// Healthy is the honest "actually working" signal: installed AND running AND
	// the kernel modules are present. A running daemon without its NFQUEUE
	// modules is up but inert, so it is reported unhealthy.
	Healthy bool `json:"healthy"`
}

// RuntimeStatus is in-memory, non-persisted per-connection health.
type RuntimeStatus struct {
	ConnID       string    `json:"id"`
	Status       Status    `json:"status"`
	LatencyMs    int       `json:"latency_ms"`
	LastCheck    time.Time `json:"last_check"`
	HandshakeAge int       `json:"handshake_age_s"`
	RxBytes      int64     `json:"rx_bytes"`
	TxBytes      int64     `json:"tx_bytes"`
	Active       bool      `json:"active"`
	Message      string    `json:"message,omitempty"`
}
