package xray

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	// Logf, when set, receives human-readable lifecycle messages (e.g. an
	// automatic binary reinstall). Optional; nil disables such logging.
	Logf func(format string, args ...any)
}

// logf emits a lifecycle message when a logger is wired, else drops it.
func (c *Controller) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
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

// ErrorLogPath is where the generated config tells Xray to write its own error
// log (warning/info/debug lines). keen-manager owns this path so it can tail the
// tunnel's real failure reason on a failed activation. Kept under the managed
// Xray config dir (not the shared /opt/var/log) so it is scoped to keen-manager.
func (c *Controller) ErrorLogPath() string {
	return filepath.Join(c.Paths.XrayConfDir, "xray-error.log")
}

// AccessLogPath is where Xray writes its access log when access logging is on.
func (c *Controller) AccessLogPath() string {
	return filepath.Join(c.Paths.XrayConfDir, "xray-access.log")
}

// LogTail returns the last maxLines non-empty lines of Xray's own error log, or
// "" when the log is absent/empty. It is used to surface WHY a bring-up failed
// (a dial reset, an i/o timeout, a REALITY mismatch) instead of the generic
// "tunnel did not carry traffic". Best-effort: any read error yields "".
func (c *Controller) LogTail(maxLines int) string {
	if maxLines <= 0 {
		maxLines = 12
	}
	data, err := os.ReadFile(c.ErrorLogPath())
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	out := make([]string, 0, maxLines)
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			out = append(out, strings.TrimSpace(ln))
		}
	}
	if len(out) > maxLines {
		out = out[len(out)-maxLines:]
	}
	return strings.Join(out, "\n")
}

// TruncateErrorLog clears the Xray error log so the next bring-up's log tail
// reflects only that attempt (best-effort; ignores errors).
func (c *Controller) TruncateErrorLog() {
	_ = os.WriteFile(c.ErrorLogPath(), nil, 0o600)
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
	if res.Err == nil {
		return nil
	}
	// A binary that cannot be executed at all (wrong CPU architecture, corrupt,
	// or not +x) produces no `-test` output to distil — just an opaque
	// "fork/exec …: exec format error". Rephrase it into something the operator
	// can act on. keen-manager auto-heals this on the next activation (Ensure
	// reinstalls the correct build), but a DPI-blocked router may need the
	// manual/offline path, so the message points there.
	if isExecFormatError(res.Err) {
		return fmt.Errorf("xray config invalid: %s", c.execFormatDetail())
	}
	// Xray writes a failed `-test` to stdout (it dies before the app logger, and
	// before honouring the config's own log.error file), so prefer stdout and
	// distil the wall of banner+trace down to the innermost cause. Fall back to
	// the exec error itself (e.g. a timeout that produced no output) so the
	// reason is never blank.
	detail := distillXrayTestError(firstNonEmptyStr(res.Stdout, res.Stderr))
	if detail == "" {
		detail = res.Err.Error()
	}
	return fmt.Errorf("xray config invalid: %s", detail)
}

// isExecFormatError reports whether err is the OS refusing to execute the
// binary (ENOEXEC "exec format error" / EACCES) rather than a config problem
// Xray itself reported. It matches both the wrapped syscall errno and the
// textual form, so it holds regardless of how the runner surfaced it.
func isExecFormatError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOEXEC) || errors.Is(err, syscall.EACCES) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "exec format error") ||
		strings.Contains(msg, "permission denied")
}

// execFormatDetail explains, actionably, why the binary would not execute —
// naming the mismatched architectures when the ELF header reveals them.
func (c *Controller) execFormatDetail() string {
	path := c.bin()
	const remedy = "keen-manager reinstalls the correct build automatically on the next activation; if your ISP blocks GitHub, put a matching xray in place and set KEEN_XRAY_URL (see README)"
	device := platform.DetectArch()
	if got, isELF := platform.ELFArch(path); isELF && got != platform.ArchUnknown &&
		device != platform.ArchUnknown && got != device {
		return fmt.Sprintf("the xray binary at %s is built for %s but this router is %s — it cannot run here (exec format error). %s", path, got, device, remedy)
	}
	return fmt.Sprintf("the xray binary at %s cannot be executed on this router (exec format error) — it is the wrong CPU architecture or corrupt. %s", path, remedy)
}

// distillXrayTestError reduces the multi-line output of a failed `xray -test`
// to one salient line. Xray prints a version banner and an "[Info] Reading
// config" line before the actual error, and formats nested causes as
// "a > b > c"; we drop the noise and keep the innermost cause (c), e.g.
// `infra/conf: invalid "password": <key>`. Pure, so it is unit-tested.
func distillXrayTestError(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	cand := ""
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		low := strings.ToLower(ln)
		if strings.Contains(low, "penetrates everything") ||
			strings.Contains(low, "anti-censorship") ||
			strings.Contains(low, "reading config") ||
			strings.Contains(low, "[info]") ||
			strings.Contains(low, "[debug]") {
			continue
		}
		cand = ln // keep the last meaningful line
	}
	if cand == "" {
		return ""
	}
	if i := strings.LastIndex(cand, " > "); i >= 0 {
		cand = strings.TrimSpace(cand[i+3:])
	}
	return cand
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
