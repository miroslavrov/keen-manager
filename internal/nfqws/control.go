package nfqws

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
)

// Controller drives the nfqws2-keenetic service.
type Controller struct {
	Paths     platform.Paths
	Runner    *platform.Runner
	ConfDir   string // /opt/etc/nfqws2
	InitPath  string // /opt/etc/init.d/S51nfqws2
	BinPath   string // /opt/usr/bin/nfqws2
	PkgName   string // opkg package name
}

// NewController returns a Controller with standard Entware paths.
func NewController(p platform.Paths, r *platform.Runner) *Controller {
	return &Controller{
		Paths:    p,
		Runner:   r,
		ConfDir:  p.NfqwsConfDir,
		InitPath: p.NfqwsInit,
		BinPath:  p.NfqwsBin,
		PkgName:  "nfqws2-keenetic",
	}
}

func (c *Controller) confFile() string { return filepath.Join(c.ConfDir, "nfqws2.conf") }

// Installed reports whether the nfqws2 binary + init script are present.
func (c *Controller) Installed() bool {
	return platform.FileExists(c.BinPath) && platform.FileExists(c.InitPath)
}

// Running reports whether the daemon is currently active.
func (c *Controller) Running() bool {
	if !c.Installed() {
		return false
	}
	out, _ := c.Runner.Output(c.InitPath, "status")
	l := strings.ToLower(out)
	if strings.Contains(l, "running") && !strings.Contains(l, "not running") {
		return true
	}
	// Fallback: pgrep the binary.
	if _, err := c.Runner.Output("pgrep", "-f", c.BinPath); err == nil {
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

// Status returns a snapshot for the API/UI.
func (c *Controller) Status() model.NfqwsStatusView {
	v := model.NfqwsStatusView{Installed: c.Installed()}
	if v.Installed {
		v.Running = c.Running()
		v.Version = c.Version()
		if m, err := c.Mode(); err == nil {
			v.Mode = m
		}
		v.KernelReady, v.MissingModules = c.KernelModulesStatus()
		v.Healthy = v.Running && v.KernelReady
	}
	return v
}

// Action runs an init-script verb: start|stop|restart|reload|status.
func (c *Controller) Action(action string) error {
	switch action {
	case "start", "stop", "restart", "reload", "status":
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	if !platform.FileExists(c.InitPath) {
		return fmt.Errorf("nfqws2 is not installed")
	}
	return c.Runner.MustRun(c.InitPath, action)
}

// Reload sends SIGHUP via the init script to reload hostlists without a full
// restart (nfqws2 supports this).
func (c *Controller) Reload() error { return c.Action("reload") }

// Install installs the nfqws2-keenetic package via opkg (device-side). The feed
// must already be configured by the keen-manager installer.
func (c *Controller) Install() error {
	if err := c.Runner.MustRun("opkg", "update"); err != nil {
		return err
	}
	return c.Runner.MustRun("opkg", "install", c.PkgName)
}
