package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/tpws"
)

// This file implements the DPI-bypass "routable interface" feature the user
// asked for: nfqws exposed "like Xray" — an IP:port that becomes a routable
// KeeneticOS interface, NOT a global inline NFQUEUE. It deliberately mirrors the
// Xray proxy-connection plumbing in proxyconn.go 1:1:
//
//	tpws (zapret socket desync proxy, SOCKS mode) on 127.0.0.1:<port>
//	   └── one managed KeeneticOS Proxy interface (ProxyN) → that SOCKS port
//	          └── Routes bind selected domains to ProxyN via dns-proxy (like AWG)
//
// Because nfqws (NFQUEUE) offers no socket endpoint, tpws — its socket-level
// sibling — provides the listener a Proxy interface can point at. The chosen
// domains are the SAME source as Routes (one source of truth): a route targets
// the reserved "bypass" target (bypassTargetID) and is otherwise an ordinary
// dns-proxy route. Strategy (how tpws desyncs) lives on the Bypass page; the
// Routes page only chooses which domains go through it.
//
// SAME ANTI-LOOP RULE AS beta.9: the bypass ProxyN is a per-service routing
// TARGET only and is never marked "use for internet access" (ip global). A
// SOCKS-proxy interface has no endpoint pinning, so making it the default would
// loop the router's own egress through it — see the long note in
// ensureManagedProxyIface and docs/XRAY-PROXY-PLAN.md §6.
//
// Everything device-mutating is dry-run aware (inert off-device); an absent
// tpws binary or Proxy client component degrades to a logged hint and never
// bricks the router.

const (
	// bypassSocksHost is tpws' loopback bind address (see tpws.DefaultBindAddr).
	bypassSocksHost = "127.0.0.1"
	// bypassIfaceDescription labels the managed interface in the Keenetic UI.
	bypassIfaceDescription = "keen-manager (DPI bypass)"
	// bypassTargetID is the reserved Routes target id that binds a route to the
	// managed DPI-bypass Proxy interface. It is NOT a real connection id — the
	// route code special-cases it (validation, resolution, labelling).
	bypassTargetID = "bypass"

	// subTargetPrefix is the reserved Routes target id prefix that binds a
	// route to ALL servers in a subscription. The target_conn_id is encoded
	// as "sub:<subscription_id>". Like bypassTargetID, it is NOT a real
	// connection id — the route code resolves it to the currently-active
	// member of that subscription (re-evaluated on every activate/failover).
	subTargetPrefix = "sub:"
)

// bypassSeedPresets are the default services seeded as bypass routes on first
// enable — Discord + YouTube, taken from the built-in preset catalog
// (internal/presets), per the user's request. The user can edit or delete them.
var bypassSeedPresets = []string{"discord", "youtube"}

// bypassEnabled reports whether the routable DPI-bypass feature is on.
func (e *Engine) bypassEnabled() bool { return e.store.Get().Bypass.Enabled }

// bypassPort returns the configured tpws SOCKS port, or the tpws default.
func (e *Engine) bypassPort() int {
	if p := e.store.Get().Bypass.Port; p > 0 {
		return p
	}
	return tpws.DefaultPort
}

// bypassStrategy returns the configured tpws desync strategy, or the default.
func (e *Engine) bypassStrategy() string {
	if s := strings.TrimSpace(e.store.Get().Bypass.Strategy); s != "" {
		return s
	}
	return tpws.DefaultStrategy
}

// bypassOpts assembles the tpws runtime options from persisted state.
func (e *Engine) bypassOpts() tpws.Options {
	return tpws.Options{Port: e.bypassPort(), Strategy: e.bypassStrategy()}
}

func (e *Engine) managedBypassIface() string {
	return strings.TrimSpace(e.store.Get().ManagedBypassIface)
}

func (e *Engine) recordManagedBypassIface(name string) error {
	return e.store.Mutate(func(s *model.State) error {
		s.ManagedBypassIface = name
		return nil
	})
}

func (e *Engine) clearManagedBypassIface() {
	_ = e.store.Mutate(func(s *model.State) error {
		s.ManagedBypassIface = ""
		return nil
	})
}

func (e *Engine) isBypassClientDown() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bypassClientDown
}

func (e *Engine) setBypassClientDown(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bypassClientDown = v
}

// ensureManagedBypassIface makes sure the single ProxyN pointing at the local
// tpws SOCKS listener exists, creating it once and recording its name. Reused
// for the life of the feature. Off-device / dry-run it is a no-op. A rejected
// create returns an error so bringUpBypass can surface a hint and latch.
//
// It is the exact analogue of ensureManagedProxyIface (Xray), including the
// critical anti-loop invariant: the interface is NEVER marked global.
func (e *Engine) ensureManagedBypassIface() (string, error) {
	if e.keenetic == nil || e.runner.DryRun {
		return "", nil
	}
	if name := e.managedBypassIface(); name != "" {
		return name, nil
	}
	if !e.caps.HasProxyClient {
		return "", fmt.Errorf("KeeneticOS Proxy client component not detected")
	}

	ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
	defer cancel()

	idx, err := keenetic.FindFreeProxyIndex(ctx, e.keenetic)
	if err != nil {
		return "", err
	}
	name := keenetic.ProxyInterfaceName(idx)
	cfg := keenetic.ProxyConfig{
		Upstream:      bypassSocksHost,
		Port:          e.bypassPort(),
		Protocol:      "socks5",
		SecurityLevel: "public",
		Description:   bypassIfaceDescription,
		Up:            true,
	}
	if err := keenetic.CreateProxyInterface(ctx, e.keenetic, name, cfg); err != nil {
		return "", err
	}
	// IMPORTANT — like the Xray ProxyN, the bypass ProxyN must NOT be marked a
	// global/default internet connection ("ip global"). It is a per-domain
	// routing TARGET only, reached via explicit dns-proxy routes from the Routes
	// page. A SOCKS-proxy interface has no endpoint pinning, so making it the
	// default would send the router's own egress into tpws and loop it back —
	// the beta.9 routing-loop lesson applies identically here. See
	// docs/XRAY-PROXY-PLAN.md §6 and ensureManagedProxyIface.
	if err := e.recordManagedBypassIface(name); err != nil {
		e.Logf("bypass: warning: could not persist managed bypass iface %s: %v", name, err)
	}
	if err := e.keenetic.Save(ctx); err != nil {
		e.Logf("bypass: warning: RCI save failed: %v", err)
	}
	e.Logf("bypass: registered %s → tpws SOCKS %s:%d as a per-service routing target (route domains to it on the Routes page; it is deliberately NOT the default connection — that would loop the router's own egress; firmware %s)", name, bypassSocksHost, e.bypassPort(), e.caps.Release)
	return name, nil
}

// teardownManagedBypassIface removes the managed bypass Proxy interface (best
// -effort) and clears its recorded name. Used when the feature is turned off.
func (e *Engine) teardownManagedBypassIface() {
	name := e.managedBypassIface()
	if name == "" {
		return
	}
	if e.keenetic != nil && !e.runner.DryRun {
		ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
		defer cancel()
		if err := keenetic.DeleteInterface(ctx, e.keenetic, name); err != nil {
			e.Logf("bypass: could not remove %s: %v", name, err)
		} else {
			_ = e.keenetic.Save(ctx)
			e.Logf("bypass: removed %s", name)
		}
	}
	e.clearManagedBypassIface()
}

// seedDefaultBypassRoutes creates the default Discord + YouTube bypass routes
// once (guarded by Bypass.Seeded), sourcing their domains from the preset
// catalog so nothing is invented. The user can freely edit or delete them.
func (e *Engine) seedDefaultBypassRoutes() {
	if e.store.Get().Bypass.Seeded {
		return
	}
	for _, id := range bypassSeedPresets {
		if _, err := e.CreateRoute("", id, nil, nil, bypassTargetID, ""); err != nil {
			e.Logf("bypass: seed default route %q: %v", id, err)
		}
	}
	_ = e.store.Mutate(func(s *model.State) error {
		s.Bypass.Seeded = true
		return nil
	})
	e.Logf("bypass: seeded default routes (%s) — edit or delete them on the Routes page", strings.Join(bypassSeedPresets, ", "))
}

// bringUpBypass enables the routable DPI-bypass exit point: ensure tpws is
// running with the current strategy, register the single managed ProxyN, seed
// the default routes once, and (re)apply any enabled bypass routes. On a
// missing tpws binary or Proxy client component it latches bypassClientDown and
// returns an error carrying an actionable hint; the router is left untouched.
func (e *Engine) bringUpBypass() error {
	if !e.runner.DryRun && !e.tpws.Installed() {
		e.setBypassClientDown(true)
		return fmt.Errorf("tpws is not installed — the routable DPI-bypass interface needs tpws (zapret's socket desync proxy); install it (opkg install tpws) and try again")
	}
	// Start / reconfigure tpws with the current port + strategy (rewrites the
	// generated init script and restarts). Inert in dry-run.
	if err := e.tpws.Apply(e.bypassOpts()); err != nil {
		return fmt.Errorf("start tpws: %w", err)
	}
	// Register the single managed Proxy interface → tpws (never global).
	if _, err := e.ensureManagedBypassIface(); err != nil {
		e.Logf("bypass: cannot expose tpws as a routable interface (%v) — install the KeeneticOS Proxy client component (General settings → Component options) and read back the RCI shape (docs/XRAY-PROXY-PLAN.md §3). tpws itself is running; only the routable interface is unavailable.", err)
		e.setBypassClientDown(true)
		return err
	}
	e.setBypassClientDown(false)
	// Seed the default Discord + YouTube routes once, then apply enabled routes
	// now that the interface exists.
	e.seedDefaultBypassRoutes()
	e.reconcileRoutes()
	return nil
}

// bringDownBypass disables the feature: stop tpws (and drop its init script so
// it doesn't restart on reboot), unapply routes bound to the bypass interface
// (keeping them in state for a later re-enable), and retire the managed ProxyN.
func (e *Engine) bringDownBypass() error {
	if err := e.tpws.Stop(); err != nil {
		e.Logf("bypass: stop tpws: %v", err)
	}
	_ = e.tpws.RemoveInitScript()
	for _, r := range e.store.Get().Routes {
		if r.TargetConnID == bypassTargetID {
			_ = e.unapplyRoute(r)
		}
	}
	e.teardownManagedBypassIface()
	e.Logf("bypass: routable DPI-bypass interface disabled")
	e.publishState()
	return nil
}

// reconcileBypass re-establishes the bypass exit point after a daemon restart /
// router reboot when the feature was on. Runs off the hot path (see Start).
func (e *Engine) reconcileBypass() {
	if !e.bypassEnabled() {
		return
	}
	if err := e.bringUpBypass(); err != nil {
		e.Logf("bypass: reconcile: %v", err)
	}
}

// SaveBypass updates the bypass feature settings (enabled / strategy / port)
// and brings the feature up or down accordingly. Toggling enabled clears the
// bypassClientDown latch so the user can retry after installing a missing
// component.
func (e *Engine) SaveBypass(fields map[string]any) error {
	// Pre-flight: turning the feature ON needs tpws present on-device. Reject
	// cleanly (without persisting enabled=true) so the toggle can't latch on
	// against a missing binary — the user installs tpws first, then enables.
	if v, ok := fields["enabled"].(bool); ok && v && !e.runner.DryRun && !e.tpws.Installed() {
		return fmt.Errorf("tpws is not installed — install it first (opkg install tpws), then enable the routable DPI-bypass interface")
	}
	var enabledChanged bool
	err := e.store.Mutate(func(s *model.State) error {
		if v, ok := fields["enabled"].(bool); ok {
			if s.Bypass.Enabled != v {
				enabledChanged = true
			}
			s.Bypass.Enabled = v
		}
		if v, ok := fields["strategy"].(string); ok {
			s.Bypass.Strategy = strings.TrimSpace(v)
		}
		if v, ok := fields["port"]; ok {
			p, ok2 := intFromJSON(v)
			if !ok2 {
				return fmt.Errorf("bypass port must be a number")
			}
			if p != 0 && (p < 1 || p > 65535) {
				return fmt.Errorf("bypass port %d out of range (1-65535)", p)
			}
			s.Bypass.Port = p
		}
		return nil
	})
	if err != nil {
		return err
	}
	if enabledChanged {
		e.setBypassClientDown(false)
	}
	e.publishState()
	if e.bypassEnabled() {
		return e.bringUpBypass()
	}
	return e.bringDownBypass()
}

// Bypass returns the UI-facing status of the routable DPI-bypass feature.
func (e *Engine) Bypass() BypassView {
	st := e.store.Get()
	v := BypassView{
		Enabled:   st.Bypass.Enabled,
		Installed: e.tpws.Installed(),
		Port:      e.bypassPort(),
		Strategy:  e.bypassStrategy(),
		Interface: e.managedBypassIface(),
		Target:    bypassTargetID,
	}
	if !e.runner.DryRun {
		v.Running = e.tpws.Running()
	}
	// Routable once the managed Proxy interface exists (so Routes can bind to it).
	v.Routable = v.Interface != ""

	switch {
	case !v.Installed && !e.runner.DryRun:
		v.Note = "tpws is not installed. The routable DPI-bypass interface needs tpws (zapret's socket desync proxy); install it with: opkg install tpws"
	case st.Bypass.Enabled && e.isBypassClientDown():
		v.Note = "tpws is running but could not be exposed as a router interface — install the KeeneticOS Proxy client component (General settings → Component options)."
	case st.Bypass.Enabled && v.Interface == "" && !e.runner.DryRun:
		v.Note = "enabling — the managed Proxy interface has not been registered yet."
	case st.Bypass.Enabled:
		v.Note = "route domains through DPI bypass from the Routes page (target: DPI Bypass)."
	default:
		v.Note = "off. Enable to run tpws and expose it as one routable Proxy interface; then send chosen domains through it from the Routes page."
	}
	return v
}

// intFromJSON coerces a JSON-decoded number (float64) or an integer/string into
// an int. Returns false when the value is not a recognisable number.
func intFromJSON(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		return atoi(n), strings.TrimSpace(n) != ""
	default:
		return 0, false
	}
}
