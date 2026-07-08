// Package route manages the (optional) transparent-proxy and kill-switch
// firewall/routing rules that make LAN traffic flow through the active Xray
// tunnel.
//
// SAFETY MODEL — this is the only part of keen-manager that touches the
// device's routing/firewall, so it is deliberately conservative:
//
//   - Every mutation goes through platform.Runner, which is dry-run aware, so it
//     is inert in tests and off-device.
//   - It uses a dedicated routing table and fwmark that sit OUTSIDE the ranges
//     KeeneticOS reserves for its own policies, and it only ever adds/removes
//     its own rules — it never flushes a built-in chain.
//   - Because KeeneticOS re-creates iptables on every topology change, the
//     installer registers an ndm netfilter.d hook that calls
//     `keen-manager route reapply`; HookScript returns that hook's contents.
//   - It is DISABLED by default. keen-manager is fully functional (config
//     generation, health, failover, nfqws, local SOCKS proxy) without it.
package route

import (
	"fmt"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

const (
	// Table and Mark are chosen to avoid KeeneticOS's reserved ranges.
	Table = 993
	Mark  = 0x2333

	// ChainMangle is our private mangle chain; we jump to it from PREROUTING and
	// OUTPUT so removal is a single flush+delete of a chain we own.
	ChainPre = "KEENMGR_TPROXY"
	ChainKS  = "KEENMGR_KILL"
)

// DefaultBypass are destination ranges that must never be sent through the
// tunnel (RFC1918 + loopback + link-local + CGNAT + multicast), so the router,
// LAN and management stay reachable even with a broken tunnel.
var DefaultBypass = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16",
	"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
}

// Manager applies transparent-proxy (TPROXY) and kill-switch rules.
type Manager struct {
	Runner     *platform.Runner
	IPBin      string
	IPTables   string
	TProxyPort int
	Bypass     []string
	SelfMark   int // SO_MARK Xray sets on its own egress (excluded from capture)
}

// New returns a Manager with sane defaults for the given platform paths.
func New(p platform.Paths, r *platform.Runner) *Manager {
	ipt := "iptables"
	if platform.FileExists(p.Root + "/sbin/iptables") {
		ipt = p.Root + "/sbin/iptables"
	}
	ip := "ip"
	if platform.FileExists(p.IPBin) {
		ip = p.IPBin
	}
	return &Manager{
		Runner:     r,
		IPBin:      ip,
		IPTables:   ipt,
		TProxyPort: 12345,
		Bypass:     append([]string(nil), DefaultBypass...),
		SelfMark:   255,
	}
}

// EnableTProxy installs the policy route + TPROXY capture rules.
func (m *Manager) EnableTProxy() error {
	if err := m.installPolicyRoute(); err != nil {
		return err
	}
	return m.applyChain(ChainPre, m.tproxyRules(), true)
}

// DisableTProxy removes the TPROXY capture rules and policy route.
func (m *Manager) DisableTProxy() error {
	_ = m.applyChain(ChainPre, m.tproxyRules(), false)
	return m.removePolicyRoute()
}

// Reapply re-installs the rules (called from the ndm hook after KeeneticOS
// flushes iptables on a topology change). Safe to call repeatedly.
func (m *Manager) Reapply() error {
	// Remove-then-add makes this idempotent.
	_ = m.applyChain(ChainPre, m.tproxyRules(), false)
	return m.EnableTProxy()
}

// EnableKillSwitch drops forwarded traffic that is not destined for the tunnel
// or an explicitly bypassed range, preventing leaks when every tunnel is down.
func (m *Manager) EnableKillSwitch() error {
	return m.applyChain(ChainKS, m.killSwitchRules(), true)
}

// DisableKillSwitch removes the kill-switch rules.
func (m *Manager) DisableKillSwitch() error {
	return m.applyChain(ChainKS, m.killSwitchRules(), false)
}

func (m *Manager) installPolicyRoute() error {
	mark := fmt.Sprintf("0x%x", Mark)
	table := fmt.Sprint(Table)

	// `ip rule add` ALWAYS appends, even an identical rule — so a naive add on
	// every Reapply() (the ndm hook fires on each topology change) would leak
	// duplicate rules indefinitely. Delete any existing copy first, then add
	// exactly one. The del is best-effort (errors when absent, which is fine).
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", mark, "lookup", table)
	if err := m.Runner.MustRun(m.IPBin, "rule", "add", "fwmark", mark, "lookup", table); err != nil {
		// Tolerate "exists" in case a concurrent add already installed it.
		if !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("ip rule add: %w", err)
		}
	}

	// `ip route replace` is idempotent (adds or updates), so it is safe to call
	// repeatedly without accumulating state.
	if err := m.Runner.MustRun(m.IPBin, "route", "replace", "local", "default", "dev", "lo", "table", table); err != nil {
		if !strings.Contains(err.Error(), "exists") {
			return fmt.Errorf("ip route replace: %w", err)
		}
	}
	return nil
}

func (m *Manager) removePolicyRoute() error {
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", fmt.Sprintf("0x%x", Mark), "lookup", fmt.Sprint(Table))
	_ = m.Runner.Run(m.IPBin, "route", "flush", "table", fmt.Sprint(Table))
	return nil
}

// tproxyRules are the mangle-table rules that capture LAN traffic into Xray's
// TPROXY inbound. They run inside our private chain ChainPre.
func (m *Manager) tproxyRules() [][]string {
	rules := [][]string{}
	// Never capture Xray's own egress (marked with SelfMark).
	rules = append(rules, []string{"-m", "mark", "--mark", fmt.Sprint(m.SelfMark), "-j", "RETURN"})
	// Never capture bypassed destinations (LAN, router, reserved ranges).
	for _, cidr := range m.Bypass {
		rules = append(rules, []string{"-d", cidr, "-j", "RETURN"})
	}
	// TPROXY the rest (tcp + udp) to Xray, tagging with our fwmark.
	for _, proto := range []string{"tcp", "udp"} {
		rules = append(rules, []string{
			"-p", proto, "-j", "TPROXY",
			"--on-port", fmt.Sprint(m.TProxyPort),
			"--tproxy-mark", fmt.Sprintf("0x%x/0x%x", Mark, Mark),
		})
	}
	return rules
}

func (m *Manager) killSwitchRules() [][]string {
	rules := [][]string{}
	for _, cidr := range m.Bypass {
		rules = append(rules, []string{"-d", cidr, "-j", "RETURN"})
	}
	// Allow anything already marked for the tunnel; drop the rest.
	rules = append(rules, []string{"-m", "mark", "--mark", fmt.Sprintf("0x%x/0x%x", Mark, Mark), "-j", "RETURN"})
	rules = append(rules, []string{"-j", "DROP"})
	return rules
}

// applyChain creates (add=true) or tears down (add=false) a private chain and
// the jump into it. Table is "mangle" for TPROXY, "filter"/FORWARD for kill.
func (m *Manager) applyChain(chain string, rules [][]string, add bool) error {
	table, hook := "mangle", "PREROUTING"
	if chain == ChainKS {
		table, hook = "filter", "FORWARD"
	}
	if add {
		// Fresh chain: create, flush, fill, then jump from the hook.
		_ = m.Runner.Run(m.IPTables, "-t", table, "-N", chain)
		_ = m.Runner.Run(m.IPTables, "-t", table, "-F", chain)
		for _, r := range rules {
			args := append([]string{"-t", table, "-A", chain}, r...)
			if err := m.Runner.MustRun(m.IPTables, args...); err != nil {
				return err
			}
		}
		// Insert the jump once (delete any prior copy first for idempotency).
		_ = m.Runner.Run(m.IPTables, "-t", table, "-D", hook, "-j", chain)
		return m.Runner.MustRun(m.IPTables, "-t", table, "-I", hook, "-j", chain)
	}
	// Teardown: remove the jump, flush and delete the chain.
	_ = m.Runner.Run(m.IPTables, "-t", table, "-D", hook, "-j", chain)
	_ = m.Runner.Run(m.IPTables, "-t", table, "-F", chain)
	_ = m.Runner.Run(m.IPTables, "-t", table, "-X", chain)
	return nil
}

// HookScript returns the contents of the ndm netfilter.d hook that re-applies
// keen-manager's rules whenever KeeneticOS rebuilds the firewall.
func HookScript(binPath string) string {
	return `#!/bin/sh
# keen-manager netfilter hook — reapplies transparent-proxy rules after
# KeeneticOS flushes iptables on a topology change. Installed by keen-manager.
[ "$type" = "iptables" ] || exit 0
[ "$table" = "mangle" ] || exit 0
` + binPath + ` route reapply >/dev/null 2>&1 &
exit 0
`
}
