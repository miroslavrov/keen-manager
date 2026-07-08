package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/miroslavrov/keen-manager/internal/awg"
	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// nativeHandshakeWindowS bounds how recent a peer handshake must be for a
// native AWG2 interface to count as "up" during verification/probing.
const nativeHandshakeWindowS = 190

// detectKeenetic probes the local RCI endpoint once at startup (best-effort)
// to learn the firmware release and whether native AWG2 is available. It runs
// only on-device (skipped in dry-run) and never fails engine construction: if
// RCI is unreachable, capabilities stay zero-valued and AWG connections fall
// back to the Entware userspace (awg-quick) path.
func (e *Engine) detectKeenetic() {
	if e.runner.DryRun || e.keenetic == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	caps, err := keenetic.DetectCapabilities(ctx, e.keenetic)
	if err != nil {
		e.logs.appendf("keenetic RCI not reachable (%v) — native AWG2 off, using awg-quick fallback", err)
		return
	}
	e.caps = caps
	if caps.Release != "" {
		e.Platform.OSVersion = caps.Release
	}
	e.logs.appendf("keenetic detected: firmware=%q wireguard=%v native-awg2=%v",
		caps.Release, caps.HasWireguard, caps.SupportsAWG2)
}

// useNativeAWG reports whether AWG connections should be provisioned through
// the KeeneticOS native AWG2 RCI path (firmware >= 5.01.A.3 with the wireguard
// component) rather than the Entware userspace awg-quick fallback. It is false
// off-device / in dry-run and false when the startup RCI probe found no native
// wireguard support.
func (e *Engine) useNativeAWG() bool {
	return e.keenetic != nil && !e.runner.DryRun && e.caps.HasWireguard && e.caps.SupportsAWG2
}

// nativeIface returns the native Wireguard interface name recorded for a
// connection (created via RCI import), if any.
func (e *Engine) nativeIface(connID string) (string, bool) {
	st := e.store.Get()
	if st.NativeIfaces == nil {
		return "", false
	}
	name, ok := st.NativeIfaces[connID]
	return name, ok && name != ""
}

func (e *Engine) recordNativeIface(connID, name string) error {
	return e.store.Mutate(func(s *model.State) error {
		if s.NativeIfaces == nil {
			s.NativeIfaces = map[string]string{}
		}
		s.NativeIfaces[connID] = name
		return nil
	})
}

func (e *Engine) clearNativeIface(connID string) {
	_ = e.store.Mutate(func(s *model.State) error {
		delete(s.NativeIfaces, connID)
		return nil
	})
}

// awgNativeUp provisions an AWG connection through the native RCI import path:
// generate a standard AWG .conf, hand it to NDMS to parse+create a native
// Wireguard interface, bring it up, mark it global for routing (best-effort),
// persist the interface-name mapping, and save the running config so it
// survives a reboot.
//
// Every step is reversible: on any hard failure the freshly-created interface
// is deleted, and Activate's verify-then-rollback deadman guards the end result
// so a misconfiguration can never strand the router — deleting the interface
// makes KeeneticOS fall back to the WAN automatically.
func (e *Engine) awgNativeUp(c model.Connection) error {
	if c.AWG == nil {
		return fmt.Errorf("missing AmneziaWG config")
	}
	if err := awg.Validate(c.AWG); err != nil {
		return fmt.Errorf("awg config invalid: %w", err)
	}
	conf := awg.Generate(c.AWG)

	ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
	defer cancel()

	// Replace any stale interface previously created for this connection so a
	// re-activation cannot leave orphaned Wireguard{N} interfaces behind.
	if prev, ok := e.nativeIface(c.ID); ok {
		_ = keenetic.DeleteInterface(ctx, e.keenetic, prev)
		e.clearNativeIface(c.ID)
	}

	res, err := keenetic.ImportConfig(ctx, e.keenetic, []byte(conf), "km-"+awgIface(c.ID)+".conf")
	if err != nil {
		return fmt.Errorf("native awg import: %w", err)
	}
	name := res.Created
	e.Logf("native AWG2: imported %s as interface %s (firmware %s)", c.Name, name, e.caps.Release)

	if err := keenetic.InterfaceUp(ctx, e.keenetic, name); err != nil {
		_ = keenetic.DeleteInterface(ctx, e.keenetic, name)
		return fmt.Errorf("native awg bring-up: %w", err)
	}
	// Make it eligible for internet routing. Best-effort: firmware variance in
	// the "global" shape must not fail activation — the interface is up and can
	// be prioritised from the Keenetic UI; Activate's probe decides success.
	if err := keenetic.SetInterfaceGlobal(ctx, e.keenetic, name, true); err != nil {
		e.Logf("native AWG2: could not mark %s global (%v) — set its priority in the Keenetic UI if traffic does not route", name, err)
	}
	if err := e.recordNativeIface(c.ID, name); err != nil {
		e.Logf("native AWG2: warning: could not persist iface mapping for %s: %v", c.Name, err)
	}
	if err := e.keenetic.Save(ctx); err != nil {
		e.Logf("native AWG2: warning: RCI configuration save failed: %v", err)
	}
	return nil
}

// awgNativeDown deletes the native Wireguard interface created for a connection
// and persists the removal. It is a no-op when the connection has no native
// interface recorded (i.e. it was brought up via the userspace path).
func (e *Engine) awgNativeDown(connID string) error {
	name, ok := e.nativeIface(connID)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 30*time.Second)
	defer cancel()
	err := keenetic.DeleteInterface(ctx, e.keenetic, name)
	e.clearNativeIface(connID)
	if serr := e.keenetic.Save(ctx); serr != nil {
		e.Logf("native AWG2: warning: RCI save after delete failed: %v", serr)
	}
	if err != nil {
		return fmt.Errorf("native awg teardown: %w", err)
	}
	e.Logf("native AWG2: removed interface %s", name)
	return nil
}

// awgNativeHealthy reports whether the native interface for connID has a recent
// peer handshake — an honest through-tunnel liveness signal, unlike a direct
// HTTP probe which can succeed over the WAN even when the tunnel is not the
// active default route.
func (e *Engine) awgNativeHealthy(connID string) bool {
	name, ok := e.nativeIface(connID)
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 6*time.Second)
	defer cancel()
	st, err := keenetic.InterfaceStatus(ctx, e.keenetic, name)
	if err != nil {
		return false
	}
	for _, p := range st.Peers {
		if p.Online && p.LastHandshakeAgeS <= nativeHandshakeWindowS {
			return true
		}
	}
	return false
}

// reconcile re-establishes the connection that was active before a daemon
// restart or router reboot. It no-ops when the active tunnel is already healthy
// (e.g. a native interface restored from KeeneticOS startup-config), so a
// routine daemon restart never churns a working tunnel.
func (e *Engine) reconcile() {
	if e.runner.DryRun {
		return
	}
	st := e.store.Get()
	id := st.ActiveConnID
	if id == "" {
		return
	}
	c, ok := findConn(st, id)
	if !ok || !c.Enabled {
		e.Logf("reconcile: previously-active connection %q is gone or disabled", id)
		return
	}
	if e.verifyOnce(c) {
		e.setRuntime(id, model.RuntimeStatus{ConnID: id, Status: model.StatusUp, Active: true, LastCheck: time.Now()})
		e.Logf("reconcile: active connection %s already healthy after restart", c.Name)
		return
	}
	e.Logf("reconcile: re-activating %s after restart", c.Name)
	if err := e.Activate(id); err != nil {
		e.Logf("reconcile: re-activate %s failed: %v", c.Name, err)
	}
}

// probeAWG computes runtime status for an AmneziaWG connection. AWG endpoints
// are UDP, so (unlike Xray) a TCP ping says nothing — liveness comes from the
// WireGuard handshake: via RCI InterfaceStatus on the native path, or `awg
// show` on the userspace path.
func (e *Engine) probeAWG(c model.Connection, rs model.RuntimeStatus) model.RuntimeStatus {
	age := -1
	if a, native := e.nativeHandshakeAge(c.ID); native {
		age = a
	} else if h, err := e.awg.Show(awgIface(c.ID)); err == nil {
		age = h.HandshakeAgeSec
		rs.RxBytes = h.RxBytes
		rs.TxBytes = h.TxBytes
	}
	if age >= 0 {
		rs.HandshakeAge = age
	}
	switch {
	case rs.Active:
		if e.verifyOnce(c) {
			rs.Status = model.StatusUp
		} else {
			rs.Status = model.StatusDegraded
			rs.Message = "tunnel handshake stale"
		}
	case age > 0 && age <= nativeHandshakeWindowS:
		rs.Status = model.StatusUp
	default:
		// A non-active AWG tunnel isn't running, so its UDP endpoint can't be
		// cheaply probed; report standby rather than a misleading "down".
		rs.Status = model.StatusUnknown
		rs.Message = "standby"
	}
	return rs
}

// nativeHandshakeAge returns the newest online peer's handshake age (seconds)
// for a connection's native interface. The bool reports whether the connection
// is on the native path at all (so callers can fall back to awg-quick's
// `awg show` parse when it is not).
func (e *Engine) nativeHandshakeAge(connID string) (int, bool) {
	name, ok := e.nativeIface(connID)
	if !ok {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 5*time.Second)
	defer cancel()
	st, err := keenetic.InterfaceStatus(ctx, e.keenetic, name)
	if err != nil {
		return 0, true
	}
	for _, p := range st.Peers {
		if p.Online {
			return int(p.LastHandshakeAgeS), true
		}
	}
	return 0, true
}
