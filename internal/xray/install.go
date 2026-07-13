package xray

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

// XrayRepo is the upstream Xray-core GitHub repository.
const XrayRepo = "XTLS/Xray-core"

// FallbackXrayVersion is used only when the latest release tag cannot be
// resolved from the GitHub API (e.g. no internet during install). It can be
// overridden at install time with the KEEN_XRAY_VERSION environment variable.
const FallbackXrayVersion = "v25.1.30"

// assetName maps a device architecture to the Xray-core release .zip asset
// filename. The four router targets (mipsle/mips softfloat, arm64, arm) plus
// amd64 for dev/testing are supported; anything else is an error.
func assetName(arch platform.Arch) (string, error) {
	switch arch {
	case platform.ArchMIPSLE:
		return "Xray-linux-mips32le.zip", nil
	case platform.ArchMIPS:
		return "Xray-linux-mips32.zip", nil
	case platform.ArchARM64:
		return "Xray-linux-arm64-v8a.zip", nil
	case platform.ArchARM:
		return "Xray-linux-arm32-v7a.zip", nil
	case platform.ArchAMD64:
		return "Xray-linux-64.zip", nil
	default:
		return "", fmt.Errorf("xray: no release asset for architecture %q", arch)
	}
}

// downloadURL builds the GitHub release asset URL for a version tag + arch.
func downloadURL(version string, arch platform.Arch) (string, error) {
	asset, err := assetName(arch)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", XrayRepo, version, asset), nil
}

// installSource returns where Install should fetch xray from. KEEN_XRAY_URL,
// when set, overrides the official GitHub release URL — the offline escape
// hatch for a router whose ISP DPI blocks GitHub's release-asset CDN. It may be
// an http(s):// or file:// URL pointing at either a release .zip or a raw xray
// binary (extractXrayBinary handles both).
func installSource(version string, arch platform.Arch) (string, error) {
	if u := strings.TrimSpace(os.Getenv("KEEN_XRAY_URL")); u != "" {
		return u, nil
	}
	return downloadURL(version, arch)
}

// fetchInstallBytes reads an install payload from a file:// path or downloads it
// over http(s)://, capped at 64 MiB.
func fetchInstallBytes(ctx context.Context, src string) ([]byte, error) {
	if rest, ok := strings.CutPrefix(src, "file://"); ok {
		data, err := os.ReadFile(rest)
		if err != nil {
			return nil, fmt.Errorf("xray: read %s: %w", src, err)
		}
		return data, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "keen-manager")
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xray: download %s: %w", src, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xray: download %s: HTTP %d", src, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MiB cap
	if err != nil {
		return nil, fmt.Errorf("xray: read download: %w", err)
	}
	return data, nil
}

// resolveVersion decides which Xray-core version to install: KEEN_XRAY_VERSION
// when set, else the latest release tag from the GitHub API, else the pinned
// FallbackXrayVersion.
func resolveVersion(ctx context.Context) string {
	if v := strings.TrimSpace(os.Getenv("KEEN_XRAY_VERSION")); v != "" {
		return v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+XrayRepo+"/releases/latest", nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "keen-manager")
		client := &http.Client{Timeout: 15 * time.Second}
		if resp, derr := client.Do(req); derr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var rel struct {
					TagName string `json:"tag_name"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil && rel.TagName != "" {
					return rel.TagName
				}
			}
		}
	}
	return FallbackXrayVersion
}

// Ensure installs xray-core (and its init script) if not already present, and
// heals a present-but-unusable binary. It is otherwise a no-op (it still ensures
// the init script) and does nothing in dry-run mode. Call it before applying an
// Xray config.
//
// A file existing at XrayBin is NOT proof it can run: a wrong-architecture or
// corrupt /opt/sbin/xray (left by an earlier install, copied from the wrong
// build, or a download the ISP's DPI cut short and that was saved anyway) still
// satisfies Installed(), yet every `xray -test` / `xray run` against it dies
// with "exec format error". Trusting mere existence is what stranded activation.
// So when a managed binary is present but unusable, reinstall the correct build
// in place rather than forking a binary we already know cannot run.
func (c *Controller) Ensure(ctx context.Context) error {
	if c.Installed() {
		if reason := c.unusableBinaryReason(); reason != "" {
			c.logf("xray: %s — reinstalling the correct build", reason)
			if err := c.Install(ctx, resolveVersion(ctx)); err != nil {
				return fmt.Errorf("%s, and reinstalling it failed: %w", reason, err)
			}
		}
		return c.ensureInitScript()
	}
	if c.Runner.DryRun {
		return nil
	}
	if err := c.Install(ctx, resolveVersion(ctx)); err != nil {
		return err
	}
	return c.ensureInitScript()
}

// unusableBinaryReason returns a human-readable reason the managed xray binary
// cannot run on this device, or "" when it looks fine. It first inspects the
// ELF header (no execution — safe on a wrong-arch binary) to catch the common
// "wrong CPU architecture" case with a precise message, then falls back to
// actually running `xray -version` to catch corruption a header check would
// miss. It returns "" in dry-run / off-device and when xray is only resolvable
// on PATH (not our managed file) — there is nothing we own to heal.
func (c *Controller) unusableBinaryReason() string {
	if c.Runner.DryRun || !platform.FileExists(c.Paths.XrayBin) {
		return ""
	}
	path := c.Paths.XrayBin
	device := platform.DetectArch()
	got, isELF := platform.ELFArch(path)
	switch {
	case !isELF:
		return fmt.Sprintf("the file at %s is not a valid executable (a corrupt or partial download, or the wrong kind of file)", path)
	case got != platform.ArchUnknown && device != platform.ArchUnknown && got != device:
		return fmt.Sprintf("the xray binary at %s is built for %s but this device is %s (exec format error)", path, got, device)
	}
	// Header looks compatible (or we could not tell) — confirm it truly runs.
	if res := c.Runner.Run(path, "-version"); res.Err != nil {
		return fmt.Sprintf("the xray binary at %s does not execute (%v)", path, res.Err)
	}
	return ""
}

// UnusableReason returns a human-readable reason if the managed xray binary
// cannot run on this device, or "" if it's fine. Public for the selftest command.
func (c *Controller) UnusableReason() string { return c.unusableBinaryReason() }

// Install downloads the given xray-core version for the detected architecture,
// extracts the xray binary to Paths.XrayBin (atomic write + chmod 0755), and
// verifies it runs. Network + filesystem effects; skipped by Ensure in dry-run.
//
// The source is the official GitHub release .zip unless KEEN_XRAY_URL is set,
// in which case that URL wins — the offline escape hatch for a router whose ISP
// DPI resets GitHub's release-asset CDN (the README documents the same file://
// trick for installing keen-manager itself). KEEN_XRAY_URL may point at a
// release .zip or straight at a raw xray binary, over http(s):// or file://.
func (c *Controller) Install(ctx context.Context, version string) error {
	arch := platform.DetectArch()
	src, err := installSource(version, arch)
	if err != nil {
		return err
	}

	data, err := fetchInstallBytes(ctx, src)
	if err != nil {
		return err
	}

	bin, err := extractXrayBinary(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Paths.XrayBin), 0o755); err != nil {
		return err
	}
	tmp := c.Paths.XrayBin + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("xray: write binary: %w", err)
	}
	if err := os.Rename(tmp, c.Paths.XrayBin); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("xray: install binary: %w", err)
	}
	_ = os.Chmod(c.Paths.XrayBin, 0o755)

	if res := c.Runner.Run(c.Paths.XrayBin, "-version"); res.Err != nil {
		return fmt.Errorf("xray: installed binary failed to run: %v", res.Err)
	}
	return nil
}

// extractXrayBinary returns the xray binary from an install payload. A raw ELF
// (a KEEN_XRAY_URL pointing straight at the binary) is used as-is; otherwise the
// payload is treated as a release .zip and its "xray" entry is extracted.
func extractXrayBinary(data []byte) ([]byte, error) {
	if isELFHeader(data) {
		return data, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("xray: open release zip: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "xray" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("xray: open zip entry: %w", err)
			}
			b, err := io.ReadAll(io.LimitReader(rc, 128<<20))
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("xray: read zip entry: %w", err)
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("xray: 'xray' binary not found in release archive")
}

// isELFHeader reports whether b starts with the ELF magic number.
func isELFHeader(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x7f && b[1] == 'E' && b[2] == 'L' && b[3] == 'F'
}

// ensureInitScript writes the Entware init script (S99xray) if absent, so Xray
// survives reboots and Start/Stop/Restart work through the standard path rather
// than the best-effort foreground fallback.
func (c *Controller) ensureInitScript() error {
	if platform.FileExists(c.InitScript) || c.Runner.DryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.InitScript), 0o755); err != nil {
		return err
	}
	script := InitScript(c.bin(), c.Paths.XrayConfDir, c.Paths.PidFile("xray"), c.Paths.LogFile("xray"))
	if err := os.WriteFile(c.InitScript, []byte(script), 0o755); err != nil {
		return err
	}
	_ = os.Chmod(c.InitScript, 0o755)
	return nil
}

// InitScript renders a minimal Entware/procd-style init script that daemonizes
// `xray run -confdir <dir>` with a pidfile and log file.
func InitScript(bin, confDir, pidFile, logFile string) string {
	return fmt.Sprintf(`#!/bin/sh
# keen-manager: xray-core service (auto-generated — safe to edit).
BIN="%s"
CONFDIR="%s"
PIDFILE="%s"
LOGFILE="%s"

start() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null; then
        return 0
    fi
    "$BIN" run -confdir "$CONFDIR" >>"$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
}
stop() {
    [ -f "$PIDFILE" ] && kill "$(cat "$PIDFILE" 2>/dev/null)" 2>/dev/null
    rm -f "$PIDFILE"
    pkill -f "$BIN run" 2>/dev/null
    return 0
}
case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    *) echo "usage: $0 {start|stop|restart}"; exit 1 ;;
esac
`, bin, confDir, pidFile, logFile)
}
