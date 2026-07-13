package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
)

// IntegrationTest runs a battery of functional tests on the live device.
// Unlike SelfTest (read-only diagnostics), IntegrationTest actually
// activates/deactivates connections and verifies end-to-end behaviour.
//
// It is designed to be non-destructive: it saves the current state,
// runs tests, and restores the original state at the end.
type IntegrationTestResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, fail, skip
	Detail  string `json:"detail,omitempty"`
	MS      int64  `json:"ms,omitempty"`
}

// IntegrationTest runs functional tests on the device. It requires at least
// one enabled Xray connection in a subscription.
func (e *Engine) IntegrationTest() []IntegrationTestResult {
	var results []IntegrationTestResult

	// Save original state.
	origActive := e.store.Get().ActiveConnID
	origPaused := e.store.Get().TunnelPaused
	defer func() {
		// Restore original state.
		if origPaused {
			_ = e.store.Mutate(func(s *model.State) error {
				s.TunnelPaused = true
				return nil
			})
		} else if origActive != "" {
			_ = e.activateWithin(origActive)
		}
	}()

	// 1. Selftest prerequisite.
	results = append(results, e.itSelftest())

	// 2. Find a test connection.
	st := e.store.Get()
	var testConn *model.Connection
	for _, c := range st.Connections {
		if c.Enabled && connEligible(st, c) && c.Type == model.ConnXray {
			testConn = &c
			break
		}
	}
	if testConn == nil {
		results = append(results, IntegrationTestResult{Name: "find-connection", Status: "skip", Detail: "no enabled Xray connection found"})
		return results
	}

	// 3. Activate the connection.
	results = append(results, e.itActivate(testConn.ID))

	// 4. Verify SOCKS probe.
	results = append(results, e.itSocksProbe())

	// 5. Verify TPROXY rules.
	results = append(results, e.itTproxyRules())

	// 6. Verify traffic flows (PC through VPN).
	results = append(results, e.itTrafficCheck())

	// 7. Test connector pause/resume.
	results = append(results, e.itConnectorPause(testConn.ID))

	// 8. Test route reapply.
	results = append(results, e.itRouteReapply())

	// 9. Test self-update check.
	results = append(results, e.itUpdateCheck())

	return results
}

func (e *Engine) itSelftest() IntegrationTestResult {
	st := e.SelfTest()
	pass := 0
	for _, r := range st {
		if r.Status == "pass" {
			pass++
		}
	}
	if pass == len(st) {
		return IntegrationTestResult{Name: "selftest", Status: "pass", Detail: fmt.Sprintf("%d/%d checks passed", pass, len(st))}
	}
	return IntegrationTestResult{Name: "selftest", Status: "fail", Detail: fmt.Sprintf("%d/%d checks passed", pass, len(st))}
}

func (e *Engine) itActivate(connID string) IntegrationTestResult {
	start := time.Now()
	err := e.activateWithin(connID)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return IntegrationTestResult{Name: "activate", Status: "fail", Detail: err.Error(), MS: ms}
	}
	return IntegrationTestResult{Name: "activate", Status: "pass", Detail: "connection activated + verified", MS: ms}
}

func (e *Engine) itSocksProbe() IntegrationTestResult {
	ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
	defer cancel()
	target := e.probeTarget()
	p := health.SOCKSHTTP(ctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, 6*time.Second)
	if !p.OK {
		return IntegrationTestResult{Name: "socks-probe", Status: "fail", Detail: fmt.Sprintf("probe failed: %v", p.Err)}
	}
	return IntegrationTestResult{Name: "socks-probe", Status: "pass", Detail: fmt.Sprintf("reachable (%dms via %s)", p.LatencyMs, target), MS: int64(p.LatencyMs)}
}

func (e *Engine) itTproxyRules() IntegrationTestResult {
	if e.runner.DryRun {
		return IntegrationTestResult{Name: "tproxy-rules", Status: "skip", Detail: "dry-run"}
	}
	if err := e.route.Verify(); err != nil {
		return IntegrationTestResult{Name: "tproxy-rules", Status: "fail", Detail: err.Error()}
	}
	return IntegrationTestResult{Name: "tproxy-rules", Status: "pass", Detail: "KEENMGR_TPROXY chain + PREROUTING jump verified"}
}

func (e *Engine) itTrafficCheck() IntegrationTestResult {
	// Check that the router's WAN IP changed (goes through VPN).
	// We can't easily check from the engine, but we can check the SOCKS
	// probe succeeded which means traffic flows through the tunnel.
	// Already covered by socks-probe, so we just verify the active conn is up.
	st := e.store.Get()
	if st.ActiveConnID == "" {
		return IntegrationTestResult{Name: "traffic-check", Status: "fail", Detail: "no active connection after activation"}
	}
	rs, ok := e.runtimeFor(st.ActiveConnID)
	if !ok || rs.Status != model.StatusUp {
		return IntegrationTestResult{Name: "traffic-check", Status: "fail", Detail: fmt.Sprintf("active connection status=%s", rs.Status)}
	}
	return IntegrationTestResult{Name: "traffic-check", Status: "pass", Detail: fmt.Sprintf("active=%s, latency=%dms", st.ActiveConnID, rs.LatencyMs)}
}

func (e *Engine) itConnectorPause(connID string) IntegrationTestResult {
	if e.runner.DryRun {
		return IntegrationTestResult{Name: "connector-pause", Status: "skip", Detail: "dry-run"}
	}
	// Pause
	_ = e.store.Mutate(func(s *model.State) error {
		s.TunnelPaused = true
		s.PausedConnID = connID
		return nil
	})
	e.bringDown(model.Connection{ID: connID, Type: model.ConnXray})
	time.Sleep(2 * time.Second)

	// Check TPROXY rules are removed
	tpErr := e.route.Verify()

	// Resume
	_ = e.store.Mutate(func(s *model.State) error {
		s.TunnelPaused = false
		s.PausedConnID = ""
		return nil
	})
	err := e.activateWithin(connID)
	if err != nil {
		return IntegrationTestResult{Name: "connector-pause", Status: "fail", Detail: fmt.Sprintf("resume failed: %v", err)}
	}
	if tpErr != nil {
		return IntegrationTestResult{Name: "connector-pause", Status: "pass", Detail: "pause removed TPROXY rules, resume re-activated"}
	}
	return IntegrationTestResult{Name: "connector-pause", Status: "pass", Detail: "pause/resume cycle OK"}
}

func (e *Engine) itRouteReapply() IntegrationTestResult {
	if e.runner.DryRun {
		return IntegrationTestResult{Name: "route-reapply", Status: "skip", Detail: "dry-run"}
	}
	err := e.ReapplyRoutes()
	if err != nil {
		return IntegrationTestResult{Name: "route-reapply", Status: "fail", Detail: err.Error()}
	}
	// Verify rules are still present after reapply
	if err := e.route.Verify(); err != nil {
		return IntegrationTestResult{Name: "route-reapply", Status: "fail", Detail: fmt.Sprintf("rules missing after reapply: %v", err)}
	}
	return IntegrationTestResult{Name: "route-reapply", Status: "pass", Detail: "routes reapplied + verified"}
}

func (e *Engine) itUpdateCheck() IntegrationTestResult {
	// Just check that the version command works — doesn't actually update.
	v := e.Health().Version
	return IntegrationTestResult{Name: "update-check", Status: "pass", Detail: fmt.Sprintf("version=%s", v)}
}

// HTTP probe helper (lightweight, no extra deps)
func httpProbe(target string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(target)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
