package engine

import (
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

func setXrayIntegration(t *testing.T, e *Engine, mode string) {
	t.Helper()
	if err := e.store.Mutate(func(s *model.State) error {
		s.Settings.XrayIntegration = mode
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func setManagedProxyIface(t *testing.T, e *Engine, name string) {
	t.Helper()
	if err := e.recordManagedProxyIface(name); err != nil {
		t.Fatal(err)
	}
}

// TestXrayModeResolution covers Settings.XrayIntegration resolution and the
// proxyClientDown latch.
func TestXrayModeResolution(t *testing.T) {
	e := newTestEngine(t)

	// Default (dry-run, no caps, no setting) → auto resolves to tproxy.
	if got := e.xrayMode(); got != xrayModeTProxy {
		t.Errorf("default mode = %q, want tproxy", got)
	}

	setXrayIntegration(t, e, "proxy")
	if got := e.xrayMode(); got != xrayModeProxy {
		t.Errorf("explicit proxy mode = %q, want proxy", got)
	}
	if !e.xrayProxyMode() {
		t.Error("xrayProxyMode() should be true when mode is proxy")
	}

	setXrayIntegration(t, e, "tproxy")
	if got := e.xrayMode(); got != xrayModeTProxy {
		t.Errorf("explicit tproxy mode = %q, want tproxy", got)
	}

	// auto + on-device caps present → proxy (no network I/O in xrayMode()).
	setXrayIntegration(t, e, "")
	e.runner.DryRun = false
	e.caps.HasProxyClient = true
	e.caps.SupportsDNSRoute = true
	if got := e.xrayMode(); got != xrayModeProxy {
		t.Errorf("auto mode with Proxy client + DNS route = %q, want proxy", got)
	}

	// Once the Proxy interface create has been rejected, even an explicit proxy
	// setting falls back to tproxy for the session.
	e.setProxyClientDown(true)
	setXrayIntegration(t, e, "proxy")
	if got := e.xrayMode(); got != xrayModeTProxy {
		t.Errorf("proxyClientDown should force tproxy, got %q", got)
	}
	e.runner.DryRun = true // restore for any later use
}

// TestBuildActiveXrayProxyModeNoSplit confirms that in proxy-connection mode
// the active config is the SOCKS-only profile even when a route targets the
// connection: routing is the router's job (dns-proxy → ProxyN), so there is no
// in-Xray direct catch-all and no tproxy inbound.
func TestBuildActiveXrayProxyModeNoSplit(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")
	setXrayIntegration(t, e, "proxy")

	if _, err := e.CreateRoute("YouTube", "", []string{"youtube.com"}, nil, "conn-x", ""); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	srv, ok := e.vault.get("conn-x")
	if !ok {
		t.Fatal("server missing from vault")
	}
	cfg, err := e.buildActiveXray("conn-x", srv, "")
	if err != nil {
		t.Fatal(err)
	}
	if hasDirectCatchAll(cfg) {
		t.Error("proxy-connection mode must not compile an in-Xray split (router routes to ProxyN)")
	}
	for _, in := range cfg.Inbounds {
		if in.Protocol == "dokodemo-door" {
			t.Error("proxy-connection mode must not add a tproxy inbound")
		}
	}
	if len(cfg.Inbounds) != 1 || cfg.Inbounds[0].Protocol != "socks" {
		t.Errorf("expected a single socks inbound, got %+v", cfg.Inbounds)
	}
}

// TestResolveRouteIfaceProxyMode confirms an Xray-targeted route resolves to the
// shared managed Proxy interface in proxy mode (and is unresolved until one
// exists), and that in TPROXY mode it uses the in-Xray split path instead.
func TestResolveRouteIfaceProxyMode(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")
	setXrayIntegration(t, e, "proxy")

	r := model.ServiceRoute{ID: "r1", TargetConnID: "conn-x", Enabled: true, Domains: []string{"youtube.com"}}

	// No ProxyN yet → unresolved.
	if iface, ok := e.resolveRouteIface(r); ok {
		t.Errorf("expected no interface before ProxyN exists, got %q", iface)
	}
	// In proxy mode an Xray route is NOT an in-Xray split route.
	if _, ok := e.routeUsesXraySplit(e.store.Get(), r); ok {
		t.Error("proxy mode: an Xray route must not use the in-Xray split path")
	}

	// Once the managed ProxyN exists, the route binds to it.
	setManagedProxyIface(t, e, "Proxy0")
	iface, ok := e.resolveRouteIface(r)
	if !ok || iface != "Proxy0" {
		t.Errorf("resolveRouteIface = (%q,%v), want (Proxy0,true)", iface, ok)
	}

	// In TPROXY mode the same route uses the in-Xray split path (and does not
	// resolve to a router interface).
	setXrayIntegration(t, e, "tproxy")
	if _, ok := e.routeUsesXraySplit(e.store.Get(), r); !ok {
		t.Error("tproxy mode: an Xray route should use the in-Xray split path")
	}
	if _, ok := e.resolveRouteIface(r); ok {
		t.Error("tproxy mode: an Xray route should not resolve to a router interface")
	}
}

// TestIntegrationOfProxyMode confirms the connection detail advertises the
// visible Proxy connection in proxy mode and the invisible TPROXY otherwise.
func TestIntegrationOfProxyMode(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")
	c, _ := findConn(e.store.Get(), "conn-x")

	// TPROXY (default in dry-run) → transparent-proxy, invisible.
	iv := e.integrationOf(c)
	if iv.Mode != "transparent-proxy" || iv.VisibleInRouter {
		t.Errorf("tproxy integration = %+v, want transparent-proxy/invisible", iv)
	}

	// Proxy mode → keenetic-proxy, visible + routable; Interface set once ProxyN
	// exists.
	setXrayIntegration(t, e, "proxy")
	iv = e.integrationOf(c)
	if iv.Mode != "keenetic-proxy" || !iv.VisibleInRouter || !iv.RoutableTarget {
		t.Errorf("proxy integration = %+v, want keenetic-proxy/visible/routable", iv)
	}
	setManagedProxyIface(t, e, "Proxy0")
	if iv := e.integrationOf(c); iv.Interface != "Proxy0" {
		t.Errorf("proxy integration Interface = %q, want Proxy0", iv.Interface)
	}
}

// TestTeardownManagedProxyIfaceOnLastDelete confirms the shared ProxyN mapping
// is retired only when the last Xray connection is removed (dry-run: no device
// call, but the state mapping is cleared).
func TestTeardownManagedProxyIfaceOnLastDelete(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")
	addXrayConn(t, e, "conn-y", "Berlin")
	setManagedProxyIface(t, e, "Proxy0")

	if err := e.DeleteConnection("conn-x"); err != nil {
		t.Fatalf("DeleteConnection conn-x: %v", err)
	}
	if e.managedProxyIface() != "Proxy0" {
		t.Error("ProxyN should persist while another Xray connection remains")
	}

	if err := e.DeleteConnection("conn-y"); err != nil {
		t.Fatalf("DeleteConnection conn-y: %v", err)
	}
	if e.managedProxyIface() != "" {
		t.Error("ProxyN mapping should be cleared after the last Xray connection is deleted")
	}
}

// TestSaveSettingsXrayIntegration covers validation + normalisation + the
// proxyClientDown reset on change.
func TestSaveSettingsXrayIntegration(t *testing.T) {
	e := newTestEngine(t)

	if err := e.SaveSettings(map[string]any{"xray_integration": "bogus"}); err == nil {
		t.Error("expected an error for an invalid xray_integration value")
	}
	if err := e.SaveSettings(map[string]any{"xray_integration": "proxy"}); err != nil {
		t.Fatalf("SaveSettings proxy: %v", err)
	}
	if got := e.store.Settings().XrayIntegration; got != "proxy" {
		t.Errorf("stored xray_integration = %q, want proxy", got)
	}
	if got := e.Settings().XrayIntegration; got != "proxy" {
		t.Errorf("view xray_integration = %q, want proxy", got)
	}

	// "auto" normalises to "" (JSON omits it) and displays as "auto".
	if err := e.SaveSettings(map[string]any{"xray_integration": "auto"}); err != nil {
		t.Fatalf("SaveSettings auto: %v", err)
	}
	if got := e.store.Settings().XrayIntegration; got != "" {
		t.Errorf("auto should store as empty, got %q", got)
	}
	if got := e.Settings().XrayIntegration; got != "auto" {
		t.Errorf("view should display empty as auto, got %q", got)
	}

	// Changing the mode clears a latched proxyClientDown.
	e.setProxyClientDown(true)
	if err := e.SaveSettings(map[string]any{"xray_integration": "tproxy"}); err != nil {
		t.Fatalf("SaveSettings tproxy: %v", err)
	}
	if e.isProxyClientDown() {
		t.Error("changing the integration mode should clear proxyClientDown")
	}
}
