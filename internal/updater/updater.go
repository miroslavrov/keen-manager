// Package updater implements self-update for the keen-manager binary: check
// GitHub releases for a newer version, download the matching arch asset,
// verify (gzip integrity + ELF arch), atomically replace the binary, and
// restart the service.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/version"
)

const repo = "miroslavrov/keen-manager"

// archToAsset maps keen-manager arch names to release asset suffixes.
func archToAsset(arch string) string {
	switch arch {
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	case "mipsle":
		return "mipsle"
	case "mips":
		return "mips"
	default:
		return ""
	}
}

// Release is the minimal GitHub release info we need.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckLatest fetches the list of releases from GitHub and returns the
// newest one by SemVer. We can't use /releases/latest because pre-release-only
// repos return 404 — so we list and max ourselves (like install.sh does).
func CheckLatest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	var best *Release
	for i := range releases {
		tag := releases[i].TagName
		if !strings.HasPrefix(tag, "v") {
			continue
		}
		if best == nil || semverCompare(tag, best.TagName) > 0 {
			best = &releases[i]
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no valid version tags found")
	}
	return best, nil
}

// IsNewer returns true if `candidate` (e.g. "v0.1.0-rc.10") is strictly newer
// than `current` (e.g. "v0.1.0-rc.9").
func IsNewer(current, candidate string) bool {
	return semverCompare(candidate, current) > 0
}

// semverCompare compares two SemVer-ish version strings (with optional leading
// 'v' and pre-release suffixes). Returns -1, 0, or 1.
// This is a pure-Go reimplementation of the installer's awk semver_max.
func semverCompare(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	// Split into release + prerelease
	aRel, aPre := splitPre(a)
	bRel, bPre := splitPre(b)
	// Compare release parts
	c := cmpRelease(aRel, bRel)
	if c != 0 {
		return c
	}
	// Same release: a version without prerelease is higher
	if aPre == "" && bPre != "" {
		return 1
	}
	if aPre != "" && bPre == "" {
		return -1
	}
	if aPre == "" && bPre == "" {
		return 0
	}
	return cmpPre(aPre, bPre)
}

func splitPre(v string) (rel, pre string) {
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		return v[:idx], v[idx+1:]
	}
	return v, ""
}

func cmpRelease(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	max := len(ap)
	if len(bp) > max {
		max = len(bp)
	}
	for i := 0; i < max; i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = atoiSafe(ap[i])
		}
		if i < len(bp) {
			bv = atoiSafe(bp[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func cmpPre(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	max := len(ap)
	if len(bp) > max {
		max = len(bp)
	}
	for i := 0; i < max; i++ {
		if i >= len(ap) {
			return -1
		}
		if i >= len(bp) {
			return 1
		}
		av, bv := ap[i], bp[i]
		if isNumeric(av) && isNumeric(bv) {
			an, bn := atoiSafe(av), atoiSafe(bv)
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		} else {
			if av < bv {
				return -1
			}
			if av > bv {
				return 1
			}
		}
	}
	return 0
}

// SelfUpdate checks for a newer release, downloads it, verifies, atomically
// replaces the running binary, and restarts the service. Returns a message
// describing what happened.
func SelfUpdate(ctx context.Context, force bool) (string, error) {
	current := version.Short()
	rel, err := CheckLatest(ctx)
	if err != nil {
		return "", fmt.Errorf("check latest: %w", err)
	}
	if !force && !IsNewer(current, rel.TagName) {
		return fmt.Sprintf("already up to date (%s)", current), nil
	}

	// Find the matching asset.
	arch := string(platform.DetectArch())
	// On a dev box, use runtime arch mapping.
	if arch == "" || archToAsset(arch) == "" {
		arch = runtimeArch()
	}
	suffix := archToAsset(arch)
	if suffix == "" {
		return "", fmt.Errorf("unsupported architecture %q", arch)
	}
	assetName := fmt.Sprintf("keen-manager-%s.gz", suffix)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("asset %s not found in release %s", assetName, rel.TagName)
	}

	// Download.
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "keen-manager")
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
	if err != nil {
		return "", fmt.Errorf("read download: %w", err)
	}

	// Decompress gzip.
	binary, err := gunzip(data)
	if err != nil {
		return "", fmt.Errorf("decompress: %w", err)
	}

	// Verify ELF arch matches the device (only on-device, not on a dev PC).
	if runtime.GOOS == "linux" {
		elfArch, isELF := platform.ELFArchBytes(binary)
		if isELF && elfArch != platform.ArchUnknown {
			deviceArch := platform.DetectArch()
			if deviceArch != platform.ArchUnknown && elfArch != deviceArch {
				return "", fmt.Errorf("downloaded binary is %s but device is %s", elfArch, deviceArch)
			}
		}
	}

	// Atomic replace: write .tmp → rename.
	binPath, err := os.Executable()
	if err != nil {
		binPath = "/opt/bin/keen-manager"
	}
	tmpPath := binPath + ".tmp"
	if err := os.WriteFile(tmpPath, binary, 0755); err != nil {
		return "", fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("replace binary: %w", err)
	}

	// Restart the service via init script.
	initScript := "/opt/etc/init.d/S99keen-manager"
	if platform.FileExists(initScript) {
		_ = exec.Command(initScript, "restart").Start()
	}

	return fmt.Sprintf("updated %s → %s, restarting service", current, rel.TagName), nil
}

func runtimeArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	case "mipsle":
		return "mipsle"
	case "mips":
		return "mips"
	default:
		return ""
	}
}
