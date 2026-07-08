package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// maxInterfaceIndex bounds the "Wireguard{N}" scan performed by
// FindFreeIndex. Keenetic does not document a hard ceiling, but 100 native
// WireGuard interfaces is far beyond anything a real device configures.
const maxInterfaceIndex = 99

// FindFreeIndex scans "GET /show/interface/" for the first unused N in
// [0, maxInterfaceIndex] such that interface "Wireguard{N}" does not exist.
// On a fresh device with no Wireguard interfaces at all, it returns 0.
func FindFreeIndex(ctx context.Context, c *Client) (int, error) {
	raw, err := c.Get(ctx, "/show/interface/")
	if err != nil {
		return 0, fmt.Errorf("keenetic: find free index: %w", err)
	}

	var listing map[string]json.RawMessage
	if err := json.Unmarshal(raw, &listing); err != nil {
		return 0, fmt.Errorf("keenetic: decode /show/interface/: %w", err)
	}

	used := make(map[int]bool, len(listing))
	for name := range listing {
		if n, ok := parseWireguardIndex(name); ok {
			used[n] = true
		}
	}

	for n := 0; n <= maxInterfaceIndex; n++ {
		if !used[n] {
			return n, nil
		}
	}
	return 0, fmt.Errorf("keenetic: no free Wireguard interface index in [0,%d]", maxInterfaceIndex)
}

// parseWireguardIndex extracts N from an interface name of the form
// "WireguardN" (case-insensitive on the prefix, since RCI has been observed
// to title-case it consistently as "Wireguard" but we don't want a firmware
// quirk to break the scan).
func parseWireguardIndex(name string) (int, bool) {
	const prefix = "wireguard"
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

// InterfaceName returns the canonical RCI interface name for index n, e.g.
// InterfaceName(3) == "Wireguard3".
func InterfaceName(n int) string {
	return fmt.Sprintf("Wireguard%d", n)
}

// InterfaceConfig is the set of base (non-AWG-specific) properties applied
// when an interface is created.
type InterfaceConfig struct {
	Description string
	ListenPort  int
	Address     string // e.g. "10.0.0.1"
	Mask        string // e.g. "255.255.255.0"
	MTU         int
	// Up, when true, brings the interface up immediately on creation. Most
	// callers want this so the interface is usable without a second call.
	Up bool
}

// CreateInterface creates (or reconfigures, if it already exists) a native
// Wireguard interface named name (see InterfaceName), applying cfg's base
// properties. It does not set AWG obfuscation parameters or peers -- see
// SetASC and AddPeer for those.
func CreateInterface(ctx context.Context, c *Client, name string, cfg InterfaceConfig) error {
	ifaceBody := map[string]any{}
	if cfg.Description != "" {
		ifaceBody["description"] = cfg.Description
	}
	if cfg.ListenPort > 0 {
		ifaceBody["listen-port"] = cfg.ListenPort
	}
	if cfg.Address != "" {
		ip := map[string]any{"address": cfg.Address}
		if cfg.Mask != "" {
			ip["mask"] = cfg.Mask
		}
		ifaceBody["ip"] = ip
	}
	if cfg.MTU > 0 {
		ifaceBody["mtu"] = cfg.MTU
	}
	// "up" is always sent explicitly (even when false) so callers can create
	// an interface deliberately administratively-down.
	ifaceBody["up"] = cfg.Up

	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: ifaceBody,
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: create interface %s: %w", name, err)
	}
	return nil
}

// DeleteInterface removes interface name entirely.
func DeleteInterface(ctx context.Context, c *Client, name string) error {
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{"no": true},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: delete interface %s: %w", name, err)
	}
	return nil
}

// InterfaceUp administratively enables interface name.
func InterfaceUp(ctx context.Context, c *Client, name string) error {
	return setInterfaceUp(ctx, c, name, true)
}

// InterfaceDown administratively disables interface name.
func InterfaceDown(ctx context.Context, c *Client, name string) error {
	return setInterfaceUp(ctx, c, name, false)
}

func setInterfaceUp(ctx context.Context, c *Client, name string, up bool) error {
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{"up": up},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: set interface %s up=%v: %w", name, up, err)
	}
	return nil
}

// ASCParams are the AmneziaWG obfuscation parameters as RCI's "asc"
// (AmneziaWG Security Configuration) object expects them.
//
// The field set is identical between AWG1.0 and AWG2 except that AWG2 adds
// S3/S4 (and its H1-H4 values may additionally be expressed as "min-max"
// ranges rather than a single decimal -- ASCFromAWGConfig always emits plain
// decimals since that is what model.AWGConfig stores). S3/S4 are pointers so
// that "unset" (AWG1.0 config) is distinguishable from "explicitly zero"
// (a valid AWG2 value): a non-nil S3/S4 signals "this is an AWG2 request" to
// SetASC, which enforces the capability gate.
type ASCParams struct {
	Jc, Jmin, Jmax int
	S1, S2         int
	// H1-H4 are decimal strings (not "min-max" ranges) since model.AWGConfig
	// stores them as fixed int64 magic-header values.
	H1, H2, H3, H4 string
	// S3, S4 are AWG2-only. Leave nil for an AWG1.0 request.
	S3, S4 *int
}

// ASCFromAWGConfig maps a model.AWGConfig's obfuscation fields onto
// ASCParams. S3/S4 are only populated (as non-nil) when caps.SupportsAWG2 is
// true AND the source config actually sets them (a zero value in AWGConfig's
// S3/S4 is ambiguous with "unset" since those fields have no separate
// presence flag, so a zero is treated as unset -- callers that need an
// explicit AWG2 zero should build ASCParams directly instead of going through
// this helper).
func ASCFromAWGConfig(m model.AWGConfig, caps Capabilities) ASCParams {
	p := ASCParams{
		Jc:   m.Jc,
		Jmin: m.Jmin,
		Jmax: m.Jmax,
		S1:   m.S1,
		S2:   m.S2,
		H1:   strconv.FormatInt(m.H1, 10),
		H2:   strconv.FormatInt(m.H2, 10),
		H3:   strconv.FormatInt(m.H3, 10),
		H4:   strconv.FormatInt(m.H4, 10),
	}
	if caps.SupportsAWG2 {
		if m.S3 != 0 {
			s3 := m.S3
			p.S3 = &s3
		}
		if m.S4 != 0 {
			s4 := m.S4
			p.S4 = &s4
		}
	}
	return p
}

// ascVerifyAttempts / ascVerifyDelay bound the read-back retry loop SetASC
// uses to confirm RCI actually applied the ASC it was just sent (NDMS applies
// wireguard config asynchronously in some firmware versions, so the very next
// read can still show the old value for a moment).
const (
	ascVerifyAttempts = 5
	ascVerifyDelay    = 150 * time.Millisecond
)

// SetASC applies AmneziaWG obfuscation parameters to interface name and reads
// them back to confirm they took effect.
//
// If p.S3 (or p.S4) is non-nil but caps.SupportsAWG2 is false, SetASC returns
// an error instead of silently dropping the AWG2-only fields -- sending them
// to firmware that doesn't understand "asc.s3"/"asc.s4" produces a rejected
// write, and dropping them instead would silently downgrade the tunnel's
// obfuscation profile, which the caller needs to know about explicitly.
func SetASC(ctx context.Context, c *Client, name string, p ASCParams, caps Capabilities) error {
	if (p.S3 != nil || p.S4 != nil) && !caps.SupportsAWG2 {
		return fmt.Errorf("keenetic: interface %s: ASC requests AWG2 fields (s3/s4) but firmware %q does not support AWG2 (needs >= 5.01.A.3)", name, caps.Release)
	}

	asc := map[string]string{
		"jc":   strconv.Itoa(p.Jc),
		"jmin": strconv.Itoa(p.Jmin),
		"jmax": strconv.Itoa(p.Jmax),
		"s1":   strconv.Itoa(p.S1),
		"s2":   strconv.Itoa(p.S2),
		"h1":   p.H1,
		"h2":   p.H2,
		"h3":   p.H3,
		"h4":   p.H4,
	}
	if p.S3 != nil {
		asc["s3"] = strconv.Itoa(*p.S3)
	}
	if p.S4 != nil {
		asc["s4"] = strconv.Itoa(*p.S4)
	}

	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{
				"wireguard": map[string]any{
					"asc": asc,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: set asc on %s: %w", name, err)
	}

	if err := verifyASC(ctx, c, name, p.Jc); err != nil {
		return fmt.Errorf("keenetic: verify asc on %s: %w", name, err)
	}
	return nil
}

// ascReadback is the shape of "GET /show/rc/interface/{name}/wireguard/asc".
type ascReadback struct {
	Jc string `json:"jc"`
}

// verifyASC re-reads the applied ASC and confirms "jc" matches wantJc,
// retrying briefly since NDMS may apply the change asynchronously.
func verifyASC(ctx context.Context, c *Client, name string, wantJc int) error {
	path := fmt.Sprintf("/show/rc/interface/%s/wireguard/asc", name)
	want := strconv.Itoa(wantJc)

	var lastErr error
	for attempt := 0; attempt < ascVerifyAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(ascVerifyDelay):
			}
		}

		raw, err := c.Get(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		var got ascReadback
		if err := json.Unmarshal(raw, &got); err != nil {
			lastErr = fmt.Errorf("decode readback: %w", err)
			continue
		}
		if got.Jc == want {
			return nil
		}
		lastErr = fmt.Errorf("jc not yet applied: got %q, want %q", got.Jc, want)
	}
	return fmt.Errorf("asc not confirmed after %d attempts: %w", ascVerifyAttempts, lastErr)
}

// AddPeer adds (or, if pubkey already exists on iface, updates) a WireGuard
// peer. psk may be empty to omit the preshared key. Each entry in allowIPs is
// parsed as a CIDR (see parseAllowedIP) and expanded into the address/mask
// pair RCI expects.
func AddPeer(ctx context.Context, c *Client, iface, pubkey, psk string, allowIPs []string) error {
	if pubkey == "" {
		return fmt.Errorf("keenetic: add peer on %s: empty public key", iface)
	}

	allowed := make([]map[string]any, 0, len(allowIPs))
	for _, cidr := range allowIPs {
		addr, mask, err := parseAllowedIP(cidr)
		if err != nil {
			return fmt.Errorf("keenetic: add peer on %s: %w", iface, err)
		}
		allowed = append(allowed, map[string]any{"address": addr, "mask": mask})
	}

	peer := map[string]any{
		"key":     pubkey,
		"connect": true,
	}
	if psk != "" {
		peer["preshared-key"] = psk
	}
	if len(allowed) > 0 {
		peer["allow-ips"] = allowed
	}

	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			iface: map[string]any{
				"wireguard": map[string]any{
					"peer": []any{peer},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: add peer on %s: %w", iface, err)
	}
	return nil
}

// RemovePeer deletes the peer identified by pubkey from iface.
func RemovePeer(ctx context.Context, c *Client, iface, pubkey string) error {
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			iface: map[string]any{
				"wireguard": map[string]any{
					"peer": []any{
						map[string]any{"key": pubkey, "no": true},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: remove peer on %s: %w", iface, err)
	}
	return nil
}

// parseAllowedIP parses a wg-quick-style AllowedIPs entry ("10.0.0.2/32",
// or a bare "10.0.0.2" defaulting to /32) into the (address, dotted-decimal
// mask) pair RCI's "allow-ips" expects. Both IPv4 and IPv6 CIDRs are
// accepted; IPv6 masks are rendered in the same dotted/hex-group form
// net.IPMask.String() produces for a 16-byte mask.
func parseAllowedIP(cidr string) (address, mask string, err error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return "", "", fmt.Errorf("empty allowed-ip entry")
	}

	if !strings.Contains(cidr, "/") {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return "", "", fmt.Errorf("invalid allowed-ip %q", cidr)
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), "255.255.255.255", nil
		}
		return ip.String(), net.CIDRMask(128, 128).String(), nil
	}

	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("invalid allowed-ip %q: %w", cidr, err)
	}
	maskStr := maskToString(ipNet.Mask)
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), maskStr, nil
	}
	return ip.String(), maskStr, nil
}

// maskToString renders a net.IPMask as dotted-decimal (IPv4) text. For an
// IPv6 mask it falls back to net.IPMask's own (hex) String representation,
// since RCI's wireguard peer schema is IPv4-oriented in practice but should
// not hard-fail on an IPv6 AllowedIPs entry.
func maskToString(m net.IPMask) string {
	if len(m) == 4 {
		return net.IP(m).String()
	}
	if v4 := net.IP(m).To4(); len(m) == 16 && v4 != nil {
		return v4.String()
	}
	return m.String()
}
