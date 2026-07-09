package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// This file drives Keenetic's native domain-routing stack — the mechanism
// behind the router's "Маршруты/DNS" section on KeeneticOS 5.x:
//
//   - an `object-group fqdn` holds a set of domain names the router resolves
//     and tracks in-kernel;
//   - a `dns-proxy route` binds that group to an interface, so every address a
//     member domain resolves to is routed through the chosen tunnel.
//
// This is the idiomatic Keenetic-native path (no ipset, no /etc/hosts, no
// iptables), and it is exactly how mature managers like awg-manager implement
// per-service routing. The RCI payload shapes below mirror what the router's
// own web UI emits and have been cross-checked against awg-manager's verified
// command layer.

// MaxDomainsPerGroup is NDMS's practical ceiling on entries in a single
// object-group. Larger domain sets are split across numbered groups
// ("{slug}-1", "{slug}-2", …) by ChunkDomains.
const MaxDomainsPerGroup = 300

// groupNameRe restricts object-group names to a conservative, NDMS-safe charset
// (letters, digits, dash, underscore). SanitizeGroupName enforces it.
var groupNameRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// SanitizeGroupName turns an arbitrary slug into an NDMS-safe object-group name.
// keen-manager prefixes its groups with "km-" so they are trivially
// distinguishable from user- or firmware-created groups and can never collide
// with them during reconciliation.
func SanitizeGroupName(slug string) string {
	s := groupNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(slug)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "route"
	}
	if !strings.HasPrefix(s, "km-") {
		s = "km-" + s
	}
	// NDMS caps group names; keep well under any limit.
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// GroupPrefix is the namespace every keen-manager object-group carries, so
// reconciliation and teardown never touch groups the user or firmware own.
const GroupPrefix = "km-"

// ChunkDomains splits domains into <=MaxDomainsPerGroup slices, one per
// object-group. baseName is sanitized; the first chunk keeps baseName and
// subsequent chunks get a "-N" suffix. Returns a name->domains map preserving
// input order within each chunk.
func ChunkDomains(baseName string, domains []string) map[string][]string {
	base := SanitizeGroupName(baseName)
	out := map[string][]string{}
	if len(domains) == 0 {
		return out
	}
	chunk := 0
	for i := 0; i < len(domains); i += MaxDomainsPerGroup {
		end := i + MaxDomainsPerGroup
		if end > len(domains) {
			end = len(domains)
		}
		name := base
		if chunk > 0 {
			name = fmt.Sprintf("%s-%d", base, chunk)
		}
		out[name] = append([]string(nil), domains[i:end]...)
		chunk++
	}
	return out
}

// SetObjectGroupFQDN creates or replaces an FQDN object-group named group with
// exactly the given domains. NDMS treats the "include" array as the desired set
// for a create, so this is effectively idempotent for our use (we always send
// the full membership for a group).
//
//	{"object-group":{"fqdn":{"km-youtube":{"include":[{"address":"youtube.com"},…]}}}}
func SetObjectGroupFQDN(ctx context.Context, c *Client, group string, domains []string) error {
	include := make([]map[string]any, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		include = append(include, map[string]any{"address": d})
	}
	if len(include) == 0 {
		return fmt.Errorf("keenetic: object-group %s: no valid domains", group)
	}
	_, err := c.Post(ctx, map[string]any{
		"object-group": map[string]any{
			"fqdn": map[string]any{
				group: map[string]any{"include": include},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: set object-group %s: %w", group, err)
	}
	return nil
}

// DeleteObjectGroup removes an FQDN object-group entirely.
func DeleteObjectGroup(ctx context.Context, c *Client, group string) error {
	_, err := c.Post(ctx, map[string]any{
		"object-group": map[string]any{
			"fqdn": map[string]any{
				group: map[string]any{"no": true},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: delete object-group %s: %w", group, err)
	}
	return nil
}

// AddDNSRoute binds an object-group to an interface so member domains route
// through it. auto=true lets NDMS keep the resolved addresses fresh as DNS TTLs
// expire (the normal case).
//
//	{"dns-proxy":{"route":[{"group":"km-youtube","interface":"Wireguard0","auto":true}]}}
func AddDNSRoute(ctx context.Context, c *Client, group, iface string, auto bool) error {
	if iface == "" {
		return fmt.Errorf("keenetic: dns route for %s: empty interface", group)
	}
	_, err := c.Post(ctx, map[string]any{
		"dns-proxy": map[string]any{
			"route": []any{
				map[string]any{"group": group, "interface": iface, "auto": auto},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: add dns route %s->%s: %w", group, iface, err)
	}
	return nil
}

// DeleteDNSRoute removes the dns-proxy route binding an object-group to an
// interface (the object-group itself is removed separately by DeleteObjectGroup).
func DeleteDNSRoute(ctx context.Context, c *Client, group, iface string) error {
	_, err := c.Post(ctx, map[string]any{
		"dns-proxy": map[string]any{
			"route": []any{
				map[string]any{"group": group, "interface": iface, "no": true},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: delete dns route %s->%s: %w", group, iface, err)
	}
	return nil
}

// dnsRouteEntry is one row of "GET /show/rc/dns-proxy" route configuration.
type dnsRouteEntry struct {
	Group     string `json:"group"`
	Interface string `json:"interface"`
}

// ListDNSRoutes reads the configured dns-proxy routes (best-effort; returns an
// empty slice if the firmware exposes the read at a different path). Used by
// reconciliation to compute a diff and to only ever remove routes keen-manager
// created (group names carrying GroupPrefix).
func ListDNSRoutes(ctx context.Context, c *Client) ([]dnsRouteEntry, error) {
	raw, err := c.Get(ctx, "/show/rc/dns-proxy")
	if err != nil {
		return nil, fmt.Errorf("keenetic: list dns routes: %w", err)
	}
	// The read shape varies across firmware; decode defensively.
	var shaped struct {
		Route []dnsRouteEntry `json:"route"`
	}
	if err := json.Unmarshal(raw, &shaped); err == nil && len(shaped.Route) > 0 {
		return shaped.Route, nil
	}
	return nil, nil
}

// OwnedDNSRoutes filters a route list to those keen-manager owns (group name
// prefixed with GroupPrefix).
func OwnedDNSRoutes(routes []dnsRouteEntry) []dnsRouteEntry {
	var out []dnsRouteEntry
	for _, r := range routes {
		if strings.HasPrefix(r.Group, GroupPrefix) {
			out = append(out, r)
		}
	}
	return out
}
