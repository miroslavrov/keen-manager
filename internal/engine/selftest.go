package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/version"
)

// SelfTestResult is one check in a selftest report.
type SelfTestResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, fail, warn, skip
	Detail string `json:"detail,omitempty"`
}

// SelfTest runs a battery of on-device checks and returns the results.
// Safe to run at any time — all checks are read-only (no mutations).
func (e *Engine) SelfTest() []SelfTestResult {
	var results []SelfTestResult

	// 1. Version + arch
	results = append(results, checkVersion(e))

	// 2. Xray binary
	results = append(results, e.checkXrayBinary())

	// 3. Xray config valid
	results = append(results, e.checkXrayConfig())

	// 4. SOCKS port reachable
	results = append(results, e.checkSocksPort())

	// 5. SOCKS end-to-end probe
	results = append(results, e.checkSocksProbe())

	// 6. TPROXY rules
	results = append(results, e.checkTproxyRules())

	// 7. ndm hook installed
	results = append(results, e.checkNdmHook())

	// 8. nfqws2
	results = append(results, e.checkNfqws())

	// 9. ip rule + route table
	results = append(results, e.checkPolicyRoute())

	// 10. ip-full binary
	results = append(results, e.checkIpBinary())

	return results
}

func checkVersion(e *Engine) SelfTestResult {
	v := version.Short()
	p := e.Platform
	return SelfTestResult{
		Name:   "version",
		Status: "pass",
		Detail: fmt.Sprintf("%s, arch=%s, os=%s", v, p.Arch, p.OSVersion),
	}
}

func (e *Engine) checkXrayBinary() SelfTestResult {
	if !e.xray.Installed() {
		return SelfTestResult{Name: "xray-binary", Status: "fail", Detail: "xray not installed (will auto-download on first activation)"}
	}
	if reason := e.xray.UnusableReason(); reason != "" {
		return SelfTestResult{Name: "xray-binary", Status: "fail", Detail: reason}
	}
	return SelfTestResult{Name: "xray-binary", Status: "pass", Detail: "installed and runnable"}
}

func (e *Engine) checkXrayConfig() SelfTestResult {
	cfgPath := e.xray.ConfigPath()
	if _, err := os.Stat(cfgPath); err != nil {
		return SelfTestResult{Name: "xray-config", Status: "warn", Detail: "no config (no connection activated yet)"}
	}
	if err := e.xray.Validate(cfgPath); err != nil {
		return SelfTestResult{Name: "xray-config", Status: "fail", Detail: fmt.Sprintf("invalid: %v", err)}
	}
	return SelfTestResult{Name: "xray-config", Status: "pass", Detail: "valid"}
}

func (e *Engine) checkSocksPort() SelfTestResult {
	addr := net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return SelfTestResult{Name: "socks-port", Status: "fail", Detail: fmt.Sprintf("127.0.0.1:%d not listening: %v", xraySocksPort, err)}
	}
	conn.Close()
	return SelfTestResult{Name: "socks-port", Status: "pass", Detail: fmt.Sprintf("listening on %s", addr)}
}

func (e *Engine) checkSocksProbe() SelfTestResult {
	st := e.store.Get()
	if st.ActiveConnID == "" {
		return SelfTestResult{Name: "socks-probe", Status: "skip", Detail: "no active connection"}
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
	defer cancel()
	target := e.probeTarget()
	p := health.SOCKSHTTP(ctx, net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), target, 6*time.Second)
	if p.OK {
		return SelfTestResult{Name: "socks-probe", Status: "pass", Detail: fmt.Sprintf("reachable (%dms via %s)", p.LatencyMs, target)}
	}
	return SelfTestResult{Name: "socks-probe", Status: "fail", Detail: fmt.Sprintf("probe failed: %v", p.Err)}
}

func (e *Engine) checkTproxyRules() SelfTestResult {
	if e.runner.DryRun {
		return SelfTestResult{Name: "tproxy-rules", Status: "skip", Detail: "dry-run"}
	}
	if err := e.route.Verify(); err != nil {
		return SelfTestResult{Name: "tproxy-rules", Status: "fail", Detail: err.Error()}
	}
	return SelfTestResult{Name: "tproxy-rules", Status: "pass", Detail: "KEENMGR_TPROXY chain installed + PREROUTING jump active"}
}

func (e *Engine) checkNdmHook() SelfTestResult {
	hookPath := e.Paths.NdmDir + "/netfilter.d/50-keen-manager"
	if !platform.FileExists(hookPath) {
		return SelfTestResult{Name: "ndm-hook", Status: "warn", Detail: "not installed (run: keen-manager install-hook)"}
	}
	return SelfTestResult{Name: "ndm-hook", Status: "pass", Detail: "installed"}
}

func (e *Engine) checkNfqws() SelfTestResult {
	nf := e.nfqws.Status()
	if !nf.Installed {
		return SelfTestResult{Name: "nfqws2", Status: "skip", Detail: "not installed"}
	}
	if !nf.Running {
		return SelfTestResult{Name: "nfqws2", Status: "warn", Detail: "installed but not running"}
	}
	return SelfTestResult{Name: "nfqws2", Status: "pass", Detail: fmt.Sprintf("running, v%s, mode=%s, healthy=%v", nf.Version, nf.Mode, nf.Healthy)}
}

func (e *Engine) checkPolicyRoute() SelfTestResult {
	if e.runner.DryRun {
		return SelfTestResult{Name: "policy-route", Status: "skip", Detail: "dry-run"}
	}
	// Check ip rule for fwmark 0x2333
	res := e.runner.Run(e.route.IPBin, "rule", "show")
	if res.Err != nil {
		return SelfTestResult{Name: "policy-route", Status: "fail", Detail: fmt.Sprintf("ip rule show failed: %v", res.Err)}
	}
	if !strings.Contains(res.Stdout, "0x2333") {
		st := e.store.Get()
		if st.ActiveConnID == "" {
			return SelfTestResult{Name: "policy-route", Status: "skip", Detail: "no active connection (no policy route expected)"}
		}
		return SelfTestResult{Name: "policy-route", Status: "fail", Detail: "fwmark 0x2333 rule not found"}
	}
	return SelfTestResult{Name: "policy-route", Status: "pass", Detail: "fwmark 0x2333/0x2333 → table 993 present"}
}

func (e *Engine) checkIpBinary() SelfTestResult {
	if platform.IPBinBroken(e.Paths) {
		return SelfTestResult{Name: "ip-binary", Status: "warn", Detail: "/opt/sbin/ip is wrong-arch/corrupt — using firmware fallback. Run: opkg install ip-full"}
	}
	return SelfTestResult{Name: "ip-binary", Status: "pass", Detail: "ok"}
}
