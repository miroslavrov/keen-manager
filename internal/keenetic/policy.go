package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// This file drives Keenetic's connection-priority / access-policy layer and
// static routing table via RCI — the pieces that let keen-manager scope a
// tunnel to specific traffic and expose it the way the router's own UI does.

// Policy is a parsed Keenetic access policy ("Приоритеты подключений").
type Policy struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	// Interfaces are the interface names permitted by the policy, in priority
	// order (best-effort; populated from the read shape when available).
	Interfaces []string `json:"interfaces,omitempty"`
}

// ListPolicies reads the configured access policies (best-effort). Returns an
// empty slice when the firmware has none or exposes them at a different path.
func ListPolicies(ctx context.Context, c *Client) ([]Policy, error) {
	raw, err := c.Get(ctx, "/show/rc/ip/policy")
	if err != nil {
		return nil, fmt.Errorf("keenetic: list policies: %w", err)
	}
	// The policy read is a map keyed by policy name; the value carries the
	// permit list. Decode leniently.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil // unknown shape — treat as "no policies visible"
	}
	out := make([]Policy, 0, len(m))
	for name, body := range m {
		p := Policy{Name: name}
		var shaped struct {
			Description string `json:"description"`
			Permit      []struct {
				Interface string `json:"interface"`
			} `json:"permit"`
		}
		if err := json.Unmarshal(body, &shaped); err == nil {
			p.Description = shaped.Description
			for _, perm := range shaped.Permit {
				if perm.Interface != "" {
					p.Interfaces = append(p.Interfaces, perm.Interface)
				}
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// PolicyPermitInterface adds (or updates) an interface permit inside an access
// policy, at the given priority order. This is how an interface is bound to a
// connection-priority policy so the router uses it for that policy's members.
//
//	{"ip":{"policy":{"Policy0":{"permit":{"global":true,"interface":"Wireguard0","order":0}}}}}
func PolicyPermitInterface(ctx context.Context, c *Client, policy, iface string, order int) error {
	if policy == "" || iface == "" {
		return fmt.Errorf("keenetic: policy permit: empty policy or interface")
	}
	_, err := c.Post(ctx, map[string]any{
		"ip": map[string]any{
			"policy": map[string]any{
				policy: map[string]any{
					"permit": map[string]any{
						"global":    true,
						"interface": iface,
						"order":     order,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: policy %s permit %s: %w", policy, iface, err)
	}
	return nil
}

// AddStaticRoute installs a static route sending a network (CIDR) through an
// interface. It uses the CLI escape hatch (`ip route <net> <mask> <iface>
// auto`) because the structured RCI shape for routes varies across firmware,
// whereas the CLI form is stable. auto lets the route follow the interface up/
// down state.
func AddStaticRoute(ctx context.Context, c *Client, cidr, iface string) error {
	network, mask, err := cidrToNetMask(cidr)
	if err != nil {
		return fmt.Errorf("keenetic: add route %q: %w", cidr, err)
	}
	if iface == "" {
		return fmt.Errorf("keenetic: add route %s: empty interface", cidr)
	}
	_, err = c.Parse(ctx, fmt.Sprintf("ip route %s %s %s auto", network, mask, iface))
	if err != nil {
		return fmt.Errorf("keenetic: add route %s via %s: %w", cidr, iface, err)
	}
	return nil
}

// DeleteStaticRoute removes a static route previously added for a network/iface.
func DeleteStaticRoute(ctx context.Context, c *Client, cidr, iface string) error {
	network, mask, err := cidrToNetMask(cidr)
	if err != nil {
		return fmt.Errorf("keenetic: delete route %q: %w", cidr, err)
	}
	_, err = c.Parse(ctx, fmt.Sprintf("no ip route %s %s %s", network, mask, iface))
	if err != nil {
		return fmt.Errorf("keenetic: delete route %s via %s: %w", cidr, iface, err)
	}
	return nil
}

// cidrToNetMask converts "10.0.0.0/24" into ("10.0.0.0", "255.255.255.0"). A
// bare address is treated as a host route (/32 or /128). IPv6 masks are
// rendered in the same form net.IP produces.
func cidrToNetMask(cidr string) (network, mask string, err error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return "", "", fmt.Errorf("empty network")
	}
	if !strings.Contains(cidr, "/") {
		ip := net.ParseIP(cidr)
		if ip == nil {
			return "", "", fmt.Errorf("invalid address %q", cidr)
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), "255.255.255.255", nil
		}
		return ip.String(), net.IP(net.CIDRMask(128, 128)).String(), nil
	}
	ip, ipNet, perr := net.ParseCIDR(cidr)
	if perr != nil {
		return "", "", fmt.Errorf("invalid CIDR %q: %w", cidr, perr)
	}
	if v4 := ip.To4(); v4 != nil {
		return ipNet.IP.String(), net.IP(ipNet.Mask).String(), nil
	}
	return ipNet.IP.String(), net.IP(ipNet.Mask).String(), nil
}
