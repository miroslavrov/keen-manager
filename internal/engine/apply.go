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
	"github.com/miroslavrov/keen-manager/internal/platform"
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
// It runs unbounded — the caller waits — so it is the right entry point for
// user-initiated activations (HTTP/CLI). Any explicit activation also clears the
// master connector pause: asking for a specific tunnel means "connector on".
func (e *Engine) Activate(id string) error {
	e.clearConnectorPause()
	return e.activate(e.baseCtx(), id)
}

// clearConnectorPause turns the master connector back on. No-op when not paused.
// Called from every interactive activation so a user action implicitly resumes
// the connector; the background loops never call this (they respect the pause).
func (e *Engine) clearConnectorPause() {
	if !e.store.Get().TunnelPaused {
		return
	}
	_ = e.store.Mutate(func(s *model.State) error {
		s.TunnelPaused = false
		s.PausedConnID = ""
		return nil
	})
}

// activate is Activate with an explicit context. Background callers on the
// shared health/failover goroutine use activateWithin to pass a per-attempt
// deadline so one hung bring-up can't stall the loop; the long-running steps
// (xray-core download and the post-activate verify) observe ctx and bail when
// it fires.
func (e *Engine) activate(ctx context.Context, id string) error {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return fmt.Errorf("connection %s not found", id)
	}
	if !c.Enabled {
		return fmt.Errorf("connection %q is disabled", c.Name)
	}
	if !subEnabled(st, c.SubscriptionID) {
		return fmt.Errorf("connection %q belongs to a disabled subscription — enable the subscription first", c.Name)
	}
	prev := st.ActiveConnID
	e.Logf("activating %s (previous active: %q)", c.Name, prev)

	if err := e.bringUpCtx(ctx, c); err != nil {
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
		if ok, detail := e.verifyActive(ctx, c); !ok {
			target := e.probeTarget()
			reason := firstNonEmpty(detail, "no response before the rollback timeout")
			// For Xray, distil the tunnel's OWN error log so the message explains
			// WHY it failed (dial reset, i/o timeout, REALITY mismatch) instead of
			// only that it "did not carry traffic". Needs the debug loglevel to be
			// most useful, but even at warning it often catches the failing line.
			if c.Type == model.ConnXray {
				if xr := e.xrayFailureReason(); xr != "" {
					reason = reason + "; xray log: " + xr
				}
			}
			e.Logf("post-activate probe failed for %s (target=%s: %s) — rolling back to %q", c.Name, target, reason, prev)
			e.rollback(prev, c)
			return fmt.Errorf("activation verification failed for %q: the tunnel did not carry traffic to %s (%s); rolled back — check the server is reachable and not DPI-blocked, or set a different probe target on the Failover page", c.Name, target, reason)
		}
	}

	// One exit point: now that the new connection has verified, tear down the
	// native interface of the connection we switched away from, so keen-manager
	// never leaves a growing pile of WireguardN interfaces on the router as the
	// user tries locations. Done only after verify so a failed switch never
	// removes the working tunnel (rollback restores prev instead).
	e.supersedePrevNativeIface(prev, id)

	e.foResetFail()
	e.setRuntime(id, model.RuntimeStatus{
		ConnID: id, Status: model.StatusUp, Active: true, LastCheck: time.Now(),
	})
	e.Logf("active connection is now %s", c.Name)
	e.publishState()
	go e.probeOne(id)
	// Re-apply enabled-but-pending service routes that were waiting on this
	// connection. AWG routes (and, in proxy-connection mode, Xray routes) need
	// the native/Proxy interface that now exists, so reconcile them onto the
	// router's dns-proxy stack. In TPROXY mode Xray routes were compiled into the
	// config bringUp just applied, so they are already live and only need marking.
	switch c.Type {
	case model.ConnAWG:
		go e.reconcileRoutes()
	case model.ConnXray:
		if e.xrayProxyMode() {
			go e.reconcileRoutes()
		} else {
			e.markActiveXrayRoutesApplied(c.ID)
		}
	}
	return nil
}

// supersedePrevNativeIface enforces the single-exit-point model after a
// successful activation: it removes the previously-active connection's native
// AWG interface (and any service routes pinned to it) so only the active
// tunnel's interface remains on the router. No-op off-device, when there is no
// previous, or when the previous connection had no native interface.
func (e *Engine) supersedePrevNativeIface(prev, active string) {
	if e.runner.DryRun || prev == "" || prev == active {
		return
	}
	ifaceName, ok := e.nativeIface(prev)
	if !ok {
		return
	}
	// Drop any dns-proxy routes bound to the interface we're about to delete so
	// no dangling route is left behind; they revert to pending and re-apply if
	// their connection is activated again.
	for _, r := range e.store.Get().Routes {
		if name, ok := e.resolveRouteIface(r); ok && name == ifaceName {
			_ = e.unapplyRoute(r)
		}
	}
	if err := e.awgNativeDown(prev); err != nil {
		e.Logf("could not remove superseded interface %s: %v", ifaceName, err)
		return
	}
	e.Logf("removed superseded native interface %s (previous active tunnel)", ifaceName)
}

// markActiveXrayRoutesApplied flags every enabled route targeting the active
// Xray connection as applied — bringUp already compiled them into the running
// config, so they are live the moment the connection is up.
func (e *Engine) markActiveXrayRoutesApplied(connID string) {
	for _, r := range e.store.Get().Routes {
		if r.Enabled && r.TargetConnID == connID {
			e.markRouteApplied(r.ID, nil, true)
		}
	}
}

// bringUp starts a connection's underlying service without changing routing.
// It runs unbounded (uses the engine's base context); background callers that
// need a deadline go through activate → bringUpCtx.
func (e *Engine) bringUp(c model.Connection) error { return e.bringUpCtx(e.baseCtx(), c) }

// bringUpCtx is bringUp with an explicit context so a bounded activation can cap
// the one step that can block for minutes — the first-run xray-core download.
func (e *Engine) bringUpCtx(ctx context.Context, c model.Connection) error {
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
		// Provision xray-core itself if missing (no-op when present / dry-run).
		// The one-time download runs over the current WAN before we capture it,
		// bounded by the smaller of 4 minutes and the caller's deadline.
		ictx, icancel := context.WithTimeout(ctx, 4*time.Minute)
		err := e.xray.Ensure(ictx)
		icancel()
		if err != nil {
			return fmt.Errorf("xray-core not available: %w", err)
		}
		// Start this attempt's error log fresh so a later failure reason reflects
		// only this bring-up (not a stale line from a previous server/attempt).
		e.xray.TruncateErrorLog()
		// Proxy-connection mode: apply the SOCKS-only config and register the
		// single managed ProxyN exit point (falls back to TPROXY on rejection).
		if e.xrayProxyMode() {
			return e.bringUpXrayProxy(c.ID, srv)
		}
		cfg, err := e.buildActiveXray(c.ID, srv, "")
		if err != nil {
			return err
		}
		// Hot-reload path: if Xray is already running (switching servers),
		// try gRPC API to swap outbounds without a process restart.
		// Falls back to Apply (full restart) on any failure.
		if e.xrayRunning() && !e.xrayProxyMode() {
			_, err = e.xray.HotReload(cfg)
		} else {
			_, err = e.xray.Apply(cfg) // writes + validates + restarts
		}
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
		// In proxy-connection mode there is no capture to release, and the shared
		// ProxyN exit point is intentionally left in place (routes survive; it is
		// only removed when the last Xray connection is deleted).
		if !e.xrayProxyMode() {
			_ = e.route.DisableTProxy()
		}
		return e.xray.Stop()
	}
	return nil
}

// applyRouting installs the capture rules appropriate to the connection type.
func (e *Engine) applyRouting(c model.Connection) error {
	switch c.Type {
	case model.ConnXray:
		if !e.xrayProxyMode() {
			// TPROXY fallback: install the transparent-proxy capture.
			if err := e.route.EnableTProxy(); err != nil {
				return err
			}
		}
		// proxy-connection mode: nothing to capture and nothing to make "default".
		// The selected traffic reaches the tunnel via per-domain dns-proxy routes
		// bound to ProxyN (applied per-route on the Routes page). ProxyN is
		// deliberately NOT marked as the global/default internet connection — a
		// SOCKS-proxy default loops the router's own DNS + Xray server-upstream
		// back through the proxy (see ensureManagedProxyIface).
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
	// Only the TPROXY path installs capture rules; proxy-connection mode routes
	// natively and leaves the shared ProxyN in place.
	if c.Type == model.ConnXray && !e.xrayProxyMode() {
		_ = e.route.DisableTProxy()
	}
	_ = e.route.DisableKillSwitch()
}

// rollback restores the previously-active connection after a failed activation.
func (e *Engine) rollback(prev string, failed model.Connection) {
	e.revertRouting(failed)
	_ = e.bringDown(failed)

	if prev != "" && prev != failed.ID {
		if pc, ok := findConn(e.store.Get(), prev); ok && connEligible(e.store.Get(), pc) {
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

// xrayRunning reports whether the Xray process is currently active (has a
// managed config and the process is alive). Used to decide between hot-reload
// (gRPC API swap) and a full Apply (process restart).
func (e *Engine) xrayRunning() bool {
	if e.runner.DryRun {
		return false
	}
	if !platform.FileExists(e.xray.ConfigPath()) {
		return false
	}
	res := e.runner.Run("pgrep", "-f", "xray run")
	return res.Err == nil
}

// verifyActive probes end-to-end connectivity through the freshly-activated
// connection, retrying until the rollback timeout elapses. It returns whether
// the path verified and, on failure, a short human detail of the last probe
// error (surfaced to the UI so the user learns WHY activation failed).
//
// For Xray in TPROXY mode, the SOCKS probe alone is insufficient: it only
// confirms Xray can reach the server, not that LAN traffic is captured into
// the tunnel. After a successful SOCKS probe, Verify() checks the TPROXY
// iptables chain is installed, so "connected" means traffic actually flows
// through the tunnel rather than a bare SOCKS reachability win.
func (e *Engine) verifyActive(ctx context.Context, c model.Connection) (bool, string) {
	timeout := time.Duration(e.rollbackTimeout()) * time.Second
	deadline := time.Now().Add(timeout)
	target := e.probeTarget()
	const per = 6 * time.Second

	// For Xray, wait for the SOCKS inbound to start listening before the
	// first probe. Xray needs a moment after the init script restarts, and
	// a premature probe would falsely report "connection refused".
	if c.Type == model.ConnXray && !e.runner.DryRun {
		waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
		e.waitPortReady(waitCtx, xraySocksHost, xraySocksPort)
		waitCancel()
	}

	lastDetail := ""
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		// Bail immediately if the caller's context (e.g. a bounded failover
		// attempt) has been cancelled, so a hung activation can't pin the loop.
		if err := ctx.Err(); err != nil {
			return false, firstNonEmpty(lastDetail, "activation attempt timed out")
		}
		// Native AWG2: the authoritative signal is a recent peer handshake (a
		// direct HTTP probe can pass over the WAN even when the tunnel is not
		// yet the active route), so check it before falling back to HTTP.
		if c.Type == model.ConnAWG {
			if _, native := e.nativeIface(c.ID); native {
				if e.awgNativeHealthy(c.ID) {
					e.Logf("verify %s: native AWG2 tunnel established (attempt %d)", c.Name, attempt)
					return true, ""
				}
				lastDetail = "no recent AmneziaWG peer handshake"
				if !sleepCtx(ctx, 2*time.Second) {
					return false, firstNonEmpty(lastDetail, "activation attempt timed out")
				}
				continue
			}
		}
		pctx, cancel := context.WithTimeout(ctx, per)
		var p health.Probe
		switch c.Type {
		case model.ConnXray:
			p = health.SOCKSHTTP(pctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, per)
		case model.ConnAWG:
			p = health.DirectHTTP(pctx, target, per)
		}
		cancel()
		if p.OK {
			// For Xray in TPROXY mode, the SOCKS probe only confirms Xray
			// can reach the server. The real question is whether LAN
			// traffic is captured into the tunnel, so verify the TPROXY
			// chain is installed before declaring success. Without this
			// check, "connected" can be true via SOCKS while TPROXY rules
			// are missing (e.g. ndm flushed them) and no LAN traffic flows.
			if c.Type == model.ConnXray && !e.xrayProxyMode() && !e.runner.DryRun {
				if err := e.route.Verify(); err != nil {
					e.Logf("verify %s: SOCKS ok but TPROXY capture missing: %v (attempt %d)", c.Name, err, attempt)
					lastDetail = "tunnel reachable via SOCKS but TPROXY capture not installed: " + err.Error()
					if !sleepCtx(ctx, 2*time.Second) {
						return false, firstNonEmpty(lastDetail, "activation attempt timed out")
					}
					continue
				}
			}
			e.Logf("verify %s: reachable (%dms, attempt %d)", c.Name, p.LatencyMs, attempt)
			return true, ""
		}
		if p.Err != nil {
			lastDetail = p.Err.Error()
		}
		if !sleepCtx(ctx, 2*time.Second) {
			return false, firstNonEmpty(lastDetail, "activation attempt timed out")
		}
	}
	return false, lastDetail
}

// waitPortReady polls a TCP port until it is accepting connections or ctx is
// cancelled. Used to avoid a false "connection refused" from a premature
// SOCKS probe right after Xray restarts.
func (e *Engine) waitPortReady(ctx context.Context, host string, port int) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// foActivateMarginS is added on top of the rollback (verify) budget to bound a
// single failover-initiated activation: enough headroom for the bring-up itself
// on top of the verify loop, while still capping a genuinely stuck attempt.
const foActivateMarginS = 45

// activateWithin runs Activate with a bounded per-attempt timeout, so one hung
// bring-up (a stuck xray-core fetch, or a server that never completes the
// verify probe) can't stall the shared health/failover goroutine. Used by every
// background (loop-driven) activation; interactive callers use Activate.
func (e *Engine) activateWithin(id string) error {
	timeout := time.Duration(e.rollbackTimeout()+foActivateMarginS) * time.Second
	ctx, cancel := context.WithTimeout(e.baseCtx(), timeout)
	defer cancel()
	return e.activate(ctx, id)
}

// sleepCtx sleeps for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// setActive persists the active connection id.
func (e *Engine) setActive(id string) error {
	return e.store.Mutate(func(s *model.State) error { s.ActiveConnID = id; return nil })
}

// buildActiveXray produces the config for the single active Xray connection.
//
// In proxy-connection mode it is the minimal SOCKS-only profile
// (Options.ProxyConnMode): the router routes to the managed ProxyN interface
// via dns-proxy, so there is no TPROXY capture and no in-Xray split — a full
// tunnel through the pinned server is correct.
//
// In TPROXY mode it carries a local SOCKS inbound (LAN proxy + probe target)
// plus a TPROXY inbound for transparent capture; outbounds carry SO_MARK 255 so
// Xray's own egress is not re-captured (route.Manager excludes that mark). When
// one or more enabled service routes target this connection, their domains and
// subnets are compiled into per-service (split-tunnel) routing so only those
// services egress through the server and the rest goes direct. excludeRouteID
// lets a teardown rebuild the config without a route that is still present in
// state (DeleteRoute/SetRouteEnabled tear down before/while mutating state).
func (e *Engine) buildActiveXray(connID string, server model.Server, excludeRouteID string) (*xray.Config, error) {
	opts := xray.Defaults()
	opts.SocksPort = xraySocksPort
	opts.EnableBalancer = false
	// Direct Xray's own error log to a known file and honour the debug toggle, so
	// a failed activation can report the tunnel's real reason (see verifyActive).
	opts.LogError = e.xray.ErrorLogPath()
	opts.LogLevel = e.xrayLogLevel()
	// Clamp the outbound MSS (fix for router-local egress on a reduced-MTU WAN).
	opts.TCPMaxSeg = e.xrayMSSClamp()
	if pt := e.probeTarget(); pt != "" {
		opts.ProbeURL = pt
		opts.PingDestination = pt
	}
	if e.xrayProxyMode() {
		opts.ProxyConnMode = true
		return xray.BuildConfig([]model.Server{server}, opts)
	}
	opts.EnableTProxy = true
	opts.TProxyPort = e.route.TProxyPort
	opts.SplitDomains, opts.SplitSubnets = e.xrayRouteMembership(connID, excludeRouteID)
	return xray.BuildConfig([]model.Server{server}, opts)
}

// xrayRouteMembership aggregates the domains/subnets of every enabled service
// route that targets the given Xray connection (skipping excludeID). An empty
// result means "no split routes" → buildActiveXray produces a full tunnel.
func (e *Engine) xrayRouteMembership(connID, excludeID string) (domains, subnets []string) {
	if connID == "" {
		return nil, nil
	}
	for _, r := range e.store.Get().Routes {
		if !r.Enabled || r.ID == excludeID || r.TargetConnID != connID {
			continue
		}
		domains = append(domains, r.Domains...)
		subnets = append(subnets, r.Subnets...)
	}
	return dedupeLower(domains), dedupe(subnets)
}

// xrayFailureReason distils Xray's own error log into one short, salient line
// explaining a failed bring-up (a dial reset, an i/o timeout, a REALITY
// mismatch), so the activation error is actionable rather than only "did not
// carry traffic". Returns "" when the log is empty. It prefers the most recent
// line matching a known failure signature and otherwise uses the last line.
func (e *Engine) xrayFailureReason() string {
	return distillXrayFailure(e.xray.LogTail(20))
}

// xrayFailureSignatures are substrings (lower-cased) that mark an Xray log line
// as an actionable failure reason worth surfacing in an activation error.
var xrayFailureSignatures = []string{
	"reality", "invalid", "rejected", "reset", "timeout", "timed out",
	"refused", "unreachable", "no route", "dial ", "handshake",
	"certificate", "context deadline", "eof", "failed to", "unable to",
}

// distillXrayFailure picks the most recent salient line from an Xray log tail
// and condenses it. Pure, so it is unit-tested without a device/log file.
func distillXrayFailure(tail string) string {
	if strings.TrimSpace(tail) == "" {
		return ""
	}
	lines := strings.Split(tail, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		low := strings.ToLower(lines[i])
		for _, s := range xrayFailureSignatures {
			if strings.Contains(low, s) {
				return condenseLogLine(lines[i])
			}
		}
	}
	return condenseLogLine(lines[len(lines)-1])
}

// condenseLogLine strips an Xray "2006/01/02 15:04:05" timestamp prefix and caps
// the length so a log line embeds cleanly in an error message.
func condenseLogLine(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 19 && s[4] == '/' && s[7] == '/' && s[10] == ' ' && s[13] == ':' {
		s = strings.TrimSpace(s[19:])
	}
	const maxLen = 240
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}

// xrayLogLevel resolves the Xray loglevel written into the generated config
// from the user setting, defaulting to "warning" and rejecting unknown values.
func (e *Engine) xrayLogLevel() string {
	return normalizeXrayLogLevel(e.store.Settings().XrayLogLevel)
}

// normalizeXrayLogLevel validates the stored Xray loglevel. Empty or unknown
// values fall back to "warning". Pure, so it is unit-tested directly.
func normalizeXrayLogLevel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug", "info", "warning", "error", "none":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "warning"
	}
}

// xrayMSSClamp resolves the effective MSS to set on Xray's server outbound from
// the user setting: 0 (or negative) → no clamp (default OFF), positive → that
// value. See model.Settings.XrayMSSClamp.
func (e *Engine) xrayMSSClamp() int {
	return normalizeXrayMSS(e.store.Settings().XrayMSSClamp)
}

// normalizeXrayMSS maps the stored clamp setting to the value handed to Xray
// (0 meaning "don't emit tcpMaxSeg"). The default is OFF: XKeen never clamps the
// MSS and the dead-tunnel bug reproduced both before and after the session-15
// clamp, so clamping is no longer applied automatically — only when the user
// sets an explicit positive MSS on the Settings page (DefaultXrayMSS is the
// suggested value). 0 (auto) and any negative both mean "no clamp". Pure, so it
// is unit-tested directly.
func normalizeXrayMSS(stored int) int {
	if stored > 0 {
		return stored
	}
	return 0
}

// probeTarget is the connectivity check URL (failover probe target, or default).
func (e *Engine) probeTarget() string {
	if pt := strings.TrimSpace(e.store.Get().Failover.ProbeTarget); pt != "" {
		return pt
	}
	return "https://www.gstatic.com/generate_204"
}

// Rollback-timeout bounds. The stored setting is a user-facing knob that may be
// left at its zero value; we interpret it explicitly rather than letting a 0
// silently mean "90s" somewhere downstream:
//   - 0 (or negative) => defaultRollbackTimeoutS — the documented default;
//   - a positive value below minRollbackTimeoutS is clamped up, so a fat-finger
//     like "3" can't trip a rollback before even one end-to-end probe finishes
//     (verifyActive runs a ~6s probe + 2s backoff per attempt).
const (
	defaultRollbackTimeoutS = 90
	minRollbackTimeoutS     = 10
)

// normalizeRollbackTimeout maps the stored rollback_timeout_s to the effective
// number of seconds verifyActive waits before giving up and rolling back. Pure
// (no engine state) so it is unit-tested directly.
func normalizeRollbackTimeout(stored int) int {
	if stored <= 0 {
		return defaultRollbackTimeoutS
	}
	if stored < minRollbackTimeoutS {
		return minRollbackTimeoutS
	}
	return stored
}

func (e *Engine) rollbackTimeout() int {
	return normalizeRollbackTimeout(e.store.Settings().RollbackTimeoutS)
}

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
//
// For the ACTIVE Xray connection, the SOCKS-based end-to-end probe is the
// authoritative signal: a TCPPing to the server's endpoint can fail even when
// the tunnel is healthy (e.g. nfqws2/ndm intercepts the router's own IPv4 TCP
// OUTPUT, or the server is reachable only via IPv6). So for the active Xray we
// probe SOCKS first, and only use TCPPing for a latency number. This prevents
// the dashboard from showing "down" on a working tunnel.
func (e *Engine) probeConnection(st model.State, c model.Connection) model.RuntimeStatus {
	rs := model.RuntimeStatus{
		ConnID:    c.ID,
		LastCheck: time.Now(),
		Active:    st.ActiveConnID == c.ID,
	}
	if !connEligible(st, c) {
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

	// Active Xray: SOCKS probe is authoritative. TCPPing may fail on the
	// router itself (ndm/nfqws2 intercepts IPv4 TCP OUTPUT) while the tunnel
	// works fine via IPv6 or an established connection.
	if c.Type == model.ConnXray && rs.Active {
		if e.verifyOnce(c) {
			rs.Status = model.StatusUp
			// Best-effort TCPPing for latency only (may fail — that's OK).
			host, port := endpointHostPort(c)
			if host != "" && port > 0 {
				ctx, cancel := context.WithTimeout(e.baseCtx(), 4*time.Second)
				p := health.TCPPing(ctx, host, port, 4*time.Second)
				cancel()
				if p.OK {
					rs.LatencyMs = p.LatencyMs
				}
			}
			return rs
		}
		rs.Status = model.StatusDegraded
		rs.Message = "tunnel probe failed (SOCKS unreachable)"
		return rs
	}

	// Non-active Xray connections: TCPPing is the only probe we can do
	// without activating them. If it fails, mark as down.
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
	default:
		rs.Status = model.StatusUp
	}
	return rs
}

// verifyOnce is a single (non-retrying) end-to-end probe of the active path.
// For Xray in TPROXY mode, a SOCKS success is followed by a TPROXY chain
// check so the health loop reports "degraded" when the capture rules are gone
// (e.g. ndm flushed them and the hook hasn't reinstalled them yet).
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
		if !health.SOCKSHTTP(ctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, 6*time.Second).OK {
			return false
		}
		// SOCKS ok — for TPROXY mode, also verify the capture chain so
		// "up" means LAN traffic actually flows through the tunnel.
		if !e.xrayProxyMode() && !e.runner.DryRun {
			return e.route.Verify() == nil
		}
		return true
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
