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
	// ServerIDs references the Connection IDs created from this subscription.
	ServerIDs []string `json:"server_ids,omitempty"`
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
	PasswordHash     string `json:"-"` // bcrypt-like; never serialized to the UI
	Theme            string `json:"theme"` // dark | light
	BackupOnChange   bool   `json:"backup_on_change"`
	RollbackTimeoutS int    `json:"rollback_timeout_s"`
	KillSwitchDefault bool  `json:"kill_switch_default"`
	// AutoSelectIntervalMin: how often to re-evaluate best location (0 = manual).
	AutoSelectIntervalMin int `json:"auto_select_interval_min"`
}

// State is the full persisted document.
type State struct {
	Connections   []Connection   `json:"connections"`
	Subscriptions []Subscription `json:"subscriptions"`
	Failover      Failover       `json:"failover"`
	Settings      Settings       `json:"settings"`
	ActiveConnID  string         `json:"active_conn_id"`
	KillSwitch    bool           `json:"kill_switch"`
	Version       int            `json:"schema_version"`

	// NativeIfaces maps a connection ID to the KeeneticOS native Wireguard
	// interface name (e.g. "Wireguard1") created for it via RCI import. It is
	// present only for AWG connections brought up on the native AWG2 path
	// (firmware >= 5.01.A.3); absent for the Entware userspace (awg-quick) path.
	// Persisted so the interface can be torn down or reconciled after a restart.
	NativeIfaces map[string]string `json:"native_ifaces,omitempty"`
}

// NfqwsStatusView is the UI-facing status of the nfqws2 service.
type NfqwsStatusView struct {
	Installed bool      `json:"installed"`
	Running   bool      `json:"running"`
	Version   string    `json:"version,omitempty"`
	Mode      NfqwsMode `json:"mode,omitempty"`
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
