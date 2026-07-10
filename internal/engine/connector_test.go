package engine

import (
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

func TestConnectorPauseRemembersActive(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "c1", "Helsinki")
	if err := e.store.Mutate(func(s *model.State) error { s.ActiveConnID = "c1"; return nil }); err != nil {
		t.Fatal(err)
	}

	if !e.ConnectorEnabled() {
		t.Fatal("connector should start enabled")
	}
	if err := e.SetConnectorEnabled(false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	st := e.store.Get()
	if !st.TunnelPaused {
		t.Error("TunnelPaused should be true after switching the connector off")
	}
	if st.PausedConnID != "c1" {
		t.Errorf("PausedConnID = %q, want c1 (remembered for restore)", st.PausedConnID)
	}
	if st.ActiveConnID != "" {
		t.Errorf("ActiveConnID = %q, want empty while paused", st.ActiveConnID)
	}
	if e.ConnectorEnabled() {
		t.Error("ConnectorEnabled() should be false while paused")
	}
}

func TestConnectorResumeClearsPause(t *testing.T) {
	e := newTestEngine(t)
	// Pause with nothing active: there is nothing to restore, so resume must not
	// error and must clear the paused flags.
	if err := e.SetConnectorEnabled(false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if !e.store.Get().TunnelPaused {
		t.Fatal("expected paused")
	}
	if err := e.SetConnectorEnabled(true); err != nil {
		t.Fatalf("resume: %v", err)
	}
	st := e.store.Get()
	if st.TunnelPaused {
		t.Error("TunnelPaused should be false after switching the connector on")
	}
	if st.PausedConnID != "" {
		t.Errorf("PausedConnID = %q, want cleared", st.PausedConnID)
	}
	if !e.ConnectorEnabled() {
		t.Error("ConnectorEnabled() should be true after resume")
	}
}
