// Package tpws drives a local tpws process — the socket-level DPI-desync proxy
// from the zapret project (the SOCKS/transparent sibling of nfqws). Unlike
// nfqws, which hooks NFQUEUE globally and exposes no socket endpoint, tpws in
// SOCKS mode listens on 127.0.0.1:<port>. That socket is what lets keen-manager
// expose DPI bypass as ONE routable KeeneticOS "Proxy" connection — the exact
// same model the Xray proxy-connection path uses (internal/engine/proxyconn.go
// and docs/XRAY-PROXY-PLAN.md): register a single managed ProxyN → the local
// tpws SOCKS port, then send only the chosen domains to it per-service via the
// router's dns-proxy stack, like a VPN tunnel — instead of a global inline
// NFQUEUE.
//
// Everything device-side goes through platform.Runner, so it is inert in
// dry-run / off-device. If the tpws binary is absent the engine surfaces a hint
// and leaves the router untouched (see engine/bypassconn.go); nothing here can
// brick the device.
package tpws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

const (
	// DefaultBindAddr is the loopback address tpws' SOCKS listener binds to. It
	// must stay on loopback: the managed Keenetic Proxy interface points at it,
	// and it must never be reachable from the WAN.
	DefaultBindAddr = "127.0.0.1"
	// DefaultPort is the local SOCKS port for the bypass proxy. It is distinct
	// from the Xray SOCKS inbound (10808) so the DPI-bypass and VPN exit points
	// can coexist as two independent Proxy connections.
	DefaultPort = 10809
	// DefaultStrategy is a conservative first-cut desync for TLS-SNI DPI. tpws
	// strategies are device- and ISP-specific and MUST be tuned on-device (the
	// Bypass page → Advanced); this default is a sane starting point, not a
	// guarantee that any given DPI is defeated.
	DefaultStrategy = "--split-tls=sni --disorder"
)

// Options are the tpws runtime parameters keen-manager supervises. They are
// rendered into the generated init script (S52tpws) as the process argv.
type Options struct {
	Port     int    // SOCKS listen port (0 → DefaultPort)
	BindAddr string // SOCKS bind address ("" → DefaultBindAddr)
	Strategy string // free-form tpws desync args ("" → DefaultStrategy)
}

func (o Options) port() int {
	if o.Port > 0 {
		return o.Port
	}
	return DefaultPort
}

func (o Options) bindAddr() string {
	if b := strings.TrimSpace(o.BindAddr); b != "" {
		return b
	}
	return DefaultBindAddr
}

func (o Options) strategy() string {
	if s := strings.TrimSpace(o.Strategy); s != "" {
		return s
	}
	return DefaultStrategy
}

// Args builds the tpws argv (excluding the binary path) for SOCKS mode. The
// strategy string is whitespace-split into individual flags, so a flag value
// must not contain spaces — tpws desync flags never do (e.g. --split-tls=sni,
// --split-pos=1, --disorder, --oob).
func (o Options) Args() []string {
	args := []string{
		"--socks",
		"--bind-addr=" + o.bindAddr(),
		fmt.Sprintf("--port=%d", o.port()),
	}
	return append(args, strings.Fields(o.strategy())...)
}

// Controller manages the tpws process and its generated init script on the
// device. Device-side effects (start/stop/restart) go through platform.Runner
// so they can be dry-run off-device.
type Controller struct {
	Paths  platform.Paths
	Runner *platform.Runner
	// InitScript is the keen-manager-owned Entware init script (S52tpws). Unlike
	// nfqws2's opkg-provided S51nfqws2, keen-manager generates this one itself so
	// it can bake the chosen SOCKS port + desync strategy into the argv.
	InitScript string
	// BinPath is the tpws binary location (Entware /opt/usr/bin/tpws by default).
	BinPath string
	// PkgName is the opkg package that provides tpws, for a best-effort install.
	PkgName string
}

// NewController returns a Controller with standard Entware paths.
func NewController(p platform.Paths, r *platform.Runner) *Controller {
	return &Controller{
		Paths:      p,
		Runner:     r,
		InitScript: filepath.Join(p.InitDir, "S52tpws"),
		BinPath:    p.TpwsBin,
		PkgName:    "tpws",
	}
}

func (c *Controller) bin() string {
	if c.BinPath != "" && platform.FileExists(c.BinPath) {
		return c.BinPath
	}
	return "tpws"
}

// Installed reports whether a tpws binary is available (at the Entware path or
// on PATH). This is the gate the engine checks before offering the routable
// bypass interface; when false it surfaces a hint to install tpws.
func (c *Controller) Installed() bool {
	return (c.BinPath != "" && platform.FileExists(c.BinPath)) || platform.Which("tpws")
}

// Running reports whether the tpws daemon is currently active.
func (c *Controller) Running() bool {
	if !c.Installed() {
		return false
	}
	if platform.FileExists(c.InitScript) {
		out, _ := c.Runner.Output(c.InitScript, "status")
		l := strings.ToLower(out)
		if strings.Contains(l, "running") && !strings.Contains(l, "not running") {
			return true
		}
	}
	if _, err := c.Runner.Output("pgrep", "-f", c.bin()); err == nil {
		return true
	}
	return false
}

// Version returns the installed package version via opkg, if available.
func (c *Controller) Version() string {
	out, err := c.Runner.Output("opkg", "status", c.PkgName)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}
	}
	return ""
}

// Apply (re)writes the init script with opts and restarts tpws so a changed
// port/strategy takes effect. It is the tpws analogue of xray.Controller.Apply.
// Inert in dry-run (Runner is a no-op and the script write is skipped).
func (c *Controller) Apply(opts Options) error {
	if err := c.writeInitScript(opts); err != nil {
		return err
	}
	return c.Restart()
}

// Start/Stop/Restart drive the generated init script (writing a default one
// first if none exists yet, so start/restart always have something to run).
func (c *Controller) Start() error   { return c.service("start") }
func (c *Controller) Stop() error    { return c.service("stop") }
func (c *Controller) Restart() error { return c.service("restart") }

func (c *Controller) service(action string) error {
	if !platform.FileExists(c.InitScript) {
		switch action {
		case "start", "restart":
			// No script yet — write one with defaults so the daemon can start.
			// Apply() overwrites it with the user's chosen options.
			if err := c.writeInitScript(Options{}); err != nil {
				return err
			}
			if c.Runner.DryRun {
				return nil // dry-run skips the write, so there is nothing to run
			}
		case "stop":
			return nil // nothing installed to stop
		}
	}
	return c.Runner.MustRun(c.InitScript, action)
}

// writeInitScript renders the S52tpws init script for opts. Skipped in dry-run
// (no device to write to), mirroring xray.ensureInitScript.
func (c *Controller) writeInitScript(opts Options) error {
	if c.Runner.DryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.InitScript), 0o755); err != nil {
		return err
	}
	script := InitScript(c.bin(), opts.Args(), c.Paths.PidFile("tpws"), c.Paths.LogFile("tpws"))
	if err := os.WriteFile(c.InitScript, []byte(script), 0o755); err != nil {
		return err
	}
	_ = os.Chmod(c.InitScript, 0o755)
	return nil
}

// RemoveInitScript deletes the generated init script (best-effort), used when
// the bypass feature is turned off so tpws does not restart on reboot.
func (c *Controller) RemoveInitScript() error {
	if c.Runner.DryRun {
		return nil
	}
	err := os.Remove(c.InitScript)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Install installs the tpws package via opkg (device-side, best-effort). The
// feed must already be configured. Returns an error if opkg fails; the caller
// falls back to a "tpws unavailable" hint.
func (c *Controller) Install() error {
	if err := c.Runner.MustRun("opkg", "update"); err != nil {
		return err
	}
	return c.Runner.MustRun("opkg", "install", c.PkgName)
}

// InitScript renders a minimal Entware/procd-style init script that daemonizes
// tpws with a pidfile and log file. The argv is baked in (SOCKS port +
// strategy), so a reconfigure rewrites this file via Apply and restarts.
func InitScript(bin string, args []string, pidFile, logFile string) string {
	return fmt.Sprintf(`#!/bin/sh
# keen-manager: tpws DPI-bypass proxy (auto-generated — safe to edit; keen-manager
# overwrites it when you change the port or strategy on the Bypass page).
BIN="%s"
ARGS="%s"
PIDFILE="%s"
LOGFILE="%s"

start() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
        return 0
    fi
    "$BIN" $ARGS >>"$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
}
stop() {
    [ -f "$PIDFILE" ] && kill "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null
    rm -f "$PIDFILE"
    pkill -f "$BIN --socks" 2>/dev/null
    return 0
}
status() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
        echo "running"
    else
        echo "not running"
    fi
}
case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    status)  status ;;
    *) echo "usage: $0 {start|stop|restart|status}"; exit 1 ;;
esac
`, bin, strings.Join(args, " "), pidFile, logFile)
}
