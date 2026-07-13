package route

import (
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// capture returns a dry-run Runner that records every command it is asked to
// run, so the emitted iptables/ip ruleset can be asserted off-device.
func capture() (*platform.Runner, *[]string) {
	var cmds []string
	r := &platform.Runner{DryRun: true}
	r.Log = func(c string) { cmds = append(cmds, c) }
	return r, &cmds
}

func testManager(r *platform.Runner) *Manager {
	return &Manager{
		Runner:     r,
		IPBin:      "ip",
		IPTables:   "iptables",
		TProxyPort: 12345,
		Bypass:     []string{"127.0.0.0/8", "192.168.0.0/16"},
		SelfMark:   255,
	}
}

func TestEnableTProxyEmitsCanonicalRules(t *testing.T) {
	r, cmds := capture()
	m := testManager(r)
	if err := m.EnableTProxy(); err != nil {
		t.Fatalf("EnableTProxy: %v", err)
	}
	all := strings.Join(*cmds, "\n")

	// Every iptables call must carry the xtables lock-wait flag.
	for _, line := range *cmds {
		if strings.HasPrefix(line, "iptables ") && !strings.Contains(line, "-w 5") {
			t.Errorf("iptables call without -w 5: %s", line)
		}
	}

	// XKeen-canonical TPROXY rule fragments that must be present.
	for _, w := range []string{
		"-m conntrack --ctstate DNAT,INVALID -j RETURN",
		"-m socket --transparent -j MARK --set-mark 0x2333/0x2333",
		"-m socket --transparent -j ACCEPT",
		"--mark 255 -j RETURN",
		"-j TPROXY --on-ip 127.0.0.1 --on-port 12345 --tproxy-mark 0x2333/0x2333",
	} {
		if !strings.Contains(all, w) {
			t.Errorf("EnableTProxy missing canonical fragment %q\n--- emitted ---\n%s", w, all)
		}
	}

	// Policy route: masked fwmark + local default route into table 993.
	if !strings.Contains(all, "ip rule add fwmark 0x2333/0x2333 lookup 993") {
		t.Errorf("expected masked fwmark policy rule; emitted:\n%s", all)
	}
	if !strings.Contains(all, "ip route replace local default dev lo table 993") {
		t.Errorf("expected local default route in table 993; emitted:\n%s", all)
	}
}

// TestSelfEgressReturnsBeforeTProxy makes sure the self-mark RETURN (and the
// bypass RETURNs) precede the TPROXY target, so Xray's own egress and the LAN
// are never captured.
func TestSelfEgressReturnsBeforeTProxy(t *testing.T) {
	r, cmds := capture()
	_ = testManager(r).EnableTProxy()
	all := strings.Join(*cmds, "\n")
	selfIdx := strings.Index(all, "--mark 255 -j RETURN")
	bypassIdx := strings.Index(all, "-d 192.168.0.0/16 -j RETURN")
	tpIdx := strings.Index(all, "-j TPROXY")
	if selfIdx < 0 || bypassIdx < 0 || tpIdx < 0 {
		t.Fatalf("missing rules (self=%d bypass=%d tproxy=%d)", selfIdx, bypassIdx, tpIdx)
	}
	if selfIdx > tpIdx || bypassIdx > tpIdx {
		t.Errorf("RETURN rules must precede TPROXY (self=%d bypass=%d tproxy=%d)", selfIdx, bypassIdx, tpIdx)
	}
}

// TestDisableTProxyTearsDown checks teardown removes the jump/chain and the
// policy rule (both mask forms) so an upgrade can't leak a stale ip rule.
func TestDisableTProxyTearsDown(t *testing.T) {
	r, cmds := capture()
	_ = testManager(r).DisableTProxy()
	all := strings.Join(*cmds, "\n")
	for _, w := range []string{
		"iptables -w 5 -t mangle -D PREROUTING -j KEENMGR_TPROXY",
		"iptables -w 5 -t mangle -X KEENMGR_TPROXY",
		"ip rule del fwmark 0x2333/0x2333 lookup 993",
		"ip rule del fwmark 0x2333 lookup 993",
		"ip route flush table 993",
	} {
		if !strings.Contains(all, w) {
			t.Errorf("DisableTProxy missing %q\n--- emitted ---\n%s", w, all)
		}
	}
}
