package engine

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// NormalizeFailoverChain cleans a proposed failover chain: it trims each entry,
// drops empties and duplicates (keeping first occurrence, preserving order),
// and partitions the result into recognised entries (a known connection ID or
// the "direct" sentinel) versus unknown ones the caller should reject. Pure, so
// CLI/API chain validation is unit-tested here without a device.
func NormalizeFailoverChain(chain, connIDs []string) (clean, unknown []string) {
	known := map[string]bool{DirectNode: true}
	for _, id := range connIDs {
		known[id] = true
	}
	seen := map[string]bool{}
	for _, raw := range chain {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	return clean, unknown
}

// DirectNode is the sentinel chain entry meaning "no tunnel" (direct / kill
// switch), always valid as the last hop of a fallback chain.
const DirectNode = "direct"

// Failover returns the current failover configuration (never nil slices).
func (e *Engine) Failover() model.Failover {
	fo := e.store.Get().Failover
	if fo.Chain == nil {
		fo.Chain = []string{}
	}
	if fo.History == nil {
		fo.History = []model.FailoverEvent{}
	}
	return fo
}

// SaveFailover persists a new failover configuration.
func (e *Engine) SaveFailover(in model.Failover) error {
	err := e.store.Mutate(func(s *model.State) error {
		hist := s.Failover.History // preserve server-side history
		s.Failover = in
		s.Failover.History = hist
		if s.Failover.CheckIntervalS <= 0 {
			s.Failover.CheckIntervalS = 30
		}
		if s.Failover.FailureThreshold <= 0 {
			s.Failover.FailureThreshold = 3
		}
		if s.Failover.Chain == nil {
			s.Failover.Chain = []string{}
		}
		// Track where the active connection sits in the chain.
		s.Failover.CurrentIndex = indexOf(s.Failover.Chain, s.ActiveConnID)
		return nil
	})
	if err != nil {
		return err
	}
	e.foResetFail()
	e.foResetBackoff()
	e.Logf("failover config saved (enabled=%v, chain=%d nodes)", in.Enabled, len(in.Chain))
	e.publishState()
	return nil
}

// SetKillSwitch toggles the leak-prevention kill switch and applies it.
func (e *Engine) SetKillSwitch(on bool) error {
	if err := e.store.Mutate(func(s *model.State) error { s.KillSwitch = on; return nil }); err != nil {
		return err
	}
	if on {
		_ = e.route.EnableKillSwitch()
	} else {
		_ = e.route.DisableKillSwitch()
	}
	e.Logf("kill switch %s", onOff(on))
	e.publishState()
	return nil
}

// ----- failover engine (called from the background loop) -----

// failoverTick evaluates the active connection and switches along the chain when
// it stays unhealthy, or returns to a higher-priority node when one recovers.
func (e *Engine) failoverTick() {
	if e.runner.DryRun {
		return
	}
	st := e.store.Get()

	// The nfqws-bypass guard runs first and independently of the chain: if we're
	// on the direct path and the DPI bypass has died, fall back to a tunnel. If
	// it switched us, stop here — the chain re-evaluates the new active next tick.
	if e.nfqwsGuardTick(st) {
		return
	}

	fo := st.Failover
	if !fo.Enabled {
		return
	}
	// There must be somewhere to fail over to: either a global chain or the
	// active connection's own per-connection fallback target.
	if len(fo.Chain) == 0 && !e.activeHasFallback(st) {
		return
	}

	if e.activeHealthy(st) {
		e.foResetFail()
		e.foResetBackoff()
		if fo.AutoReturn {
			e.maybeAutoReturn(st)
		}
		return
	}

	if n := e.foIncFail(); n < fo.FailureThreshold {
		e.Logf("failover: active unhealthy (%d/%d)", n, fo.FailureThreshold)
		return
	}
	// Past the threshold we want to switch. When the whole chain is unreachable,
	// failToNext can't switch anything — so instead of re-probing every server on
	// every tick (which hammers them and can trip server-side rate limits),
	// respect an exponential-with-jitter backoff window between attempts. The
	// window is cleared the instant we switch or the active recovers.
	if e.inBackoff(time.Now()) {
		return
	}
	if e.failToNext(st, "active connection failed the health probe") {
		e.foResetBackoff()
	} else {
		d := e.bumpBackoff()
		e.Logf("failover: no reachable node in the chain; backing off ~%s before retrying", d.Round(time.Second))
	}
}

// Failover backoff bounds. When the whole chain is down, attempts are spaced by
// base*2^(streak-1) capped at max, with jitter (see backoffDelay).
const (
	foBackoffBase = 30 * time.Second
	foBackoffMax  = 5 * time.Minute
)

// backoffDelay is the wait before the next failover attempt for a 1-based
// consecutive-failure streak: an exponential backoff (base doubling) clamped to
// max, with "full jitter" via frac in [0,1) so the result lands in [d/2, d].
// Pure (no engine state, injected randomness) so it is unit-tested directly.
func backoffDelay(streak int, base, max time.Duration, frac float64) time.Duration {
	if streak < 1 {
		streak = 1
	}
	d := base
	for i := 1; i < streak; i++ {
		d *= 2
		if d >= max {
			d = max
			break
		}
	}
	if d > max {
		d = max
	}
	if frac < 0 {
		frac = 0
	} else if frac >= 1 {
		frac = 0.999999
	}
	return d/2 + time.Duration(float64(d/2)*frac)
}

// inBackoff reports whether we are still inside the current backoff window.
func (e *Engine) inBackoff(now time.Time) bool {
	e.foMu.Lock()
	defer e.foMu.Unlock()
	return now.Before(e.foBackoffUntil)
}

// bumpBackoff grows the failure streak and arms the next backoff window,
// returning the chosen delay (for logging).
func (e *Engine) bumpBackoff() time.Duration {
	e.foMu.Lock()
	defer e.foMu.Unlock()
	e.foBackoffStreak++
	d := backoffDelay(e.foBackoffStreak, foBackoffBase, foBackoffMax, rand.Float64())
	e.foBackoffUntil = time.Now().Add(d)
	return d
}

// foResetBackoff clears the backoff window and streak (on switch/recovery).
func (e *Engine) foResetBackoff() {
	e.foMu.Lock()
	e.foBackoffStreak = 0
	e.foBackoffUntil = time.Time{}
	e.foMu.Unlock()
}

// activeHealthy reports whether the active connection currently passes an
// end-to-end probe.
func (e *Engine) activeHealthy(st model.State) bool {
	if st.ActiveConnID == "" {
		return false
	}
	c, ok := findConn(st, st.ActiveConnID)
	if !ok || !c.Enabled {
		return false
	}
	return e.verifyOnce(c)
}

// failToNext advances to the next reachable node after the active one in the
// chain (the last node may be "direct"). It returns true when it actually
// switched the active path (activated a node, dropped to direct, or used a
// per-connection fallback), and false when nothing reachable was found — the
// caller uses that to drive the outage backoff.
func (e *Engine) failToNext(st model.State, reason string) bool {
	fo := st.Failover
	start := indexOf(fo.Chain, st.ActiveConnID)
	from := st.ActiveConnID

	for i := start + 1; i < len(fo.Chain); i++ {
		node := fo.Chain[i]
		if node == DirectNode {
			e.goDirect(from, "chain exhausted — "+reason)
			e.setCurrentIndex(i)
			return true
		}
		if e.nodeReachable(node) {
			if err := e.activateWithin(node); err != nil {
				e.Logf("failover: activating %s failed: %v", node, err)
				continue
			}
			e.recordFailover(from, node, reason)
			e.setCurrentIndex(i)
			e.foResetFail()
			return true
		}
	}

	// Global chain exhausted (or empty): honour the failed connection's own
	// per-connection fallback target (VPN → other VPN → AWG → direct), so a
	// user can pin a specific safety net per connection without maintaining the
	// global chain.
	if e.failToConnFallback(from, reason) {
		return true
	}
	e.Logf("failover: no reachable node after current; leaving as-is")
	return false
}

// activeHasFallback reports whether the active connection defines a
// per-connection fallback target.
func (e *Engine) activeHasFallback(st model.State) bool {
	c, ok := findConn(st, st.ActiveConnID)
	return ok && c.FallbackTo != ""
}

// failToConnFallback attempts the per-connection fallback target of connection
// `from`. Returns true when it switched (or went direct), false when there is
// no usable per-connection fallback.
func (e *Engine) failToConnFallback(from, reason string) bool {
	if from == "" {
		return false
	}
	c, ok := findConn(e.store.Get(), from)
	if !ok || c.FallbackTo == "" || c.FallbackTo == from {
		return false
	}
	if c.FallbackTo == DirectNode {
		e.goDirect(from, "per-connection fallback — "+reason)
		e.foResetFail()
		return true
	}
	if e.nodeReachable(c.FallbackTo) {
		if err := e.activateWithin(c.FallbackTo); err == nil {
			e.recordFailover(from, c.FallbackTo, "per-connection fallback — "+reason)
			e.foResetFail()
			return true
		}
	}
	return false
}

// maybeAutoReturn switches back to the earliest healthy node in the chain when
// it outranks the currently active node.
func (e *Engine) maybeAutoReturn(st model.State) {
	fo := st.Failover
	cur := indexOf(fo.Chain, st.ActiveConnID)
	if cur <= 0 {
		return // already at (or above) the top
	}
	for i := 0; i < cur; i++ {
		node := fo.Chain[i]
		if node == DirectNode {
			continue
		}
		if e.nodeReachable(node) {
			if err := e.activateWithin(node); err != nil {
				continue
			}
			e.recordFailover(st.ActiveConnID, node, "higher-priority node recovered — auto-return")
			e.setCurrentIndex(i)
			return
		}
	}
}

// goDirect drops the tunnel: clears the active connection and, if configured,
// engages the kill switch so nothing leaks on the direct path.
func (e *Engine) goDirect(from, reason string) {
	if c, ok := findConn(e.store.Get(), from); ok {
		_ = e.bringDown(c)
	}
	_ = e.setActive("")
	if e.store.Get().Settings.KillSwitchDefault {
		_ = e.route.EnableKillSwitch()
	}
	e.recordFailover(from, DirectNode, reason)
	e.publishState()
}

// nodeReachable is a cheap TCP-ping reachability check for a chain node.
func (e *Engine) nodeReachable(connID string) bool {
	if connID == DirectNode {
		return true
	}
	c, ok := findConn(e.store.Get(), connID)
	if !ok || !c.Enabled {
		return false
	}
	host, port := endpointHostPort(c)
	// AWG endpoints are UDP; a TCP ping is meaningless, so treat an AWG node
	// that has an endpoint as eligible and let Activate's handshake verify
	// decide. Otherwise AWG fallback nodes would never be selected at all.
	if c.Type == model.ConnAWG {
		return host != ""
	}
	if host == "" || port == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 6*time.Second)
	defer cancel()
	return health.TCPPing(ctx, host, port, 6*time.Second).OK
}

func (e *Engine) recordFailover(from, to, reason string) {
	ev := model.FailoverEvent{Time: time.Now(), From: from, To: to, Reason: reason}
	_ = e.store.Mutate(func(s *model.State) error {
		s.Failover.History = append([]model.FailoverEvent{ev}, s.Failover.History...)
		if len(s.Failover.History) > 50 {
			s.Failover.History = s.Failover.History[:50]
		}
		return nil
	})
	e.Logf("failover: %s -> %s (%s)", from, to, reason)
	e.publishState()
}

func (e *Engine) setCurrentIndex(i int) {
	_ = e.store.Mutate(func(s *model.State) error { s.Failover.CurrentIndex = i; return nil })
}

// ----- nfqws-bypass guard -----

// nfqwsGuardTick implements "DPI bypass died → fall back to a tunnel". It only
// acts on the direct path (no active tunnel), where nfqws2 is the mechanism
// carrying otherwise-blocked traffic; once a tunnel is up, the bypass is no
// longer load-bearing so the guard stands down. Returns true when it switched
// the active connection (so the caller can skip the chain logic this tick).
func (e *Engine) nfqwsGuardTick(st model.State) bool {
	fo := st.Failover
	if !fo.NfqwsGuard || fo.NfqwsFallbackTo == "" {
		return false
	}
	// Only guard the direct path; a live tunnel doesn't depend on nfqws2.
	if st.ActiveConnID != "" {
		e.nfResetFail()
		return false
	}
	unhealthy, reason := e.nfqwsUnhealthy(fo.NfqwsProbeDomains)
	if !unhealthy {
		e.nfResetFail()
		return false
	}
	threshold := fo.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}
	if n := e.nfIncFail(); n < threshold {
		e.Logf("nfqws guard: bypass unhealthy (%s) (%d/%d)", reason, n, threshold)
		return false
	}

	target := fo.NfqwsFallbackTo
	if target == DirectNode {
		// We're already direct; "falling back" to direct can't restore blocked
		// sites, so there is nothing useful to do.
		e.nfResetFail()
		return false
	}
	if !e.nodeReachable(target) {
		e.Logf("nfqws guard: fallback %s not reachable", target)
		return false
	}
	if err := e.activateWithin(target); err != nil {
		e.Logf("nfqws guard: activating %s failed: %v", target, err)
		return false
	}
	e.recordFailover(DirectNode, target, "nfqws bypass dead — "+reason)
	e.nfResetFail()
	return true
}

// nfqwsUnhealthy reports whether the nfqws2 DPI bypass is effectively dead, with
// a human reason. It is only meaningful when nfqws2 is installed (nothing to
// guard otherwise). Signals, in order: daemon not running, required NFQUEUE
// kernel modules missing, and — when probe domains are configured — every one
// of them failing on the direct path (a positive sign the strategy stopped
// working even though the daemon looks up). Any single probe domain reachable
// means the bypass is working, so the probe never false-positives on an
// unrelated single-site outage.
func (e *Engine) nfqwsUnhealthy(probeDomains []string) (bool, string) {
	if !e.nfqws.Installed() {
		return false, ""
	}
	if bad, reason := nfqwsDaemonUnhealthy(e.nfqws.Running(), e.nfqwsKernelReady()); bad {
		return true, reason
	}
	domains := cleanProbeDomains(probeDomains)
	if len(domains) == 0 {
		return false, ""
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
	defer cancel()
	for _, d := range domains {
		if health.DirectHTTP(ctx, "https://"+d+"/", 6*time.Second).OK {
			return false, "" // at least one should-bypass domain is reachable
		}
	}
	return true, fmt.Sprintf("probe of %d should-bypass domain(s) failed on the direct path", len(domains))
}

// nfqwsKernelReady is a thin wrapper so nfqwsUnhealthy reads cleanly.
func (e *Engine) nfqwsKernelReady() bool {
	ready, _ := e.nfqws.KernelModulesStatus()
	return ready
}

// nfqwsDaemonUnhealthy is the pure daemon/kernel decision, split out so it can be
// unit-tested without a device.
func nfqwsDaemonUnhealthy(running, kernelReady bool) (bool, string) {
	if !running {
		return true, "daemon not running"
	}
	if !kernelReady {
		return true, "NFQUEUE kernel modules missing"
	}
	return false, ""
}

// cleanProbeDomains normalises and dedups the configured probe domains.
func cleanProbeDomains(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = sanitizeDomain(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func (e *Engine) nfIncFail() int {
	e.foMu.Lock()
	defer e.foMu.Unlock()
	e.nfFail++
	return e.nfFail
}

func (e *Engine) nfResetFail() {
	e.foMu.Lock()
	e.nfFail = 0
	e.foMu.Unlock()
}

// ----- fail counter -----

func (e *Engine) foIncFail() int {
	e.foMu.Lock()
	defer e.foMu.Unlock()
	e.foFail++
	return e.foFail
}

func (e *Engine) foResetFail() {
	e.foMu.Lock()
	e.foFail = 0
	e.foMu.Unlock()
}

// ----- small helpers -----

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

func onOff(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
