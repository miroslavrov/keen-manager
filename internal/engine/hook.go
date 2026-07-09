package engine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// HookStatusView reports whether the ndm netfilter.d hook keen-manager installs
// (to reapply transparent-proxy / kill-switch rules after KeeneticOS rebuilds
// iptables on a topology change) is present and correctly wired. It turns "the
// hook silently isn't firing" — an invisible failure that lets Xray TPROXY
// routing quietly break after a WAN flap — into something the UI/CLI can show.
type HookStatusView struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	// Wired is true when the installed hook actually invokes keen-manager's
	// `route reapply` (guards against a truncated or foreign file at the path).
	Wired bool `json:"wired"`
	// BinaryPresent is true when the binary the hook calls still exists; a stale
	// hook left pointing at a moved/removed binary would silently stop firing.
	BinaryPresent bool `json:"binary_present"`
}

// HookPath is the absolute path of the ndm netfilter hook.
func (e *Engine) HookPath() string {
	return filepath.Join(e.Paths.NdmDir, "netfilter.d", HookName)
}

// HookInstalled reports whether the ndm netfilter hook file exists.
func (e *Engine) HookInstalled() bool {
	return platform.FileExists(e.HookPath())
}

// HookStatus inspects the installed hook: presence, that it invokes `route
// reapply`, and that the binary it calls still exists. Read-only (dry-run safe).
func (e *Engine) HookStatus() HookStatusView {
	v := HookStatusView{Path: e.HookPath()}
	b, err := os.ReadFile(v.Path)
	if err != nil {
		return v
	}
	v.Installed = true
	body := string(b)
	v.Wired = strings.Contains(body, "route reapply")
	if bin := hookBinaryPath(body); bin != "" {
		v.BinaryPresent = platform.FileExists(bin)
	}
	return v
}

// hookBinaryPath extracts the binary path the hook invokes (the token before
// " route reapply"), so HookStatus can check it still exists. "" if not found.
// Pure, so it is unit-tested against route.HookScript's output.
func hookBinaryPath(script string) string {
	const marker = " route reapply"
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			return strings.TrimSpace(line[:i])
		}
	}
	return ""
}
