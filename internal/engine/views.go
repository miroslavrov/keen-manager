package engine

import (
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// This file defines the JSON DTOs the HTTP API returns. They intentionally
// mirror the front-end contract in web/src/lib/types.ts exactly. Keeping them
// separate from the domain model lets the model evolve without breaking the API
// and, crucially, guarantees secrets in model.Server (json:"-") never leak here.

// isoOrEmpty renders a timestamp as RFC3339 (UTC), or "" when zero so the field
// is omitted and the UI shows a dash instead of the Go zero time.
func isoOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// statusStr maps the internal status to the five states the UI understands.
// model.StatusUnknown collapses to "checking" (a benign, transient look).
func statusStr(s model.Status) string {
	switch s {
	case model.StatusUp, model.StatusDown, model.StatusDegraded,
		model.StatusDisabled, model.StatusChecking:
		return string(s)
	default:
		return string(model.StatusChecking)
	}
}

// HealthView is GET /api/health.
type HealthView struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	Arch          string `json:"arch"`
	OS            string `json:"os"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// AuthStateView is GET /api/auth.
type AuthStateView struct {
	Enabled       bool `json:"enabled"`
	Authenticated bool `json:"authenticated"`
}

// ConnView is a connection list item.
type ConnView struct {
	ID             string         `json:"id"`
	Type           model.ConnType `json:"type"`
	Name           string         `json:"name"`
	Enabled        bool           `json:"enabled"`
	Status         string         `json:"status"`
	Active         bool           `json:"active"`
	Location       string         `json:"location,omitempty"`
	Endpoint       string         `json:"endpoint,omitempty"`
	LatencyMs      int            `json:"latency_ms,omitempty"`
	LastCheck      string         `json:"last_check,omitempty"`
	SubscriptionID string         `json:"subscription_id,omitempty"`
	FallbackTo     string         `json:"fallback_to,omitempty"`
}

// ConnDetailView adds config/traffic detail for the single-connection endpoint.
type ConnDetailView struct {
	ConnView
	ConfigPreview string `json:"config_preview,omitempty"`
	HandshakeAgeS int    `json:"handshake_age_s,omitempty"`
	RxBytes       int64  `json:"rx_bytes,omitempty"`
	TxBytes       int64  `json:"tx_bytes,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

// ServerView is one server inside a subscription.
type ServerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Status    string `json:"status"`
	Active    bool   `json:"active"`
}

// SubUserInfoView is the quota block some panels return.
type SubUserInfoView struct {
	UsedBytes  int64  `json:"used_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Expire     string `json:"expire,omitempty"`
}

// SubView is a subscription list item.
type SubView struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	URL                 string           `json:"url"`
	Host                string           `json:"host"`
	Protocol            string           `json:"protocol"`
	ServerCount         int              `json:"server_count"`
	LastUpdate          string           `json:"last_update,omitempty"`
	UpdateIntervalHours int              `json:"update_interval_hours,omitempty"`
	UserInfo            *SubUserInfoView `json:"userinfo,omitempty"`
	AutoSelectBest      bool             `json:"auto_select_best"`
}

// WanView is the router's upstream summary.
type WanView struct {
	Interface     string `json:"interface"`
	IP            string `json:"ip"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// StateView is the aggregate GET /api/state.
type StateView struct {
	ActiveConnectionID string                `json:"active_connection_id,omitempty"`
	Connections        []ConnView            `json:"connections"`
	Nfqws              model.NfqwsStatusView `json:"nfqws"`
	Failover           model.Failover        `json:"failover"`
	Wan                WanView               `json:"wan"`
	KillSwitch         bool                  `json:"kill_switch"`
}

// NfqwsConfigView is GET/PUT /api/nfqws/config.
type NfqwsConfigView struct {
	Raw  string          `json:"raw"`
	Mode model.NfqwsMode `json:"mode"`
}

// NfqwsListView is a single hostlist file.
type NfqwsListView struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// DomainCheckView is the domain-reachability probe result.
type DomainCheckView struct {
	Domain   string `json:"domain"`
	DirectOK bool   `json:"direct_ok"`
	BypassOK bool   `json:"bypass_ok"`
	Note     string `json:"note,omitempty"`
}

// PlatformView is the read-only device facts block inside settings.
type PlatformView struct {
	Arch        string `json:"arch"`
	OSVersion   string `json:"os_version"`
	EntwarePath string `json:"entware_path"`
}

// SettingsView is GET /api/settings.
type SettingsView struct {
	Port                  int          `json:"port"`
	AuthEnabled           bool         `json:"auth_enabled"`
	Theme                 string       `json:"theme"`
	BackupOnChange        bool         `json:"backup_on_change"`
	RollbackTimeoutS      int          `json:"rollback_timeout_s"`
	KillSwitchDefault     bool         `json:"kill_switch_default"`
	AutoSelectIntervalMin int          `json:"auto_select_interval_min"`
	Platform              PlatformView `json:"platform"`
}

// LogView is GET /api/logs.
type LogView struct {
	Service string   `json:"service"`
	Lines   []string `json:"lines"`
}

// okResult is the generic mutation acknowledgement.
type okResult map[string]any

func ok() okResult { return okResult{"ok": true} }
