package nfqws

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// RequiredKernelModules are the netfilter kernel modules nfqws2 depends on to do
// its work: it queues packets to userspace via NFQUEUE, which needs
// nfnetlink_queue (the netlink transport) and xt_NFQUEUE (the iptables target).
// If either is absent, nfqws2 can start but its rules never fire — the DPI
// bypass silently does nothing. We treat their presence as a readiness gate so
// "running" never masquerades as "working".
var RequiredKernelModules = []string{"nfnetlink_queue", "xt_NFQUEUE"}

// moduleCompressionSuffixes are the compressed-module extensions seen on
// Keenetic/Entware and mainline distros; a module file may be any of these on
// top of the ".ko" base.
var moduleCompressionSuffixes = []string{"", ".gz", ".xz", ".zst"}

// KernelModulesStatus reports whether every RequiredKernelModule is available on
// this device — either already loaded (in /proc/modules) or present on disk in a
// module directory (loadable on demand, which is how nfqws2's init script pulls
// them in). It returns the readiness flag plus the names of any modules that are
// neither loaded nor on disk.
//
// This is a read-only probe (safe on- and off-device). Off-device /proc/modules
// and the module dirs are absent, so it honestly reports "not ready" — callers
// gate that behind Installed() so a dev box isn't flagged.
func (c *Controller) KernelModulesStatus() (ready bool, missing []string) {
	proc, _ := os.ReadFile("/proc/modules")
	loaded := string(proc)
	dirs := platform.KernelModuleDirs()
	for _, m := range RequiredKernelModules {
		if moduleLoaded(loaded, m) || moduleOnDisk(dirs, m) {
			continue
		}
		missing = append(missing, m)
	}
	return len(missing) == 0, missing
}

// moduleLoaded reports whether module name appears in /proc/modules content.
// Each line is "name size refcount deps state address"; the first field is the
// module name (kernel-normalised with underscores). Comparison normalises '-' to
// '_' and is case-insensitive so xt_NFQUEUE vs xt_nfqueue etc. still matches.
func moduleLoaded(procModules, name string) bool {
	want := normalizeModuleName(name)
	for _, line := range strings.Split(procModules, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if normalizeModuleName(fields[0]) == want {
			return true
		}
	}
	return false
}

// moduleOnDisk reports whether a "<name>.ko" file (optionally compressed) exists
// anywhere under any of dirs. It walks each directory tree (kernel modules live
// in nested subdirs) and stops at the first match. Missing/unreadable dirs are
// skipped rather than treated as errors.
func moduleOnDisk(dirs []string, name string) bool {
	want := normalizeModuleName(name)
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		found := false
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Skip unreadable subtrees; a missing root dir just yields no match.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if moduleFileMatches(d.Name(), want) {
				found = true
				return fs.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}

// moduleFileMatches reports whether a filename is the kernel-object file for the
// (already normalised) module want — i.e. "<want>.ko" with an optional
// compression suffix, compared case-insensitively with '-'/'_' folded.
func moduleFileMatches(filename, want string) bool {
	lower := strings.ToLower(filename)
	for _, suf := range moduleCompressionSuffixes {
		trimmed := strings.TrimSuffix(lower, suf)
		if trimmed == lower && suf != "" {
			continue // suffix not present; only "" (no suffix) always applies
		}
		if stem, ok := strings.CutSuffix(trimmed, ".ko"); ok {
			if normalizeModuleName(stem) == want {
				return true
			}
		}
	}
	return false
}

// normalizeModuleName lowercases and folds '-' to '_' so the many spellings of a
// module name (file vs /proc, dash vs underscore, case) compare equal.
func normalizeModuleName(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "_")
}
