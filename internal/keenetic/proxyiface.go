package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// This file drives KeeneticOS "Proxy" interfaces — the connections created by
// the Proxy client system component (Other Connections → Proxy Connections,
// HTTP/HTTPS/SOCKS5). keen-manager registers exactly one such interface
// ("ProxyN") pointing at its own local Xray SOCKS inbound (127.0.0.1:10808) so
// an Xray subscription shows up as ONE stable, visible connection in the router
// UI; switching server only rewrites the Xray config under the hood, never this
// interface.
//
// It mirrors the native-AWG interface helpers in iface.go (create / find-free /
// delete-via-DeleteInterface) so the engine's apply pipeline treats a Proxy
// exit point the same reversible way it treats a WireguardN one.
//
// ─────────────────────────────────────────────────────────────────────────────
// RCI SHAPE — informed by an on-device read-back (session 13); still degrades
// safely (see docs/XRAY-PROXY-PLAN.md §3/§6)
// ─────────────────────────────────────────────────────────────────────────────
// The Keenetic CLI documents the config-interface commands (proxy upstream
// <host> [<port>]; proxy protocol socks5; proxy connect; interface
// security-level …; up) but NOT the RCI JSON nesting. A read-back of the managed
// interface on the user's live router (KeeneticOS 5.1.0, arm64) was:
//
//	# curl -s http://localhost:79/rci/show/rc/interface/Proxy0
//	{"description":"keen-manager (Xray)","security-level":{"public":true},
//	 "ip":{"mtu":"1500","name-servers":true},
//	 "proxy":{"upstream":{"host":"127.0.0.1","port":"10808"}},"up":true}
//
// That confirmed the upstream/description/up nesting AND pinpointed the P0
// routing bug: KeeneticOS auto-assigns a connected proxy interface a GLOBAL
// (internet-access) priority and, for a public internet interface, makes it a
// DNS source — so it swallowed the entire LAN's traffic and the router's own DNS
// into the SOCKS tunnel, which (looping its own upstream back through itself)
// then carried nothing. Session 13 also nailed the write shapes on-device:
// clearing "ip global" returned "global priority cleared", and the zone must be
// written in OBJECT form ({"public":true}) — the bare-string form is rejected
// with "no input". proxyInterfaceBody / HardenProxyInterface now pin the
// anti-hijack invariant (ip global off, name-servers off for v4+v6) while
// KEEPING the interface "public" (the correct zone for an internet-egress proxy;
// the bug was the priority, not the zone) so it is a per-domain routing TARGET
// only. It is isolated in one function so the shape stays trivial to correct; a
// rejected write still surfaces as an error → the engine falls back to TPROXY.

// ProxyInterfaceName returns the canonical RCI interface name for index n, e.g.
// ProxyInterfaceName(0) == "Proxy0".
func ProxyInterfaceName(n int) string {
	return fmt.Sprintf("Proxy%d", n)
}

// parseProxyIndex extracts N from an interface name of the form "ProxyN"
// (case-insensitive on the prefix). Mirrors parseWireguardIndex.
func parseProxyIndex(name string) (int, bool) {
	const prefix = "proxy"
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, prefix) {
		return 0, false
	}
	suffix := name[len(prefix):]
	if suffix == "" {
		return 0, false
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// FindFreeProxyIndex scans "GET /show/interface/" for the first unused N in
// [0, maxInterfaceIndex] such that "ProxyN" does not exist. On a device with no
// Proxy interfaces it returns 0.
func FindFreeProxyIndex(ctx context.Context, c *Client) (int, error) {
	raw, err := c.Get(ctx, "/show/interface/")
	if err != nil {
		return 0, fmt.Errorf("keenetic: find free proxy index: %w", err)
	}

	var listing map[string]json.RawMessage
	if err := json.Unmarshal(raw, &listing); err != nil {
		return 0, fmt.Errorf("keenetic: decode /show/interface/: %w", err)
	}

	used := make(map[int]bool, len(listing))
	for name := range listing {
		if n, ok := parseProxyIndex(name); ok {
			used[n] = true
		}
	}

	for n := 0; n <= maxInterfaceIndex; n++ {
		if !used[n] {
			return n, nil
		}
	}
	return 0, fmt.Errorf("keenetic: no free Proxy interface index in [0,%d]", maxInterfaceIndex)
}

// ProxyConfig is the set of properties applied when a Proxy interface is
// created. keen-manager always points it at its own loopback SOCKS inbound, so
// Upstream is "127.0.0.1" and Protocol is "socks5" in practice.
type ProxyConfig struct {
	Upstream string // proxy server host, e.g. "127.0.0.1"
	Port     int    // proxy server port, e.g. 10808
	Protocol string // "socks5" | "http" | "https" (default "socks5")
	// UDP enables SOCKS5 UDP mode ("proxy socks5-udp"). Left off for a first cut
	// (TCP-only); DNS/UDP through the proxy may additionally need DoT/DoH or a
	// udpgw upstream — see docs/XRAY-PROXY-PLAN.md §2.
	UDP bool
	// SecurityLevel is the interface security zone, written in object form
	// ({"<level>":true}). keen-manager uses "public" — the correct zone for a
	// proxy that egresses to the internet. The managed proxy is kept out of the
	// router's default route not by its zone but by clearing its global
	// (internet-access) priority + DNS sourcing; see engine/proxyconn.go and
	// docs/XRAY-PROXY-PLAN.md §6.
	SecurityLevel string
	Description   string
	// Up brings the interface up and starts it connecting on creation.
	Up bool
}

// proxyInterfaceBody builds the inner RCI "interface".<name> object for a Proxy
// interface. Kept separate (and unit-tested) so the guessed shape is easy to
// diff against an on-device read-back and correct in exactly one place.
func proxyInterfaceBody(cfg ProxyConfig) map[string]any {
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = "socks5"
	}

	// proxy sub-block: "upstream <host> [<port>]" + "protocol <p>" + "connect".
	upstream := map[string]any{"host": cfg.Upstream}
	if cfg.Port > 0 {
		upstream["port"] = cfg.Port
	}
	proxy := map[string]any{
		"upstream": upstream,
		"protocol": protocol,
		"connect":  cfg.Up,
	}
	if cfg.UDP {
		proxy["socks5-udp"] = true
	}

	body := map[string]any{
		"proxy": proxy,
		"up":    cfg.Up,
		// Anti-hijack invariant (see engine/proxyconn.go + docs/XRAY-PROXY-PLAN.md
		// §6). The managed Proxy is a per-domain ROUTE TARGET, never the router's
		// internet uplink. KeeneticOS auto-assigns a connected proxy interface a
		// GLOBAL (internet-access) priority and, for a public internet interface,
		// makes it a DNS source — which turned it into a default route for the
		// whole LAN AND the router itself. Both must be forced off:
		//   • ip global       — the internet-access (default-route) priority.
		//                        A SOCKS proxy has no server-endpoint pinning, so as
		//                        a default it loops the router's own DNS + Xray's
		//                        server-upstream back through itself. Clear it. (On
		//                        the user's device this returned "global priority
		//                        cleared" — session 13.)
		//   • ip name-servers  — routes the router's DNS resolution through the
		//                        interface; over a TCP-only SOCKS that has stalled
		//                        that hangs every lookup system-wide. Force it OFF
		//                        for BOTH v4 (ip) and v6 (ipv6).
		"ip": map[string]any{
			"global":       false,
			"name-servers": false,
		},
		"ipv6": map[string]any{
			"name-servers": false,
		},
	}
	if cfg.Description != "" {
		body["description"] = cfg.Description
	}
	if cfg.SecurityLevel != "" {
		// The zone is a nested key ({"public":true}), NOT a bare string: the string
		// form is rejected by RCI with "no input" (confirmed on-device, session 13),
		// and the read-back reports the object form.
		body["security-level"] = map[string]any{cfg.SecurityLevel: true}
	}
	return body
}

// HardenProxyInterface re-applies the anti-hijack invariant to an EXISTING
// managed Proxy interface: clear its GLOBAL (internet-access / default-route)
// priority and stop it sourcing the router's DNS (v4 + v6). keen-manager creates
// the interface once and reuses it across server switches, so an interface left
// with the firmware's auto-assigned global priority hijacks the whole router's
// egress into the SOCKS tunnel. This heals it in place WITHOUT recreating the
// interface (recreation would churn the dns-proxy routes bound to it) and WITHOUT
// touching its security zone (a proxy is legitimately "public"; the bug was the
// priority, not the zone). Best-effort and fully reversible; a rejected write is
// returned so the caller can log and carry on.
func HardenProxyInterface(ctx context.Context, c *Client, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("keenetic: harden proxy interface: empty name")
	}
	body := map[string]any{
		"ip": map[string]any{
			"global":       false,
			"name-servers": false,
		},
		"ipv6": map[string]any{
			"name-servers": false,
		},
	}
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{name: body},
	})
	if err != nil {
		return fmt.Errorf("keenetic: harden proxy interface %s: %w", name, err)
	}
	return nil
}

// CreateProxyInterface creates (or reconfigures, if it already exists) a Proxy
// interface named name with cfg's properties. A rejected write comes back as an
// RCI error envelope (see client.go) and is returned as an error so the engine
// can fall back to TPROXY.
func CreateProxyInterface(ctx context.Context, c *Client, name string, cfg ProxyConfig) error {
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: proxyInterfaceBody(cfg),
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: create proxy interface %s: %w", name, err)
	}
	return nil
}

// SetProxyUpstream re-points an existing Proxy interface at host:port. Normally
// unnecessary for keen-manager (the upstream is always its constant loopback
// SOCKS inbound) — provided for completeness / future re-point flows.
func SetProxyUpstream(ctx context.Context, c *Client, name, host string, port int) error {
	upstream := map[string]any{"host": host}
	if port > 0 {
		upstream["port"] = port
	}
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{
				"proxy": map[string]any{"upstream": upstream},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: set proxy upstream on %s: %w", name, err)
	}
	return nil
}

// ProxyConnect issues "proxy connect" on a Proxy interface (optionally binding
// the outbound connection to a specific egress interface via "connect via
// <iface>"). An empty via means "any interface" (the default), which is what we
// want for a loopback upstream.
func ProxyConnect(ctx context.Context, c *Client, name, via string) error {
	connect := map[string]any{}
	proxy := map[string]any{"connect": true}
	if v := strings.TrimSpace(via); v != "" {
		connect["via"] = v
		proxy["connect"] = connect
	}
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{"proxy": proxy},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: proxy connect on %s: %w", name, err)
	}
	return nil
}
