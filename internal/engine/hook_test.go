package engine

import (
	"path/filepath"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/route"
)

func TestHookBinaryPath(t *testing.T) {
	script := route.HookScript("/opt/bin/keen-manager")
	if got := hookBinaryPath(script); got != "/opt/bin/keen-manager" {
		t.Fatalf("hookBinaryPath = %q, want /opt/bin/keen-manager", got)
	}
	if got := hookBinaryPath("#!/bin/sh\necho nothing here\n"); got != "" {
		t.Fatalf("hookBinaryPath on unrelated script = %q, want empty", got)
	}
}

func TestHookStatusInstallRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := platform.Paths{
		Root:        dir,
		DataDir:     dir,
		BackupDir:   filepath.Join(dir, "backups"),
		LogDir:      filepath.Join(dir, "log"),
		RunDir:      filepath.Join(dir, "run"),
		XrayConfDir: filepath.Join(dir, "xray"),
		NdmDir:      filepath.Join(dir, "ndm"),
	}
	e, err := New(p, true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Before install: absent.
	if st := e.HookStatus(); st.Installed {
		t.Fatalf("hook should not be installed initially: %+v", st)
	}
	if e.HookInstalled() {
		t.Fatal("HookInstalled should be false before install")
	}

	// After install: present + wired, and the binary it points at (the test
	// binary, via os.Executable) exists.
	if err := e.InstallHook(); err != nil {
		t.Fatalf("InstallHook: %v", err)
	}
	st := e.HookStatus()
	if !st.Installed || !st.Wired {
		t.Fatalf("hook should be installed and wired: %+v", st)
	}
	if !st.BinaryPresent {
		t.Fatalf("hook should point at an existing binary: %+v", st)
	}
	if st.Path == "" || !e.HookInstalled() {
		t.Fatalf("hook path/installed inconsistent: %+v", st)
	}

	// After uninstall: absent again.
	if err := e.UninstallHook(); err != nil {
		t.Fatalf("UninstallHook: %v", err)
	}
	if e.HookInstalled() {
		t.Fatal("HookInstalled should be false after uninstall")
	}
}
