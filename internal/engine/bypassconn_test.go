package engine

import (
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/tpws"
)

// TestBypassRouteTargetSentinel confirms a route can target the reserved
// "bypass" target without a real connection, is labelled "DPI Bypass", and
// resolves to the managed bypass Proxy interface only once it exists.
func TestBypassRouteTargetSentinel(t *testing.T) {
	e := newTestEngine(t)

	rv, err := e.CreateRoute("Discord", "discord", nil, nil, bypassTargetID, "")
	if err != nil {
		t.Fatalf("CreateRoute to bypass target: %v", err)
	}
	if rv.TargetConnID != bypassTargetID {
		t.Errorf("route target = %q, want %q", rv.TargetConnID, bypassTargetID)
	}
	if rv.TargetName != "DPI Bypass" {
		t.Errorf("route target name = %q, want \"DPI Bypass\"", rv.TargetName)
	}

	r := model.ServiceRoute{ID: "r1", TargetConnID: bypassTargetID, Enabled: true, Domains: []string{"discord.com"}}
	// No managed bypass interface yet → unresolved.
	if iface, ok := e.resolveRouteIface(r); ok {
		t.Errorf("expected no interface before the bypass iface exists, got %q", iface)
	}
	// Once the managed bypass ProxyN exists, the route binds to it.
	if err := e.recordManagedBypassIface("Proxy1"); err != nil {
		t.Fatal(err)
	}
	if iface, ok := e.resolveRouteIface(r); !ok || iface != "Proxy1" {
		t.Errorf("resolveRouteIface = (%q,%v), want (Proxy1,true)", iface, ok)
	}
}

// TestSaveBypassEnableSeedsRoutes confirms enabling the feature seeds exactly
// the Discord + YouTube default routes (from the preset catalog), marks the
// state seeded, and does not duplicate them on a later save.
func TestSaveBypassEnableSeedsRoutes(t *testing.T) {
	e := newTestEngine(t)

	if err := e.SaveBypass(map[string]any{"enabled": true}); err != nil {
		t.Fatalf("SaveBypass enable: %v", err)
	}
	st := e.store.Get()
	if !st.Bypass.Enabled {
		t.Error("bypass should be enabled")
	}
	if !st.Bypass.Seeded {
		t.Error("bypass should be marked seeded after first enable")
	}
	got := map[string]bool{}
	for _, r := range st.Routes {
		if r.TargetConnID == bypassTargetID {
			got[r.PresetID] = true
		}
	}
	if len(got) != 2 || !got["discord"] || !got["youtube"] {
		t.Errorf("seeded bypass presets = %v, want {discord, youtube}", got)
	}

	// A later save (strategy change, still enabled) must not duplicate the seed.
	if err := e.SaveBypass(map[string]any{"strategy": "--split-pos=1 --oob"}); err != nil {
		t.Fatalf("SaveBypass strategy: %v", err)
	}
	n := 0
	for _, r := range e.store.Get().Routes {
		if r.TargetConnID == bypassTargetID {
			n++
		}
	}
	if n != 2 {
		t.Errorf("bypass routes after re-save = %d, want 2 (no duplication)", n)
	}
	if e.Bypass().Strategy != "--split-pos=1 --oob" {
		t.Errorf("strategy not updated: %q", e.Bypass().Strategy)
	}
}

// TestSaveBypassPortValidation confirms an out-of-range port is rejected (and
// rolled back) while a valid port is stored.
func TestSaveBypassPortValidation(t *testing.T) {
	e := newTestEngine(t)

	if err := e.SaveBypass(map[string]any{"port": float64(70000)}); err == nil {
		t.Error("expected an error for an out-of-range port")
	}
	if e.store.Get().Bypass.Port != 0 {
		t.Errorf("invalid port should not persist, got %d", e.store.Get().Bypass.Port)
	}
	if err := e.SaveBypass(map[string]any{"port": float64(10900)}); err != nil {
		t.Fatalf("SaveBypass valid port: %v", err)
	}
	if e.bypassPort() != 10900 {
		t.Errorf("bypassPort = %d, want 10900", e.bypassPort())
	}
}

// TestSaveBypassDisableTeardown confirms disabling the feature clears the
// managed interface mapping and the enabled flag (dry-run: no device call).
func TestSaveBypassDisableTeardown(t *testing.T) {
	e := newTestEngine(t)
	if err := e.SaveBypass(map[string]any{"enabled": true}); err != nil {
		t.Fatalf("SaveBypass enable: %v", err)
	}
	// Simulate the on-device interface having been registered.
	if err := e.recordManagedBypassIface("Proxy1"); err != nil {
		t.Fatal(err)
	}
	if err := e.SaveBypass(map[string]any{"enabled": false}); err != nil {
		t.Fatalf("SaveBypass disable: %v", err)
	}
	if e.bypassEnabled() {
		t.Error("bypass should be disabled")
	}
	if e.managedBypassIface() != "" {
		t.Error("managed bypass interface should be cleared on disable")
	}
}

// TestSaveBypassEnableRequiresTpws confirms enabling on a device without tpws is
// rejected cleanly and does not latch the feature on.
func TestSaveBypassEnableRequiresTpws(t *testing.T) {
	e := newTestEngine(t)
	if platform.Which("tpws") {
		t.Skip("tpws present on PATH in this environment; pre-flight test not meaningful")
	}
	// Simulate on-device (non-dry-run) with no tpws binary present.
	e.runner.DryRun = false
	defer func() { e.runner.DryRun = true }()

	if err := e.SaveBypass(map[string]any{"enabled": true}); err == nil {
		t.Error("expected an error enabling bypass without tpws installed")
	}
	if e.bypassEnabled() {
		t.Error("bypass must not be enabled when tpws is missing")
	}
}

// TestBypassViewDefaults confirms the view reports sensible defaults off-device.
func TestBypassViewDefaults(t *testing.T) {
	v := newTestEngine(t).Bypass()
	if v.Enabled {
		t.Error("bypass should default to disabled")
	}
	if v.Port != tpws.DefaultPort {
		t.Errorf("default port = %d, want %d", v.Port, tpws.DefaultPort)
	}
	if v.Strategy != tpws.DefaultStrategy {
		t.Errorf("default strategy = %q, want %q", v.Strategy, tpws.DefaultStrategy)
	}
	if v.Target != bypassTargetID {
		t.Errorf("target = %q, want %q", v.Target, bypassTargetID)
	}
}

// TestReconcileBypassDisabledNoop confirms reconcileBypass does nothing when the
// feature is off (no seeding, no state change).
func TestReconcileBypassDisabledNoop(t *testing.T) {
	e := newTestEngine(t)
	e.reconcileBypass()
	if len(e.store.Get().Routes) != 0 {
		t.Error("reconcileBypass must not seed routes when the feature is disabled")
	}
	if e.store.Get().Bypass.Seeded {
		t.Error("reconcileBypass must not mark seeded when disabled")
	}
}
