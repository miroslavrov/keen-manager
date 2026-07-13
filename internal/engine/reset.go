package engine

import (
	"fmt"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// ResetAll performs a factory reset. It tears down every device-side artifact
// keen-manager created — per-service routes (object-groups + dns-proxy), the
// active tunnel and its TPROXY capture, the kill-switch, and the managed Xray
// and DPI-bypass Proxy interfaces — stops the managed Xray and tpws processes,
// then wipes the persisted state document and the secret vault back to
// first-run defaults. The previous state.json is snapshotted into the backup
// directory first (see config.Store.Reset), so a reset stays recoverable.
//
// The teardown is deliberately best-effort: a router that rejects one RCI call
// must not stop the configuration from being cleared. All device-mutating work
// is a no-op off-device / in dry-run. It mirrors the documented manual recovery
// (stop → rm -rf /opt/etc/keen-manager → start) without disturbing components
// keen-manager does not own (the nfqws2 binary, its config, or its lists).
func (e *Engine) ResetAll() error {
	st := e.store.Get()

	// 1) Remove per-service routes from the router first, so no dns-proxy route
	//    is left pointing at an interface we are about to delete.
	for _, r := range st.Routes {
		_ = e.unapplyRoute(r)
	}

	// 2) Bring the active tunnel down and drop its capture + kill-switch rules
	//    so LAN traffic returns to the direct WAN path instead of a dead proxy.
	if st.ActiveConnID != "" {
		if c, ok := findConn(st, st.ActiveConnID); ok {
			e.revertRouting(c)
			_ = e.bringDown(c)
		}
	}
	_ = e.route.DisableTProxy()
	_ = e.route.DisableKillSwitch()

	// 3) Stop the managed services and retire the shared exit interfaces.
	_ = e.xray.Stop()
	_ = e.tpws.Stop()
	_ = e.tpws.RemoveInitScript()
	e.teardownManagedBypassIface()
	e.teardownManagedProxyIface()

	// 4) Wipe the persisted config and the secret vault back to defaults.
	if err := e.store.Reset(); err != nil {
		return fmt.Errorf("reset state: %w", err)
	}
	if err := e.vault.reset(); err != nil {
		return fmt.Errorf("reset vault: %w", err)
	}

	// 5) Clear volatile in-memory state so nothing survives the wipe: per-conn
	//    health, auth sessions, the proxy/bypass "down" latches, and the
	//    failover backoff counters.
	e.mu.Lock()
	e.runtime = map[string]*model.RuntimeStatus{}
	e.proxyClientDown = false
	e.bypassClientDown = false
	e.mu.Unlock()

	e.sessMu.Lock()
	e.sessions = map[string]time.Time{}
	e.sessMu.Unlock()

	e.foMu.Lock()
	e.foFail, e.nfFail, e.foBackoffStreak = 0, 0, 0
	e.foBackoffUntil = time.Time{}
	e.foMu.Unlock()

	e.Logf("all settings reset to defaults (connections, subscriptions, routes, failover, DPI bypass and stored credentials cleared)")
	e.publishState()
	return nil
}
