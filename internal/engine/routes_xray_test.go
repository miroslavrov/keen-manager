package engine

import (
	"path/filepath"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/xray"
)

// newTestEngine builds a dry-run Engine backed by temp dirs (no device I/O).
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	p := platform.Paths{
		Root:        dir,
		DataDir:     dir,
		BackupDir:   filepath.Join(dir, "backups"),
		LogDir:      filepath.Join(dir, "log"),
		RunDir:      filepath.Join(dir, "run"),
		XrayConfDir: filepath.Join(dir, "xray"),
	}
	e, err := New(p, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func addXrayConn(t *testing.T, e *Engine, id, name string) {
	t.Helper()
	srv := model.Server{
		ID: id, Name: name, Protocol: model.ProtoVLESS,
		Address: "1.1.1.1", Port: 443, UUID: "839d4028-2984-4e66-8e62-f4c127b52f49",
		Flow: "xtls-rprx-vision", Security: "reality", Network: "tcp",
		SNI: "cdn3-87.yahoo.com", Fingerprint: "firefox",
		PublicKey: "CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw", ShortID: "07ddc43269d197c0",
	}
	e.vault.put(id, srv)
	if err := e.store.Mutate(func(s *model.State) error {
		s.Connections = append(s.Connections, model.Connection{
			ID: id, Type: model.ConnXray, Name: name, Enabled: true, Xray: publicServer(&srv),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestXrayRouteMembership confirms an enabled route targeting an Xray connection
// contributes its domains/subnets to that connection's split-tunnel membership,
// and that excludeID drops it (used by teardown rebuilds).
func TestXrayRouteMembership(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")

	rv, err := e.CreateRoute("YouTube", "", []string{"YouTube.com", "youtube.com"}, []string{"203.0.113.0/24"}, "conn-x", "")
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	// Not active yet → the route is pending, not applied.
	if rv.Applied {
		t.Error("route should be pending until its Xray connection is active")
	}

	domains, subnets := e.xrayRouteMembership("conn-x", "")
	if len(domains) != 1 || domains[0] != "youtube.com" {
		t.Errorf("membership domains = %v, want [youtube.com] (deduped/lowercased)", domains)
	}
	if len(subnets) != 1 || subnets[0] != "203.0.113.0/24" {
		t.Errorf("membership subnets = %v, want [203.0.113.0/24]", subnets)
	}

	// Excluding the route empties the membership.
	if d, s := e.xrayRouteMembership("conn-x", rv.ID); len(d) != 0 || len(s) != 0 {
		t.Errorf("excluded membership should be empty, got domains=%v subnets=%v", d, s)
	}
}

// TestBuildActiveXraySplitFromRoute confirms the active Xray config gains a
// split-tunnel direct catch-all once a route targets the connection, and is a
// full tunnel (no catch-all) when none do.
func TestBuildActiveXraySplitFromRoute(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")
	srv, ok := e.vault.get("conn-x")
	if !ok {
		t.Fatal("server missing from vault")
	}

	// No routes yet → full tunnel, no direct catch-all.
	cfg, err := e.buildActiveXray("conn-x", srv, "")
	if err != nil {
		t.Fatal(err)
	}
	if hasDirectCatchAll(cfg) {
		t.Error("expected a full tunnel before any route targets the connection")
	}

	if _, err := e.CreateRoute("YouTube", "", []string{"youtube.com"}, nil, "conn-x", ""); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	cfg, err = e.buildActiveXray("conn-x", srv, "")
	if err != nil {
		t.Fatal(err)
	}
	if !hasDirectCatchAll(cfg) {
		t.Error("expected split-tunnel routing (a direct catch-all) after a route targets the connection")
	}
}

// hasDirectCatchAll reports whether cfg carries a split-tunnel direct catch-all
// rule (present only when per-service routing is compiled in).
func hasDirectCatchAll(cfg *xray.Config) bool {
	if cfg == nil || cfg.Routing == nil {
		return false
	}
	for _, r := range cfg.Routing.Rules {
		if r.OutboundTag == "direct" {
			return true
		}
	}
	return false
}
