package awg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
)

// Controller manages AmneziaWG tunnels on the device.
//
// SAFETY: all state-changing operations go through platform.Runner (dry-run
// aware). The preferred path is `awg-quick`, which is the proven wg-quick fork.
// A manual ip/awg fallback is provided for environments without awg-quick.
type Controller struct {
	Paths  platform.Paths
	Runner *platform.Runner
	// ConfDir is where per-tunnel .conf files live (awg-quick reads these).
	ConfDir string
}

// NewController returns a Controller using the standard Entware AWG conf dir.
func NewController(p platform.Paths, r *platform.Runner) *Controller {
	return &Controller{Paths: p, Runner: r, ConfDir: filepath.Join(p.Root, "etc", "amneziawg")}
}

func (c *Controller) confPath(name string) string {
	return filepath.Join(c.ConfDir, name+".conf")
}

func (c *Controller) awgQuick() (string, bool) {
	for _, p := range []string{filepath.Join(c.Paths.Root, "bin", "awg-quick"), "awg-quick"} {
		if platform.Which(p) {
			return p, true
		}
	}
	return "", false
}

func (c *Controller) awgBin() string {
	if platform.FileExists(c.Paths.AwgBin) {
		return c.Paths.AwgBin
	}
	return "awg"
}

func (c *Controller) ipBin() string {
	if platform.FileExists(c.Paths.IPBin) {
		return c.Paths.IPBin
	}
	return "ip"
}

// WriteConfig validates and writes a tunnel .conf to ConfDir (root-only perms).
func (c *Controller) WriteConfig(name string, cfg *model.AWGConfig) (string, error) {
	if err := Validate(cfg); err != nil {
		return "", err
	}
	if err := os.MkdirAll(c.ConfDir, 0o700); err != nil {
		return "", err
	}
	path := c.confPath(name)
	if err := os.WriteFile(path, []byte(Generate(cfg)), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Up brings a tunnel up. If cfg is non-nil it is written first.
func (c *Controller) Up(name string, cfg *model.AWGConfig) error {
	if cfg != nil {
		if _, err := c.WriteConfig(name, cfg); err != nil {
			return err
		}
	}
	if bin, ok := c.awgQuick(); ok {
		return c.Runner.MustRun(bin, "up", name)
	}
	return c.manualUp(name)
}

// Down tears a tunnel down.
func (c *Controller) Down(name string) error {
	if bin, ok := c.awgQuick(); ok {
		return c.Runner.MustRun(bin, "down", name)
	}
	return c.manualDown(name)
}

// manualUp is the fallback when awg-quick is not installed. It performs the core
// wg-quick steps with `ip` and `awg`. AllowedIPs routing is intentionally left
// to a higher layer (policy-based routing) to avoid clobbering the default route.
func (c *Controller) manualUp(name string) error {
	conf := c.confPath(name)
	if !platform.FileExists(conf) {
		return fmt.Errorf("config %s not found", conf)
	}
	steps := [][]string{
		{c.ipBin(), "link", "add", "dev", name, "type", "amneziawg"},
		{c.awgBin(), "setconf", name, conf},
		{c.ipBin(), "link", "set", "up", "dev", name},
	}
	for _, s := range steps {
		if err := c.Runner.MustRun(s[0], s[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) manualDown(name string) error {
	return c.Runner.MustRun(c.ipBin(), "link", "del", "dev", name)
}

// Show returns parsed health for a tunnel interface.
func (c *Controller) Show(name string) (Health, error) {
	out, err := c.Runner.Output(c.awgBin(), "show", name)
	if err != nil {
		return Health{HandshakeAgeSec: -1}, err
	}
	return ParseShow(out), nil
}

// EndpointHost extracts host:port from a config's peer endpoint (for pinning a
// host route via the WAN gateway to prevent a routing loop when AllowedIPs=/0).
func EndpointHost(cfg *model.AWGConfig) string {
	ep := cfg.Peer.Endpoint
	if i := strings.LastIndexByte(ep, ':'); i >= 0 {
		return strings.Trim(ep[:i], "[]")
	}
	return ep
}
