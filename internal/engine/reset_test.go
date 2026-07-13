package engine

import (
	"testing"
	"time"

	"github.com/miroslavrov/keen-manager/internal/config"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// TestResetAll seeds a fully-configured engine (connection + secret, a route, a
// subscription, failover, kill-switch, managed interfaces, a web password and a
// live session), runs the factory reset, and asserts every one of those is gone
// — persisted state back to defaults, the secret vault emptied, and volatile
// in-memory state cleared.
func TestResetAll(t *testing.T) {
	e := newTestEngine(t)

	// Seed connection + vault secret + a route targeting it.
	addXrayConn(t, e, "conn-x", "Amsterdam")
	if _, err := e.CreateRoute("YouTube", "", []string{"youtube.com"}, nil, "conn-x", ""); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	// Seed a web UI password (persists a hash to the vault + enables auth).
	if err := e.SetPassword("hunter2"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	// Seed a live auth session and the rest of the state surface.
	e.sessMu.Lock()
	e.sessions["tok"] = time.Now().Add(time.Hour)
	e.sessMu.Unlock()
	if err := e.store.Mutate(func(s *model.State) error {
		s.Subscriptions = append(s.Subscriptions, model.Subscription{ID: "sub-1", Name: "blanc", Enabled: true})
		s.ActiveConnID = "conn-x"
		s.KillSwitch = true
		s.ManagedProxyIface = "Proxy0"
		s.ManagedBypassIface = "Proxy1"
		s.Failover.Enabled = true
		s.Failover.Chain = []string{"conn-x"}
		return nil
	}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	// Preconditions.
	if _, ok := e.vault.get("conn-x"); !ok {
		t.Fatal("precondition: vault should hold the seeded server")
	}
	if e.vault.authHash() == "" {
		t.Fatal("precondition: vault should hold the auth hash")
	}

	if err := e.ResetAll(); err != nil {
		t.Fatalf("ResetAll: %v", err)
	}

	// Persisted state is back to first-run defaults.
	got := e.store.Get()
	want := config.DefaultState()
	if len(got.Connections) != 0 {
		t.Errorf("connections not cleared: %+v", got.Connections)
	}
	if len(got.Subscriptions) != 0 {
		t.Errorf("subscriptions not cleared: %+v", got.Subscriptions)
	}
	if len(got.Routes) != 0 {
		t.Errorf("routes not cleared: %+v", got.Routes)
	}
	if got.ActiveConnID != "" {
		t.Errorf("active connection not cleared: %q", got.ActiveConnID)
	}
	if got.KillSwitch {
		t.Error("kill-switch not cleared")
	}
	if got.ManagedProxyIface != "" || got.ManagedBypassIface != "" {
		t.Errorf("managed interfaces not cleared: proxy=%q bypass=%q", got.ManagedProxyIface, got.ManagedBypassIface)
	}
	if got.Failover.Enabled || len(got.Failover.Chain) != 0 {
		t.Errorf("failover not reset: %+v", got.Failover)
	}
	if got.Failover.ProbeTarget != want.Failover.ProbeTarget {
		t.Errorf("failover probe target = %q, want default %q", got.Failover.ProbeTarget, want.Failover.ProbeTarget)
	}
	if got.Settings.Port != want.Settings.Port {
		t.Errorf("port = %d, want default %d", got.Settings.Port, want.Settings.Port)
	}
	if got.Settings.AuthEnabled || got.Settings.PasswordHash != "" {
		t.Error("auth not reset (enabled or hash still set)")
	}
	if got.Version != config.SchemaVersion {
		t.Errorf("schema version = %d, want %d", got.Version, config.SchemaVersion)
	}

	// Vault emptied.
	if _, ok := e.vault.get("conn-x"); ok {
		t.Error("vault server not cleared")
	}
	if e.vault.authHash() != "" {
		t.Error("vault auth hash not cleared")
	}

	// Volatile in-memory state cleared.
	e.sessMu.Lock()
	nSessions := len(e.sessions)
	e.sessMu.Unlock()
	if nSessions != 0 {
		t.Errorf("sessions not cleared: %d remain", nSessions)
	}
	e.mu.RLock()
	nRuntime := len(e.runtime)
	e.mu.RUnlock()
	if nRuntime != 0 {
		t.Errorf("runtime not cleared: %d remain", nRuntime)
	}
}
