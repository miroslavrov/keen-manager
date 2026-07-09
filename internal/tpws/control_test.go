package tpws

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// TestOptionsArgsDefaults confirms an empty Options renders the SOCKS-mode argv
// with the package defaults (loopback bind, DefaultPort, DefaultStrategy).
func TestOptionsArgsDefaults(t *testing.T) {
	got := Options{}.Args()
	want := []string{"--socks", "--bind-addr=127.0.0.1", "--port=10809", "--split-tls=sni", "--disorder"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("default Args() = %v, want %v", got, want)
	}
}

// TestOptionsArgsOverrides confirms explicit port/bind/strategy override the
// defaults and the strategy is whitespace-split into individual flags.
func TestOptionsArgsOverrides(t *testing.T) {
	got := Options{Port: 12000, BindAddr: "127.0.0.5", Strategy: "--split-pos=1 --oob --disorder"}.Args()
	want := []string{"--socks", "--bind-addr=127.0.0.5", "--port=12000", "--split-pos=1", "--oob", "--disorder"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Args() = %v, want %v", got, want)
	}
}

// TestOptionsArgsAlwaysSocks confirms --socks is always first (the Keenetic
// Proxy interface points at a SOCKS5 upstream, so tpws must be a SOCKS server).
func TestOptionsArgsAlwaysSocks(t *testing.T) {
	if a := (Options{Strategy: "  "}).Args(); a[0] != "--socks" {
		t.Errorf("expected --socks first, got %v", a)
	}
}

// TestInitScriptRendersArgv confirms the generated init script bakes in the
// binary, the full argv and the pid/log paths, and exposes the standard verbs.
func TestInitScriptRendersArgv(t *testing.T) {
	s := InitScript("/opt/usr/bin/tpws", Options{Port: 10809}.Args(), "/var/run/tpws.pid", "/opt/var/log/tpws.log")
	for _, want := range []string{
		`BIN="/opt/usr/bin/tpws"`,
		`--socks --bind-addr=127.0.0.1 --port=10809`,
		`PIDFILE="/var/run/tpws.pid"`,
		`LOGFILE="/opt/var/log/tpws.log"`,
		"restart) stop; sleep 1; start",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("init script missing %q\n---\n%s", want, s)
		}
	}
}

// TestControllerDetection confirms Installed() keys off the binary path and the
// controller wires the S52tpws init path under the Entware init dir.
func TestControllerDetection(t *testing.T) {
	dir := t.TempDir()
	p := platform.Paths{
		InitDir: filepath.Join(dir, "init.d"),
		TpwsBin: filepath.Join(dir, "tpws"),
		RunDir:  filepath.Join(dir, "run"),
		LogDir:  filepath.Join(dir, "log"),
	}
	r := platform.NewRunner()
	r.DryRun = true
	c := NewController(p, r)

	if c.InitScript != filepath.Join(p.InitDir, "S52tpws") {
		t.Errorf("InitScript = %q, want .../S52tpws", c.InitScript)
	}
	// No binary on disk and (assuming) none on PATH → not installed / not running.
	if platform.Which("tpws") {
		t.Skip("tpws present on PATH in this environment; detection test not meaningful")
	}
	if c.Installed() {
		t.Error("Installed() should be false with no tpws binary")
	}
	if c.Running() {
		t.Error("Running() should be false when not installed")
	}

	// Apply is inert in dry-run (no script written, Runner is a no-op).
	if err := c.Apply(Options{}); err != nil {
		t.Errorf("Apply in dry-run should be a no-op, got %v", err)
	}
	if platform.FileExists(c.InitScript) {
		t.Error("dry-run Apply must not write the init script")
	}
}
