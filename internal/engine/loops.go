package engine

import (
	"context"
	"sync"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/updater"
)

// startLoops launches the background workers. They exit when the engine context
// is cancelled (Stop).
func (e *Engine) startLoops() {
	e.wg.Add(4)
	go e.healthFailoverLoop()
	go e.autoSelectLoop()
	go e.subRefreshLoop()
	go e.autoUpdateLoop()
	e.Logf("background loops started")
}

// healthFailoverLoop probes every connection and runs the failover engine on the
// configured interval.
func (e *Engine) healthFailoverLoop() {
	defer e.wg.Done()
	// Give the daemon a moment to settle, then probe once immediately.
	select {
	case <-e.ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}
	e.probeAll()

	for {
		interval := time.Duration(max(e.store.Get().Failover.CheckIntervalS, 10)) * time.Second
		select {
		case <-e.ctx.Done():
			return
		case <-time.After(interval):
			e.probeAll()
			e.failoverTick()
		}
	}
}

// autoSelectLoop periodically re-ranks auto-best subscriptions and, when the
// active connection belongs to one, migrates to a meaningfully faster server.
func (e *Engine) autoSelectLoop() {
	defer e.wg.Done()
	for {
		mins := e.store.Get().Settings.AutoSelectIntervalMin
		wait := 60 * time.Second
		if mins > 0 {
			wait = time.Duration(mins) * time.Minute
		}
		select {
		case <-e.ctx.Done():
			return
		case <-time.After(wait):
			if mins > 0 {
				e.autoSelectTick()
			}
		}
	}
}

// subRefreshLoop refreshes subscriptions whose update interval has elapsed.
func (e *Engine) subRefreshLoop() {
	defer e.wg.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-time.After(30 * time.Minute):
			e.refreshDueSubscriptions()
		}
	}
}

// probeAll refreshes runtime status for every connection, with bounded
// concurrency so a router with a large subscription isn't overwhelmed.
func (e *Engine) probeAll() {
	st := e.store.Get()
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, c := range st.Connections {
		if !connEligible(st, c) {
			// Individually disabled, or its subscription stream is off.
			e.setRuntime(c.ID, model.RuntimeStatus{ConnID: c.ID, Status: model.StatusDisabled, LastCheck: time.Now()})
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c model.Connection) {
			defer wg.Done()
			defer func() { <-sem }()
			e.setRuntime(c.ID, e.probeConnection(st, c))
		}(c)
	}
	wg.Wait()
	e.publishState()
}

// autoSelectTick migrates the active connection to a faster server within its
// auto-best subscription, with hysteresis to avoid flapping.
func (e *Engine) autoSelectTick() {
	if e.runner.DryRun {
		return
	}
	st := e.store.Get()
	if st.TunnelPaused {
		return // connector master switch off — don't migrate/activate anything
	}
	active, ok := findConn(st, st.ActiveConnID)
	if !ok || active.SubscriptionID == "" {
		return
	}
	sub, ok := findSub(st, active.SubscriptionID)
	if !ok || !sub.AutoSelectBest || !sub.Enabled {
		return // no auto-select from a disabled subscription stream
	}

	// Only ENABLED members are candidates — a server the user switched off (e.g.
	// a home-country node) is excluded, so auto-best never migrates onto it just
	// because its latency is lowest.
	members := subMembers(st, sub.ID)
	best := e.fastest(members)
	if best == "" || best == active.ID {
		return
	}

	bestRS, _ := e.runtimeFor(best)
	activeRS, _ := e.runtimeFor(active.ID)
	// Switch if the active is unhealthy, or the best is >40% faster.
	switchNow := activeRS.Status != model.StatusUp ||
		(bestRS.LatencyMs > 0 && activeRS.LatencyMs > 0 && bestRS.LatencyMs*100 < activeRS.LatencyMs*60)
	if !switchNow {
		return
	}
	e.Logf("auto-select: migrating to faster server in %s", sub.Name)
	if err := e.activateWithin(best); err != nil {
		e.Logf("auto-select activate failed: %v", err)
		return
	}
	e.recordFailover(active.ID, best, "auto-select best location")
}

// refreshDueSubscriptions refreshes any subscription whose update interval passed.
func (e *Engine) refreshDueSubscriptions() {
	if e.runner.DryRun {
		return
	}
	for _, s := range e.store.Get().Subscriptions {
		if s.UpdateInterval <= 0 || s.LastUpdate == nil {
			continue
		}
		if time.Since(*s.LastUpdate) >= time.Duration(s.UpdateInterval)*time.Hour {
			if _, err := e.RefreshSubscription(s.ID); err != nil {
				e.Logf("auto-refresh %s failed: %v", s.Name, err)
			}
		}
	}
}

// autoUpdateLoop periodically checks GitHub for a newer keen-manager release.
// When a newer version is found AND no VPN connection is active (to avoid
// interrupting a tunnel), it downloads, verifies, atomically replaces the
// binary, and restarts the service. Controlled by Settings.AutoUpdateIntervalHours
// (0 = off, 24 = once a day). The first check happens 5 minutes after startup
// (not immediately, so a fresh boot doesn't race with other init).
func (e *Engine) autoUpdateLoop() {
	defer e.wg.Done()
	// Initial delay: 5 minutes after daemon start.
	select {
	case <-e.ctx.Done():
		return
	case <-time.After(5 * time.Minute):
	}
	for {
		hours := e.store.Get().Settings.AutoUpdateIntervalHours
		if hours <= 0 {
			// Disabled — sleep an hour and re-check the setting (so the
			// user can enable it from the UI without restarting the daemon).
			select {
			case <-e.ctx.Done():
				return
			case <-time.After(1 * time.Hour):
				continue
			}
		}
		e.autoUpdateTick()
		wait := time.Duration(hours) * time.Hour
		select {
		case <-e.ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// autoUpdateTick does one check-and-update cycle. It skips the update when a
// VPN connection is active (to avoid interrupting a tunnel mid-session) — the
// next tick will retry.
func (e *Engine) autoUpdateTick() {
	if e.runner.DryRun {
		return
	}
	// Don't update while a tunnel is active — the restart would interrupt it.
	st := e.store.Get()
	if st.ActiveConnID != "" && !st.TunnelPaused {
		e.Logf("auto-update: skipping (VPN connection active)")
		return
	}
	e.Logf("auto-update: checking for a newer release...")
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Minute)
	defer cancel()
	msg, err := updater.SelfUpdate(ctx, false)
	if err != nil {
		e.Logf("auto-update: %v", err)
		return
	}
	e.Logf("auto-update: %s", msg)
}
