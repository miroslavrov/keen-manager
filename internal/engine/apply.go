package engine

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/xray"
)

// This file implements the "don't brick the router" apply model:
//
//   1. validate before apply   — Xray configs are `xray -test`ed and AWG fields
//                                checked before anything is written or started;
//   2. bring the tunnel up     — via the proven awg-quick / xray init paths;
//   3. capture routing         — only after the tunnel is up (TPROXY for Xray;
//                                awg-quick owns its own routes);
//   4. verify + rollback       — probe end-to-end connectivity within the
//                                rollback timeout and, if it fails, revert to
//                                the previously-active connection automatically.
//
// In dry-run mode every device command is inert, so steps 2–4's network checks
// are skipped (there is nothing real to probe).

// Activate makes a connection the active default path, with automatic rollback.
func (e *Engine) Activate(id string) error {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return fmt.Errorf("connection %s not found", id)
	}
	if !c.Enabled {
		return fmt.Errorf("connection %q is disabled", c.Name)
	}
	prev := st.ActiveConnID
	e.Logf("activating %s (previous active: %q)", c.Name, prev)

	if err := e.bringUp(c); err != nil {
		return fmt.Errorf("bring up %s: %w", c.Name, err)
	}
	if err := e.setActive(id); err != nil {
		return err
	}
	if err := e.applyRouting(c); err != nil {
		e.Logf("routing failed for %s: %v — rolling back", c.Name, err)
		e.rollback(prev, c)
		return fmt.Errorf("apply routing: %w", err)
	}

	// Verify + rollback deadman (skipped in dry-run: nothing real to probe).
	if !e.runner.DryRun {
		if !e.verifyActive(c) {
			e.Logf("post-activate probe failed for %s — rolling back to %q", c.Name, prev)
			e.rollback(prev, c)
			return fmt.Errorf("activation verification failed for %q; rolled back", c.Name)
		}
	}

	e.foResetFail()
	e.setRuntime(id, model.RuntimeStatus{
		ConnID: id, Status: model.StatusUp, Active: true, LastCheck: time.Now(),
	})
	e.Logf("active connection is now %s", c.Name)
	e.publishState()
	go e.probeOne(id)
	return nil
}

// bringUp starts a connection's underlying service without changing routing.
func (e *Engine) bringUp(c model.Connection) error {
	switch c.Type {
	case model.ConnAWG:
		if c.AWG == nil {
			return fmt.Errorf("missing AmneziaWG config")
		}
		// Prefer the KeeneticOS native AWG2 path on capable firmware (5.1.0+);
		// fall back to the Entware userspace awg-quick path otherwise.
		if e.useNativeAWG() {
			return e.awgNativeUp(c)
		}
		return e.awg.Up(awgIface(c.ID), c.AWG)
	case model.ConnXray:
		srv, ok := e.vault.get(c.ID)
		if !ok {
			return fmt.Errorf("server credentials missing from vault")
		}
		cfg, err := e.buildActiveXray(srv)
		if err != nil {
			return err
		}
		_, err = e.xray.Apply(cfg) // writes + validates + restarts
		return err
	}
	return fmt.Errorf("unknown connection type %q", c.Type)
}

// bringDown stops a connection's service and releases any capture it owned.
func (e *Engine) bringDown(c model.Connection) error {
	switch c.Type {
	case model.ConnAWG:
		// A recorded native interface means this tunnel was brought up via RCI
		// import; tear it down the same way. Otherwise use the userspace path.
		if _, native := e.nativeIface(c.ID); native {
			return e.awgNativeDown(c.ID)
		}
		return e.awg.Down(awgIface(c.ID))
	case model.ConnXray:
		// Release TPROXY capture first so traffic is never sent to a dead proxy.
		_ = e.route.DisableTProxy()
		return e.xray.Stop()
	}
	return nil
}

// applyRouting installs the capture rules appropriate to the connection type.
func (e *Engine) applyRouting(c model.Connection) error {
	switch c.Type {
	case model.ConnXray:
		if err := e.route.EnableTProxy(); err != nil {
			return err
		}
	case model.ConnAWG:
		// awg-quick installs routes from AllowedIPs; nothing extra to add.
	}
	if e.store.Get().KillSwitch {
		_ = e.route.EnableKillSwitch()
	}
	return nil
}

// revertRouting tears down capture rules (idempotent).
func (e *Engine) revertRouting(c model.Connection) {
	if c.Type == model.ConnXray {
		_ = e.route.DisableTProxy()
	}
	_ = e.route.DisableKillSwitch()
}

// rollback restores the previously-active connection after a failed activation.
func (e *Engine) rollback(prev string, failed model.Connection) {
	e.revertRouting(failed)
	_ = e.bringDown(failed)

	if prev != "" && prev != failed.ID {
		if pc, ok := findConn(e.store.Get(), prev); ok && pc.Enabled {
			if err := e.bringUp(pc); err == nil {
				_ = e.setActive(prev)
				_ = e.applyRouting(pc)
				e.Logf("rolled back to %s", pc.Name)
				e.publishState()
				return
			}
		}
	}
	_ = e.setActive("")
	e.publishState()
}

// verifyActive probes end-to-end connectivity through the freshly-activated
// connection, retrying until the rollback timeout elapses.
func (e *Engine) verifyActive(c model.Connection) bool {
	timeout := time.Duration(e.rollbackTimeout()) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	deadline := time.Now().Add(timeout)
	target := e.probeTarget()
	const per = 6 * time.Second

	for attempt := 1; time.Now().Before(deadline); attempt++ {
		// Native AWG2: the authoritative signal is a recent peer handshake (a
		// direct HTTP probe can pass over the WAN even when the tunnel is not
		// yet the active route), so check it before falling back to HTTP.
		if c.Type == model.ConnAWG {
			if _, native := e.nativeIface(c.ID); native {
				if e.awgNativeHealthy(c.ID) {
					e.Logf("verify %s: native AWG2 tunnel established (attempt %d)", c.Name, attempt)
					return true
				}
				time.Sleep(2 * time.Second)
				continue
			}
		}
		ctx, cancel := context.WithTimeout(e.baseCtx(), per)
		var p health.Probe
		switch c.Type {
		case model.ConnXray:
			p = health.SOCKSHTTP(ctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, per)
		case model.ConnAWG:
			p = health.DirectHTTP(ctx, target, per)
		}
		cancel()
		if p.OK {
			e.Logf("verify %s: reachable (%dms, attempt %d)", c.Name, p.LatencyMs, attempt)
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// setActive persists the active connection id.
func (e *Engine) setActive(id string) error {
	return e.store.Mutate(func(s *model.State) error { s.ActiveConnID = id; return nil })
}

// buildActiveXray produces the config for the single active Xray connection:
// a local SOCKS inbound (LAN proxy + probe target) plus a TPROXY inbound for
// transparent capture. Outbounds carry SO_MARK 255 so Xray's own egress is not
// re-captured (route.Manager excludes that mark).
func (e *Engine) buildActiveXray(server model.Server) (*xray.Config, error) {
	opts := xray.Defaults()
	opts.SocksPort = xraySocksPort
	opts.EnableTProxy = true
	opts.TProxyPort = e.route.TProxyPort
	opts.EnableBalancer = false
	if pt := e.probeTarget(); pt != "" {
		opts.ProbeURL = pt
		opts.PingDestination = pt
	}
	return xray.BuildConfig([]model.Server{server}, opts)
}

// probeTarget is the connectivity check URL (failover probe target, or default).
func (e *Engine) probeTarget() string {
	if pt := strings.TrimSpace(e.store.Get().Failover.ProbeTarget); pt != "" {
		return pt
	}
	return "https://www.gstatic.com/generate_204"
}

func (e *Engine) rollbackTimeout() int { return e.store.Settings().RollbackTimeoutS }

func (e *Engine) baseCtx() context.Context {
	if e.ctx != nil {
		return e.ctx
	}
	return context.Background()
}

// ----- probing -----

// probeOne probes a single connection now and stores its runtime status.
func (e *Engine) probeOne(id string) {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return
	}
	e.setRuntime(id, e.probeConnection(st, c))
}

// probeConnection computes a fresh runtime status for a connection. The baseline
// signal is a TCP ping to the endpoint (cheap, and the thing that goes dark when
// a server is blocked or its IP rotates). The active connection additionally
// gets an end-to-end through-tunnel probe.
func (e *Engine) probeConnection(st model.State, c model.Connection) model.RuntimeStatus {
	rs := model.RuntimeStatus{
		ConnID:    c.ID,
		LastCheck: time.Now(),
		Active:    st.ActiveConnID == c.ID,
	}
	if !c.Enabled {
		rs.Status = model.StatusDisabled
		return rs
	}
	// Off-device / dry-run: don't touch the network; report an honest unknown.
	if e.runner.DryRun {
		rs.Status = model.StatusChecking
		rs.Message = "probing disabled (dry-run)"
		return rs
	}

	// AWG endpoints are UDP — a TCP ping to them is meaningless — so AWG uses
	// WireGuard handshake liveness instead of the endpoint TCP probe that Xray
	// (TCP-based) relies on. Without this an active, healthy AWG tunnel would
	// always read as "down" because its UDP port never answers a TCP connect.
	if c.Type == model.ConnAWG {
		return e.probeAWG(c, rs)
	}

	host, port := endpointHostPort(c)
	reachable := false
	if host != "" && port > 0 {
		ctx, cancel := context.WithTimeout(e.baseCtx(), 6*time.Second)
		p := health.TCPPing(ctx, host, port, 6*time.Second)
		cancel()
		reachable = p.OK
		rs.LatencyMs = p.LatencyMs
	}

	switch {
	case !reachable:
		rs.Status = model.StatusDown
		rs.Message = "endpoint unreachable"
	case rs.Active:
		if e.verifyOnce(c) {
			rs.Status = model.StatusUp
		} else {
			rs.Status = model.StatusDegraded
			rs.Message = "endpoint up but tunnel probe failed"
		}
	default:
		rs.Status = model.StatusUp
	}
	return rs
}

// verifyOnce is a single (non-retrying) end-to-end probe of the active path.
func (e *Engine) verifyOnce(c model.Connection) bool {
	// Native AWG2: use the peer-handshake signal (see verifyActive).
	if c.Type == model.ConnAWG {
		if _, native := e.nativeIface(c.ID); native {
			return e.awgNativeHealthy(c.ID)
		}
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 6*time.Second)
	defer cancel()
	target := e.probeTarget()
	switch c.Type {
	case model.ConnXray:
		return health.SOCKSHTTP(ctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, 6*time.Second).OK
	case model.ConnAWG:
		return health.DirectHTTP(ctx, target, 6*time.Second).OK
	}
	return false
}

// awgIface derives a stable, valid (<=15 char) interface name from a conn id.
func awgIface(id string) string {
	b := strings.Builder{}
	b.WriteString("km")
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 15 {
		s = s[:15]
	}
	return s
}
