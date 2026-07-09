package engine

import (
	"context"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// Interfaces returns the router's interfaces as reported live by KeeneticOS
// over RCI (GET /show/interface/), annotated with keen-manager's own view:
// which interface a managed connection created, and whether it can back a
// Routes dns-proxy route. This is the data behind the "pick a router
// interface" dropdown — the interfaces are pulled from the device, not
// synthesised from keen-manager's connection list.
//
// Off-device / dry-run (or when the RCI endpoint is unreachable) it returns an
// empty list with an explanatory Note rather than an error, so the UI degrades
// to a clear empty state instead of a red error.
func (e *Engine) Interfaces() InterfacesView {
	dnsAvail := e.dnsRoutingAvailable()
	if e.keenetic == nil || e.runner.DryRun {
		return InterfacesView{
			Interfaces:          []InterfaceView{},
			DNSRoutingAvailable: dnsAvail,
			Note:                "router interfaces are read live from KeeneticOS on-device; none are available in this mode",
		}
	}

	ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
	defer cancel()
	infos, err := keenetic.ListInterfaces(ctx, e.keenetic)
	if err != nil {
		return InterfacesView{
			Interfaces:          []InterfaceView{},
			DNSRoutingAvailable: dnsAvail,
			Note:                "could not read interfaces from the router: " + err.Error(),
		}
	}

	// Reverse-map the native interface name -> keen-manager connection id so a
	// router interface can be tied back to the connection that created it.
	st := e.store.Get()
	byIface := map[string]string{}
	for cid, name := range st.NativeIfaces {
		if name != "" {
			byIface[name] = cid
		}
	}
	// The shared managed Proxy interface (Xray exit point) maps back to the
	// active Xray connection when one is active, else the first Xray connection.
	managedProxy := e.managedProxyIface()
	proxyConnID := ""
	if managedProxy != "" {
		if c, ok := findConn(st, st.ActiveConnID); ok && c.Type == model.ConnXray {
			proxyConnID = c.ID
		} else {
			for _, c := range st.Connections {
				if c.Type == model.ConnXray {
					proxyConnID = c.ID
					break
				}
			}
		}
	}

	out := make([]InterfaceView, 0, len(infos))
	for _, in := range infos {
		// Both native WireGuard and KeeneticOS Proxy interfaces can back a
		// dns-proxy route (a Routes target); the router's own VPN server cannot.
		routable := (in.IsWireguard || in.IsProxy) && !in.IsBuiltInVPNServer()
		managedID := byIface[in.Name]
		if managedID == "" && managedProxy != "" && in.Name == managedProxy {
			managedID = proxyConnID
		}
		out = append(out, InterfaceView{
			Name:          in.Name,
			Label:         firstNonEmpty(strings.TrimSpace(in.Description), in.Name),
			Description:   in.Description,
			Type:          in.Type,
			Up:            in.Up,
			Connected:     in.Connected,
			Address:       in.Address,
			Security:      in.SecurityLevel,
			IsWireguard:   in.IsWireguard,
			IsProxy:       in.IsProxy,
			Routable:      routable,
			ManagedConnID: managedID,
		})
	}
	return InterfacesView{Interfaces: out, DNSRoutingAvailable: dnsAvail}
}
