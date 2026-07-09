package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// InterfaceInfo is the parsed summary of one router interface as reported by
// "GET /show/interface/". It carries the facts the UI needs to present a
// pick-list of real router interfaces (the "pull interfaces from KeeneticOS"
// feature): the NDMS id, the human label, the transport type, a normalised
// up/connected state, and the assigned address.
type InterfaceInfo struct {
	// Name is the NDMS interface id, e.g. "Wireguard0", "Bridge0",
	// "GigabitEthernet1". This is the value dns-proxy routes and static routes
	// bind to.
	Name string
	// SysName is the kernel-facing "interface-name" (e.g. "nwg0"). Best-effort:
	// for WireGuard tunnels NDMS often echoes the NDMS id here rather than the
	// real kernel device, so it is informational only.
	SysName string
	// Type is the NDMS transport type string, e.g. "Wireguard", "Bridge",
	// "GigabitEthernet", "PPP".
	Type string
	// Description is the user-facing label set in the Keenetic UI (may be empty).
	Description string
	// Up is the normalised administrative/link state (NDMS reports it via a mix
	// of state/link words that include "running"/"pending"/"disabled").
	Up bool
	// Connected reports the NDMS "connected" flag ("yes"/"no").
	Connected bool
	// Address is the interface's primary IPv4 address (may be empty).
	Address string
	// SecurityLevel is "public" (WAN-facing) / "private" (LAN) when reported.
	SecurityLevel string
	// Priority is the connection-priority NDMS assigns for default-route
	// selection (0 when not reported).
	Priority int
	// IsWireguard is true for native WireGuard/AmneziaWG interfaces — the only
	// interfaces that can back a dns-proxy route (a Routes target).
	IsWireguard bool
}

// ifaceWire mirrors the per-entry object of "GET /show/interface/". The field
// names are the literal NDMS wire keys (kebab-case), verified against real
// KeeneticOS captures. Absent fields decode to their zero value, so a sparse
// entry (e.g. a Bridge with no mtu/uptime) is not an error.
type ifaceWire struct {
	ID            string `json:"id"`
	InterfaceName string `json:"interface-name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	State         string `json:"state"`
	Link          string `json:"link"`
	Connected     string `json:"connected"`
	SecurityLevel string `json:"security-level"`
	Address       string `json:"address"`
	Mask          string `json:"mask"`
	Priority      int    `json:"priority"`
}

// builtInVPNServerDescription is the description KeeneticOS gives its own
// bundled WireGuard VPN *server* interface. It is filtered from routable
// targets since routing client traffic into the router's own server is never
// what the user means.
const builtInVPNServerDescription = "Wireguard VPN Server"

// ListInterfaces reads "GET /show/interface/" and returns every router
// interface as an InterfaceInfo, sorted by name. The RCI response is a JSON
// object keyed by interface id; each value is decoded defensively so an
// unfamiliar or sparse entry is skipped/zero-filled rather than failing the
// whole listing.
func ListInterfaces(ctx context.Context, c *Client) ([]InterfaceInfo, error) {
	raw, err := c.Get(ctx, "/show/interface/")
	if err != nil {
		return nil, fmt.Errorf("keenetic: list interfaces: %w", err)
	}

	var listing map[string]json.RawMessage
	if err := json.Unmarshal(raw, &listing); err != nil {
		return nil, fmt.Errorf("keenetic: decode /show/interface/: %w", err)
	}

	out := make([]InterfaceInfo, 0, len(listing))
	for id, rm := range listing {
		var w ifaceWire
		if err := json.Unmarshal(rm, &w); err != nil {
			// A single malformed entry must not sink the whole listing.
			continue
		}
		name := id
		if strings.TrimSpace(w.ID) != "" {
			name = w.ID
		}
		out = append(out, InterfaceInfo{
			Name:          name,
			SysName:       w.InterfaceName,
			Type:          w.Type,
			Description:   w.Description,
			Up:            ifaceStateUp(w.State) || ifaceStateUp(w.Link),
			Connected:     ifaceStateUp(w.Connected),
			Address:       w.Address,
			SecurityLevel: w.SecurityLevel,
			Priority:      w.Priority,
			IsWireguard:   strings.EqualFold(strings.TrimSpace(w.Type), "Wireguard"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// IsBuiltInVPNServer reports whether an interface is the router's own bundled
// WireGuard VPN server (matched by its NDMS description), which is not a valid
// routing target for outbound client traffic.
func (i InterfaceInfo) IsBuiltInVPNServer() bool {
	return strings.EqualFold(strings.TrimSpace(i.Description), builtInVPNServerDescription)
}

// ifaceStateUp normalises the several "is it up?" words NDMS uses across its
// state/link/connected fields ("up", "running", "connected", "yes") to a bool.
// "pending"/"disabled"/"down"/"" all read as not-up (per NDMS's layer-state
// vocabulary, where only "running" counts as up).
func ifaceStateUp(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "up", "running", "connected", "yes":
		return true
	default:
		return false
	}
}
