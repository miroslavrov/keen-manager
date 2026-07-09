package xray

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// Controller manages the Xray process and its configuration on the device.
// Device-side effects (start/stop) go through platform.Runner so they can be
// dry-run off-device.
type Controller struct {
	Paths  platform.Paths
	Runner *platform.Runner
	// InitScript is the Entware init script name (default S99xray) if present.
	InitScript string
}

// NewController returns a Controller with defaults.
func NewController(p platform.Paths, r *platform.Runner) *Controller {
	return &Controller{Paths: p, Runner: r, InitScript: filepath.Join(p.InitDir, "S99xray")}
}

// Installed reports whether an Xray binary is available.
func (c *Controller) Installed() bool {
	return platform.FileExists(c.Paths.XrayBin) || platform.Which("xray")
}

// ConfigPath is the generated config location.
func (c *Controller) ConfigPath() string {
	return filepath.Join(c.Paths.XrayConfDir, "config.json")
}

func (c *Controller) bin() string {
	if platform.FileExists(c.Paths.XrayBin) {
		return c.Paths.XrayBin
	}
	return "xray"
}

// Validate runs `xray -test` against a config file and returns an error if the
// config is invalid. This is the critical pre-apply safety gate.
//
// The explicit `-format json` matters: Xray infers a config's format from the
// file's extension, and our pre-apply temp file is written as
// "config.json.tmp" (see WriteConfig — the ".tmp" suffix keeps `xray run
// -confdir` from ever merging a half-written temp, since confdir only reads
// *.json/*.yaml/*.toml). Without the flag, `xray -test -config config.json.tmp`
// fails before it ever parses the body with "Failed to get format of …". Since
// keen-manager always emits JSON, we force the format so validation works
// regardless of the file's name.
func (c *Controller) Validate(configPath string) error {
	if !c.Installed() {
		return fmt.Errorf("xray binary not found")
	}
	res := c.Runner.Run(c.bin(), "-test", "-config", configPath, "-format", "json")
	if res.Err != nil {
		return fmt.Errorf("xray config invalid: %s", firstNonEmptyStr(res.Stderr, res.Stdout))
	}
	return nil
}

// WriteConfig writes the config to disk (creating a timestamped backup of any
// existing config first) and validates it. It does NOT restart Xray.
func (c *Controller) WriteConfig(cfg *Config) (string, error) {
	if err := os.MkdirAll(c.Paths.XrayConfDir, 0o755); err != nil {
		return "", err
	}
	data, err := Marshal(cfg)
	if err != nil {
		return "", err
	}
	path := c.ConfigPath()

	// Backup existing config.
	if platform.FileExists(path) {
		bak := filepath.Join(c.Paths.BackupDir, fmt.Sprintf("xray-config-%d.json", time.Now().Unix()))
		if old, rerr := os.ReadFile(path); rerr == nil {
			_ = os.MkdirAll(c.Paths.BackupDir, 0o755)
			_ = os.WriteFile(bak, old, 0o600)
		}
	}

	// Write to a temp file, validate, then atomically rename. The ".tmp" suffix
	// (rather than a second ".json") is deliberate: `xray run -confdir` only
	// merges *.json/*.yaml/*.toml, so a crash between write and rename can never
	// leave a partial config the daemon would load on its next start. Validate
	// forces `-format json` so the unrecognised extension doesn't trip Xray's
	// format detection. Clear any stale temp from a previously interrupted write.
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := c.Validate(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// Apply writes+validates the config and restarts Xray. Returns the config path.
func (c *Controller) Apply(cfg *Config) (string, error) {
	path, err := c.WriteConfig(cfg)
	if err != nil {
		return "", err
	}
	if err := c.Restart(); err != nil {
		return path, err
	}
	return path, nil
}

// Start/Stop/Restart drive the init script when present, else manage the process
// directly (foreground binary backgrounded by the OS init).
func (c *Controller) Start() error   { return c.service("start") }
func (c *Controller) Stop() error    { return c.service("stop") }
func (c *Controller) Restart() error { return c.service("restart") }

func (c *Controller) service(action string) error {
	if platform.FileExists(c.InitScript) {
		return c.Runner.MustRun(c.InitScript, action)
	}
	// Fallback: no init script. Best-effort direct control.
	switch action {
	case "stop", "restart":
		_ = c.Runner.Run("pkill", "-f", c.bin()+" run")
		if action == "stop" {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
		fallthrough
	case "start":
		res := c.Runner.Run(c.bin(), "run", "-confdir", c.Paths.XrayConfDir)
		return res.Err
	}
	return nil
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
