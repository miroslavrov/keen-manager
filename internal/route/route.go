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
//
// TPROXY CANON — the capture chain mirrors the proven XKeen/Xray transparent-
// proxy ruleset, which is what makes captured traffic actually reach the tunnel
// rather than blackholing (the "connects but only local IPs work" report):
//
//   - every iptables call passes `-w` so KeeneticOS's concurrent ndm rewrites
//     can't fail us with "another app is holding the xtables lock" (exit 4);
//   - `-m socket --transparent` diverts packets that already belong to one of
//     Xray's transparent sockets (replies + mid-stream) straight to local
//     delivery instead of re-TPROXY'ing or dropping them;
//   - `-m conntrack --ctstate DNAT,INVALID -j RETURN` leaves port-forwards and
//     invalid packets alone;
//   - the TPROXY target sets `--on-ip 127.0.0.1` so delivery is to the local
//     Xray listener regardless of the incoming interface;
//   - the policy route matches the fwmark with a mask so a mark KeeneticOS may
//     have already set in other bits doesn't defeat the exact-match rule.
//
// The two optional-module rules (`-m socket`, `-m conntrack`) are best-effort:
// on an iptables build lacking xt_socket/xt_conntrack they are skipped with a
// log line and capture still works for new connections.
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

	// iptWaitSeconds documents the intended xtables lock-wait timeout (5s).
	// The actual flag is bare "-w" (see iptArgs) because iptables v1.4.21 on
	// Keenetic rejects the numeric form ("-w 5" → "Bad argument `5'") while the
	// bare form carries the same default 5s wait on every version that
	// supports -w at all (1.4.20+).
	iptWaitSeconds = "5"
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
	// Pick a `ip` binary that is actually runnable on this CPU. A present-but-
	// wrong-arch/corrupt /opt/sbin/ip (the classic Entware "exec format error")
	// is skipped in favour of the next candidate rather than fataling every
	// `ip rule add`; ResolveIP falls back to the firmware ip and finally PATH.
	ip := platform.ResolveIP(p)
	return &Manager{
		Runner:     r,
		IPBin:      ip,
		IPTables:   ipt,
		TProxyPort: 12345,
		Bypass:     append([]string(nil), DefaultBypass...),
		SelfMark:   255,
	}
}

// iptRule is one rule in a private chain. A bestEffort rule is applied with Run
// (its failure is logged, not fatal) so an iptables build lacking an optional
// match module (xt_socket / xt_conntrack) degrades to plain capture instead of
// aborting the whole tunnel bring-up.
type iptRule struct {
	args       []string
	bestEffort bool
}

// iptArgs prepends the xtables lock-wait flag to an iptables argument list. A
// fresh slice is returned each call (never aliases a shared backing array).
//
// Uses bare "-w" (no numeric argument) because iptables v1.4.21 on Keenetic
// supports the flag but not the timeout form ("-w 5" → "Bad argument `5'").
// The bare flag carries the same default ~5s wait on all versions that
// support -w (1.4.20+), which comfortably outlasts a concurrent ndm rewrite.
func iptArgs(args ...string) []string {
	return append([]string{"-w"}, args...)
}

// markMask is the fwmark match in mask form ("0x2333/0x2333") so only our bits
// are compared — a mark KeeneticOS set in other bits won't defeat the match.
func (m *Manager) markMask() string { return fmt.Sprintf("0x%x/0x%x", Mark, Mark) }

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

// Verify reports whether the TPROXY capture chain is installed and the
// PREROUTING jump points to it. It is a read-only probe (iptables -S) so it is
// safe to call at any time, including from the post-activate verify loop. In
// dry-run mode it returns nil (nothing to verify off-device).
func (m *Manager) Verify() error {
	if m.Runner.DryRun {
		return nil
	}
	// The chain must exist and contain rules.
	res := m.Runner.Run(m.IPTables, iptArgs("-t", "mangle", "-S", ChainPre)...)
	if res.Err != nil {
		return fmt.Errorf("TPROXY chain %s not installed: %w", ChainPre, res.Err)
	}
	if len(strings.TrimSpace(res.Stdout)) == 0 {
		return fmt.Errorf("TPROXY chain %s is empty", ChainPre)
	}
	// The PREROUTING hook must jump to our chain.
	res = m.Runner.Run(m.IPTables, iptArgs("-t", "mangle", "-S", "PREROUTING")...)
	if res.Err != nil {
		return fmt.Errorf("cannot read PREROUTING: %w", res.Err)
	}
	if !strings.Contains(res.Stdout, "-j "+ChainPre) {
		return fmt.Errorf("PREROUTING does not jump to %s", ChainPre)
	}
	return nil
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
	mm := m.markMask()
	legacy := fmt.Sprintf("0x%x", Mark) // pre-rc.8 unmasked form
	table := fmt.Sprint(Table)

	// `ip rule add` ALWAYS appends, even an identical rule — so a naive add on
	// every Reapply() (the ndm hook fires on each topology change) would leak
	// duplicate rules indefinitely. Delete any existing copy first (both the
	// current masked form and the legacy unmasked one an older build may have
	// left), then add exactly one. The dels are best-effort (error when absent).
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", mm, "lookup", table)
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", legacy, "lookup", table)
	if err := m.Runner.MustRun(m.IPBin, "rule", "add", "fwmark", mm, "lookup", table); err != nil {
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
	table := fmt.Sprint(Table)
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", m.markMask(), "lookup", table)
	_ = m.Runner.Run(m.IPBin, "rule", "del", "fwmark", fmt.Sprintf("0x%x", Mark), "lookup", table)
	_ = m.Runner.Run(m.IPBin, "route", "flush", "table", table)
	return nil
}

// tproxyRules are the mangle-table rules that capture LAN traffic into Xray's
// TPROXY inbound. They run inside our private chain ChainPre, in this order:
// divert established transparent sockets, skip DNAT/invalid, skip our own
// egress and the bypass ranges, then TPROXY everything else.
func (m *Manager) tproxyRules() []iptRule {
	mm := m.markMask()
	rules := []iptRule{
		// Leave port-forwarded (DNAT) and invalid packets alone — they are not
		// LAN egress and must not be captured. Best-effort: needs xt_conntrack.
		{args: []string{"-m", "conntrack", "--ctstate", "DNAT,INVALID", "-j", "RETURN"}, bestEffort: true},
		// Packets that already belong to one of Xray's transparent sockets
		// (replies + mid-stream of an established capture): (re)mark for local
		// delivery and accept, so they reach the existing socket instead of being
		// re-TPROXY'd or lost. This is the XKeen "-m socket --transparent" divert;
		// its absence is the classic "handshake OK but no data / only local IPs
		// work" TPROXY failure. Best-effort: needs xt_socket.
		{args: []string{"-p", "tcp", "-m", "socket", "--transparent", "-j", "MARK", "--set-mark", mm}, bestEffort: true},
		{args: []string{"-p", "tcp", "-m", "socket", "--transparent", "-j", "ACCEPT"}, bestEffort: true},
		{args: []string{"-p", "udp", "-m", "socket", "--transparent", "-j", "MARK", "--set-mark", mm}, bestEffort: true},
		{args: []string{"-p", "udp", "-m", "socket", "--transparent", "-j", "ACCEPT"}, bestEffort: true},
		// Never capture Xray's own egress (marked with SelfMark).
		{args: []string{"-m", "mark", "--mark", fmt.Sprint(m.SelfMark), "-j", "RETURN"}},
	}
	// Never capture bypassed destinations (LAN, router, reserved ranges).
	for _, cidr := range m.Bypass {
		rules = append(rules, iptRule{args: []string{"-d", cidr, "-j", "RETURN"}})
	}
	// TPROXY the rest (tcp + udp) to Xray's transparent inbound on the local
	// listener, tagging with our fwmark. --on-ip 127.0.0.1 pins delivery to the
	// local Xray socket regardless of which interface the packet arrived on
	// (the default is the incoming interface IP, which misdelivers on a router).
	for _, proto := range []string{"tcp", "udp"} {
		rules = append(rules, iptRule{args: []string{
			"-p", proto, "-j", "TPROXY",
			"--on-ip", "127.0.0.1",
			"--on-port", fmt.Sprint(m.TProxyPort),
			"--tproxy-mark", mm,
		}})
	}
	return rules
}

func (m *Manager) killSwitchRules() []iptRule {
	mm := m.markMask()
	rules := []iptRule{}
	for _, cidr := range m.Bypass {
		rules = append(rules, iptRule{args: []string{"-d", cidr, "-j", "RETURN"}})
	}
	// Allow anything already marked for the tunnel; drop the rest.
	rules = append(rules, iptRule{args: []string{"-m", "mark", "--mark", mm, "-j", "RETURN"}})
	rules = append(rules, iptRule{args: []string{"-j", "DROP"}})
	return rules
}

// applyChain creates (add=true) or tears down (add=false) a private chain and
// the jump into it. Table is "mangle" for TPROXY, "filter"/FORWARD for kill.
// Every iptables call carries -w so a concurrent ndm rewrite can't fail it.
func (m *Manager) applyChain(chain string, rules []iptRule, add bool) error {
	table, hook := "mangle", "PREROUTING"
	if chain == ChainKS {
		table, hook = "filter", "FORWARD"
	}
	if add {
		// Fresh chain: create, flush, fill, then jump from the hook.
		_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-N", chain)...)
		_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-F", chain)...)
		for _, r := range rules {
			args := iptArgs(append([]string{"-t", table, "-A", chain}, r.args...)...)
			if r.bestEffort {
				// Optional-module rule: log and continue on failure so a build
				// without xt_socket/xt_conntrack still captures new connections.
				if res := m.Runner.Run(m.IPTables, args...); res.Err != nil && m.Runner.Log != nil {
					m.Runner.Log(fmt.Sprintf("keen-manager: optional TPROXY rule skipped (%v): %s",
						res.Err, strings.Join(r.args, " ")))
				}
				continue
			}
			if err := m.Runner.MustRun(m.IPTables, args...); err != nil {
				return err
			}
		}
		// Insert the jump once (delete any prior copy first for idempotency).
		_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-D", hook, "-j", chain)...)
		return m.Runner.MustRun(m.IPTables, iptArgs("-t", table, "-I", hook, "-j", chain)...)
	}
	// Teardown: remove the jump, flush and delete the chain.
	_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-D", hook, "-j", chain)...)
	_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-F", chain)...)
	_ = m.Runner.Run(m.IPTables, iptArgs("-t", table, "-X", chain)...)
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
