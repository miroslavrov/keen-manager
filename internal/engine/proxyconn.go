package engine

import (
	"context"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// This file implements the "Xray as a single KeeneticOS Proxy connection"
// model (docs/XRAY-PROXY-PLAN.md): keen-manager runs its local Xray with a
// SOCKS inbound on 127.0.0.1:10808 and registers ONE managed "ProxyN"
// interface pointing at it. Every Xray connection shares that single exit
// point; switching server/"select best" only rewrites the Xray config under
// the hood, so the router keeps showing one stable Proxy connection. Routes
// bind to ProxyN via the same dns-proxy stack as native AWG.
//
// TPROXY stays as the fallback for firmware without the Proxy client component
// (or when the user forces it), so nothing here can strand an Xray user: a
// rejected Proxy-interface write degrades to the existing transparent-proxy
// path with a logged hint.

const (
	xrayModeProxy  = "proxy"  // one visible KeeneticOS Proxy connection → local SOCKS
	xrayModeTProxy = "tproxy" // legacy transparent-proxy capture (invisible interface)

	// proxyIfaceDescription labels the managed interface in the Keenetic UI.
	proxyIfaceDescription = "keen-manager (Xray)"
)

// xrayMode resolves how an Xray connection is wired to the router, honouring
// the user's Settings.XrayIntegration override and otherwise auto-detecting:
// proxy-connection when the Proxy client component looks present and the native
// DNS-routing stack is available, else TPROXY. Once a Proxy-interface create
// has been rejected this session (proxyClientDown), it sticks to TPROXY.
func (e *Engine) xrayMode() string {
	switch strings.ToLower(strings.TrimSpace(e.store.Settings().XrayIntegration)) {
	case xrayModeTProxy:
		return xrayModeTProxy
	case xrayModeProxy:
		if e.isProxyClientDown() {
			return xrayModeTProxy
		}
		return xrayModeProxy
	}
	// auto
	if e.isProxyClientDown() {
		return xrayModeTProxy
	}
	if e.keenetic != nil && !e.runner.DryRun && e.caps.HasProxyClient && e.caps.SupportsDNSRoute {
		return xrayModeProxy
	}
	return xrayModeTProxy
}

// xrayProxyMode reports whether the resolved Xray wiring is the single-Proxy
// -connection model (vs. TPROXY). Convenience wrapper used across the engine.
func (e *Engine) xrayProxyMode() bool { return e.xrayMode() == xrayModeProxy }

func (e *Engine) isProxyClientDown() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.proxyClientDown
}

func (e *Engine) setProxyClientDown(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.proxyClientDown = v
}

// managedProxyIface returns the single Proxy interface keen-manager registered
// for the Xray exit point, or "" if none exists yet.
func (e *Engine) managedProxyIface() string {
	return strings.TrimSpace(e.store.Get().ManagedProxyIface)
}

func (e *Engine) recordManagedProxyIface(name string) error {
	return e.store.Mutate(func(s *model.State) error {
		s.ManagedProxyIface = name
		return nil
	})
}

func (e *Engine) clearManagedProxyIface() {
	_ = e.store.Mutate(func(s *model.State) error {
		s.ManagedProxyIface = ""
		return nil
	})
}

// ensureManagedProxyIface makes sure the single ProxyN exit point exists,
// creating it once (pointed at the local Xray SOCKS inbound) and recording its
// name. It is reused for the life of the install — server switches never touch
// it. Off-device / dry-run it is a no-op (there is no router to create it on).
// A rejected create returns an error so bringUp can fall back to TPROXY.
func (e *Engine) ensureManagedProxyIface() (string, error) {
	if e.keenetic == nil || e.runner.DryRun {
		return "", nil
	}
	if name := e.managedProxyIface(); name != "" {
		return name, nil
	}

	ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
	defer cancel()

	idx, err := keenetic.FindFreeProxyIndex(ctx, e.keenetic)
	if err != nil {
		return "", err
	}
	name := keenetic.ProxyInterfaceName(idx)
	cfg := keenetic.ProxyConfig{
		Upstream:      xraySocksHost,
		Port:          xraySocksPort,
		Protocol:      "socks5",
		SecurityLevel: "public",
		Description:   proxyIfaceDescription,
		Up:            true,
	}
	if err := keenetic.CreateProxyInterface(ctx, e.keenetic, name, cfg); err != nil {
		return "", err
	}
	// IMPORTANT — do NOT mark ProxyN as a global/default internet connection
	// ("ip global" / the "use for internet access" checkbox). A SOCKS-proxy
	// interface is a per-domain routing TARGET, not a default route.
	//
	// Unlike a WireGuard kernel tunnel — whose server endpoint stays reachable
	// over the WAN because NDMS pins an endpoint host-route — a proxy interface
	// has no endpoint pinning. If ProxyN became the default, the router's OWN
	// egress (UDP DNS resolution + Xray's TCP connection out to the vless server)
	// would be sent into ProxyN → the local SOCKS 127.0.0.1:10808 → Xray → and,
	// having no other default, straight back into ProxyN: a tight routing loop.
	// SOCKS is also TCP-only, so UDP DNS through it dies outright. That is the
	// "it storms / no site loads / without a policy it swallows all traffic"
	// failure reported on-device. Traffic must reach the tunnel ONLY through
	// explicit dns-proxy routes bound to ProxyN (the Routes page), exactly like
	// AWG's per-service routes; the WAN stays the default so DNS and Xray's own
	// upstream egress normally. See docs/XRAY-PROXY-PLAN.md §6 (routing loop).
	if err := e.recordManagedProxyIface(name); err != nil {
		e.Logf("proxy-conn: warning: could not persist managed proxy iface %s: %v", name, err)
	}
	if err := e.keenetic.Save(ctx); err != nil {
		e.Logf("proxy-conn: warning: RCI save failed: %v", err)
	}
	e.Logf("proxy-conn: registered %s → SOCKS %s:%d as a per-service routing target (route domains to it on the Routes page; it is deliberately NOT the default connection — that would loop the router's own DNS/Xray-upstream; firmware %s)", name, xraySocksHost, xraySocksPort, e.caps.Release)
	return name, nil
}

// teardownManagedProxyIfaceIfUnused removes the shared Proxy interface once no
// Xray connection remains (e.g. after the last Xray connection is deleted), so
// keen-manager cleans up after itself. It keeps the interface while any Xray
// connection still exists — the exit point is shared, not per-connection.
func (e *Engine) teardownManagedProxyIfaceIfUnused() {
	name := e.managedProxyIface()
	if name == "" {
		return
	}
	for _, c := range e.store.Get().Connections {
		if c.Type == model.ConnXray {
			return // still in use by at least one Xray connection
		}
	}
	if e.keenetic != nil && !e.runner.DryRun {
		ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
		defer cancel()
		if err := keenetic.DeleteInterface(ctx, e.keenetic, name); err != nil {
			e.Logf("proxy-conn: could not remove %s: %v", name, err)
		} else {
			_ = e.keenetic.Save(ctx)
			e.Logf("proxy-conn: removed %s (no Xray connections remain)", name)
		}
	}
	e.clearManagedProxyIface()
}

// bringUpXrayProxy brings up an Xray connection in proxy-connection mode:
// apply the SOCKS-only config for the active server, then ensure the single
// ProxyN exit point exists. If the Proxy-interface write is rejected (component
// absent / wrong shape), it latches proxyClientDown and returns errProxyFallback
// so bringUp can retry via TPROXY. cfgFor builds the mode-appropriate config.
func (e *Engine) bringUpXrayProxy(connID string, srv model.Server) error {
	cfg, err := e.buildActiveXray(connID, srv, "")
	if err != nil {
		return err
	}
	if _, err := e.xray.Apply(cfg); err != nil {
		return err
	}
	if _, err := e.ensureManagedProxyIface(); err != nil {
		e.Logf("proxy-conn: Proxy client unavailable (%v) — falling back to TPROXY for Xray. Install the Proxy client component (General settings → Component options) for a single visible connection, and read back the real RCI shape (see docs/XRAY-PROXY-PLAN.md §3).", err)
		e.setProxyClientDown(true)
		// Rebuild + apply the TPROXY-mode config; applyRouting will now enable
		// the transparent-proxy capture because xrayMode() has flipped.
		tproxyCfg, berr := e.buildActiveXray(connID, srv, "")
		if berr != nil {
			return berr
		}
		if _, aerr := e.xray.Apply(tproxyCfg); aerr != nil {
			return aerr
		}
	}
	return nil
}
