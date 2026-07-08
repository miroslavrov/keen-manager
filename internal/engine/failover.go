package engine

import (
	"context"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
)

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
	fo := st.Failover
	if !fo.Enabled || len(fo.Chain) == 0 {
		return
	}

	if e.activeHealthy(st) {
		e.foResetFail()
		if fo.AutoReturn {
			e.maybeAutoReturn(st)
		}
		return
	}

	if n := e.foIncFail(); n < fo.FailureThreshold {
		e.Logf("failover: active unhealthy (%d/%d)", n, fo.FailureThreshold)
		return
	}
	e.failToNext(st, "active connection failed the health probe")
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
// chain (the last node may be "direct").
func (e *Engine) failToNext(st model.State, reason string) {
	fo := st.Failover
	start := indexOf(fo.Chain, st.ActiveConnID)
	from := st.ActiveConnID

	for i := start + 1; i < len(fo.Chain); i++ {
		node := fo.Chain[i]
		if node == DirectNode {
			e.goDirect(from, "chain exhausted — "+reason)
			e.setCurrentIndex(i)
			return
		}
		if e.nodeReachable(node) {
			if err := e.Activate(node); err != nil {
				e.Logf("failover: activating %s failed: %v", node, err)
				continue
			}
			e.recordFailover(from, node, reason)
			e.setCurrentIndex(i)
			e.foResetFail()
			return
		}
	}
	e.Logf("failover: no reachable node after current; leaving as-is")
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
			if err := e.Activate(node); err != nil {
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
