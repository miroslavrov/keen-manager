package nfqws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleLoaded(t *testing.T) {
	// A realistic /proc/modules excerpt.
	proc := `nfnetlink_queue 20480 1 - Live 0x0000000000000000
xt_NFQUEUE 16384 3 - Live 0x0000000000000000
nf_conntrack 155648 5 xt_NFQUEUE,nf_nat - Live 0x0000000000000000`

	if !moduleLoaded(proc, "nfnetlink_queue") {
		t.Error("nfnetlink_queue should be reported loaded")
	}
	// Case / dash folding: xt_NFQUEUE listed uppercase; query lowercase+dash.
	if !moduleLoaded(proc, "xt-nfqueue") {
		t.Error("xt_NFQUEUE should match xt-nfqueue (case/dash folded)")
	}
	if moduleLoaded(proc, "xt_TPROXY") {
		t.Error("xt_TPROXY is absent and must not be reported loaded")
	}
	if moduleLoaded("", "nfnetlink_queue") {
		t.Error("empty /proc/modules must report nothing loaded")
	}
}

func TestModuleFileMatches(t *testing.T) {
	yes := map[string]string{
		"nfnetlink_queue.ko":     "nfnetlink_queue",
		"nfnetlink_queue.ko.gz":  "nfnetlink_queue",
		"nfnetlink_queue.ko.xz":  "nfnetlink_queue",
		"xt_NFQUEUE.ko":          "xt_nfqueue", // file uppercase, want normalised
		"xt_NFQUEUE.ko.zst":      "xt_nfqueue",
	}
	for file, want := range yes {
		if !moduleFileMatches(file, want) {
			t.Errorf("moduleFileMatches(%q,%q) = false, want true", file, want)
		}
	}
	no := map[string]string{
		"nfnetlink_queue.txt":    "nfnetlink_queue", // not a .ko
		"xt_NFQUEUEv2.ko":        "xt_nfqueue",       // different module
		"README":                 "nfnetlink_queue",
	}
	for file, want := range no {
		if moduleFileMatches(file, want) {
			t.Errorf("moduleFileMatches(%q,%q) = true, want false", file, want)
		}
	}
}

func TestModuleOnDisk(t *testing.T) {
	root := t.TempDir()
	// Nested layout like /lib/modules/<kver>/kernel/net/netfilter/.
	deep := filepath.Join(root, "kernel", "net", "netfilter")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "xt_NFQUEUE.ko"), []byte("elf"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs := []string{filepath.Join(root, "does-not-exist"), root}
	if !moduleOnDisk(dirs, normalizeModuleName("xt_NFQUEUE")) {
		t.Error("xt_NFQUEUE.ko should be found in the nested tree")
	}
	if moduleOnDisk(dirs, normalizeModuleName("nfnetlink_queue")) {
		t.Error("nfnetlink_queue is absent on disk and must not be found")
	}
	// A totally missing directory set yields no match, no panic.
	if moduleOnDisk([]string{filepath.Join(root, "nope")}, "xt_nfqueue") {
		t.Error("missing dir must not report a match")
	}
}

func TestKernelModulesStatusOffDevice(t *testing.T) {
	// Off-device there is no /proc/modules entry and no module dirs for these,
	// so a fresh controller must honestly report not-ready with both missing.
	c := newTestController(t)
	ready, missing := c.KernelModulesStatus()
	if ready {
		// This can only be true if the CI host genuinely has these modules; that
		// would be unusual for a sandbox but not a correctness failure, so only
		// assert the invariant that ready <=> no missing.
		if len(missing) != 0 {
			t.Errorf("ready=true but missing=%v", missing)
		}
		return
	}
	if len(missing) == 0 {
		t.Error("not ready but no missing modules listed")
	}
}
