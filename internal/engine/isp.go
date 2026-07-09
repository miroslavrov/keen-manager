package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/keenetic"
)

// DetectISPInterface picks the WAN/ISP uplink device name to seed nfqws2's
// ISP_INTERFACE. It prefers the RCI interface listing, which is authoritative
// even when a VPN tunnel currently owns the OS default route (PickWANInterface
// filters tunnels out), returning that interface's kernel device name — what
// nfqws2 filters on. It falls back to the OS default-route device only when RCI
// is unavailable, and refuses an obviously tunnel-like fallback so an active VPN
// can't be mistaken for the WAN. Returns ("","",err) when nothing WAN-like can
// be determined (e.g. off-device). Validate the chosen interface on-device.
func (e *Engine) DetectISPInterface() (dev, source string, err error) {
	if e.keenetic != nil && !e.runner.DryRun {
		ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
		infos, lerr := keenetic.ListInterfaces(ctx, e.keenetic)
		cancel()
		if lerr == nil {
			if wan, ok := keenetic.PickWANInterface(infos); ok {
				return firstNonEmpty(strings.TrimSpace(wan.SysName), wan.Name), "rci", nil
			}
		}
	}
	if w := e.detectWAN(); w.Interface != "" && !looksLikeTunnelDev(w.Interface) {
		return w.Interface, "default-route", nil
	}
	return "", "", fmt.Errorf("could not determine the WAN interface — run on-device (an active tunnel may be masking it)")
}

// looksLikeTunnelDev reports whether a device name is a VPN/tunnel interface
// rather than a physical/PPP WAN uplink, so the default-route fallback in
// DetectISPInterface never mistakes an active tunnel for the ISP interface.
// Pure, so it is unit-tested. (PPPoE "ppp0" is a real WAN and is NOT matched.)
func looksLikeTunnelDev(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, p := range []string{"wireguard", "nwg", "awg", "wg", "km", "proxy", "tun", "tap"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}
