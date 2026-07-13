package engine

import (
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// isSubTarget reports whether a route's TargetConnID is a subscription
// sentinel ("sub:<id>").
func isSubTarget(targetConnID string) bool {
	return strings.HasPrefix(targetConnID, subTargetPrefix)
}

// subIDFromTarget extracts the subscription ID from a "sub:<id>" sentinel.
// Returns "" if the target is not a subscription target.
func subIDFromTarget(targetConnID string) string {
	if !isSubTarget(targetConnID) {
		return ""
	}
	return strings.TrimPrefix(targetConnID, subTargetPrefix)
}

// isReservedTarget reports whether a target is a reserved sentinel (bypass
// or sub:) rather than a real connection ID. Used in validation to skip
// the findConn check.
func isReservedTarget(targetConnID string) bool {
	return targetConnID == bypassTargetID || isSubTarget(targetConnID)
}

// resolveSubTarget resolves a subscription-target route to the currently
// active member's native interface. If no member is active, returns "",
// false (the route stays pending until a member activates).
func (e *Engine) resolveSubTarget(st model.State, subID string) (string, bool) {
	// If the active connection belongs to this subscription, use its interface.
	active := st.ActiveConnID
	if active != "" {
		if ac, ok := findConn(st, active); ok && ac.SubscriptionID == subID {
			if name, ok := e.nativeIface(active); ok {
				return name, true
			}
			// Xray proxy-conn mode: use the shared ProxyN interface.
			if e.xrayProxyMode() && ac.Type == model.ConnXray {
				if p := e.managedProxyIface(); p != "" {
					return p, true
				}
			}
		}
	}
	// No active member of this subscription — route stays pending.
	return "", false
}

// subTargetLabel returns a human-readable label for a subscription target,
// e.g. "Sub: vpn (63 servers)".
func subTargetLabel(st model.State, subID string) string {
	sub, ok := findSub(st, subID)
	if !ok {
		return "Sub: (unknown)"
	}
	count := len(subMembers(st, subID))
	return "Sub: " + sub.Name + " (" + intToStr(count) + " servers)"
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
