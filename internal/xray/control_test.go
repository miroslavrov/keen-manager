package xray

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
)

// fakeXray writes a stub binary so Controller.Installed() reports true without a
// real xray on the box. The Runner is in dry-run, so it is never executed.
func fakeXray(t *testing.T) (platform.Paths, *[]string, *platform.Runner) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var cmds []string
	r := &platform.Runner{DryRun: true, Log: func(c string) { cmds = append(cmds, c) }}
	p := platform.Paths{
		XrayBin:     bin,
		XrayConfDir: filepath.Join(dir, "conf"),
		BackupDir:   filepath.Join(dir, "backups"),
	}
	return p, &cmds, r
}

// TestValidateForcesJSONFormat guards the session-7 fix for the on-device error
// "Failed to get format of …/config.json.tmp": Xray picks a config's format from
// its extension, so validation MUST pass an explicit `-format json` for the
// unrecognised ".tmp" temp file.
func TestValidateForcesJSONFormat(t *testing.T) {
	p, cmds, r := fakeXray(t)
	c := NewController(p, r)

	tmp := c.ConfigPath() + ".tmp"
	if err := c.Validate(tmp); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if len(*cmds) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(*cmds), *cmds)
	}
	got := (*cmds)[0]
	for _, want := range []string{"-test", "-config", tmp, "-format json"} {
		if !strings.Contains(got, want) {
			t.Errorf("validate command missing %q:\n%s", want, got)
		}
	}
}

// TestWriteConfigAtomicRename confirms WriteConfig validates a ".tmp" file and
// leaves only the final config.json (never a ".json" temp that `xray run
// -confdir` would merge).
func TestWriteConfigAtomicRename(t *testing.T) {
	p, cmds, r := fakeXray(t)
	c := NewController(p, r)

	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	path, err := c.WriteConfig(cfg)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	if path != c.ConfigPath() {
		t.Errorf("path = %q, want %q", path, c.ConfigPath())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("final config.json missing: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file should be gone, stat err = %v", err)
	}
	// The validation ran against the ".tmp" file with the forced JSON format.
	if len(*cmds) == 0 || !strings.Contains((*cmds)[0], "-format json") {
		t.Errorf("expected a -format json validate command, got %v", *cmds)
	}
}

// TestIsExecFormatError covers the classifier that separates an OS-level
// "can't run this binary" failure (wrong CPU arch / corrupt / not +x) from a
// config error Xray itself reported — so the former gets a legible message
// instead of a raw "fork/exec …: exec format error".
func TestIsExecFormatError(t *testing.T) {
	yes := []error{
		syscall.ENOEXEC,
		syscall.EACCES,
		fmt.Errorf("fork/exec /opt/sbin/xray: %w", syscall.ENOEXEC),
		errors.New("fork/exec /opt/sbin/xray: exec format error"),
		errors.New("fork/exec /opt/sbin/xray: permission denied"),
	}
	for _, e := range yes {
		if !isExecFormatError(e) {
			t.Errorf("isExecFormatError(%v) = false, want true", e)
		}
	}
	no := []error{
		nil,
		errors.New("context deadline exceeded"),
		errors.New(`infra/conf: invalid "password"`),
	}
	for _, e := range no {
		if isExecFormatError(e) {
			t.Errorf("isExecFormatError(%v) = true, want false", e)
		}
	}
}

// TestExecFormatDetail confirms the operator-facing message names the arch
// mismatch (when the ELF header reveals it) and always points at the remedy.
func TestExecFormatDetail(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "xray")
	arm64ELF := make([]byte, 20)
	copy(arm64ELF, []byte{0x7f, 'E', 'L', 'F', 2, 1})
	arm64ELF[18] = 0xb7 // EM_AARCH64
	if err := os.WriteFile(bin, arm64ELF, 0o755); err != nil {
		t.Fatal(err)
	}
	c := NewController(platform.Paths{XrayBin: bin}, &platform.Runner{DryRun: true})
	detail := c.execFormatDetail()
	if !strings.Contains(detail, "arm64") || !strings.Contains(detail, "KEEN_XRAY_URL") {
		t.Fatalf("execFormatDetail = %q, want it to name arm64 and the KEEN_XRAY_URL remedy", detail)
	}
}

// TestDistillXrayTestError guards the session-18 fix that made "xray config
// invalid" legible: a failed `xray -test` prints a version banner + an
// "[Info] Reading config" line before the real cause, and nests causes as
// "a > b > c". We must strip the noise and surface the innermost cause.
func TestDistillXrayTestError(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "reality-invalid-password",
			in: "Xray 26.3.27 (Xray, Penetrates Everything.) d2758a0 (go1.26.1 linux/amd64)\n" +
				"A unified platform for anti-censorship.\n" +
				"2026/07/11 00:39:01.596149 [Info] infra/conf/serial: Reading config: &{Name:/x.json Format:json}\n" +
				"Failed to start: main: failed to load config files: [/x.json] > infra/conf: failed to build outbound config with tag srv > infra/conf: Failed to build REALITY config. > infra/conf: invalid \"password\": z9foAieCPO2/M0Q=",
			want: "infra/conf: invalid \"password\": z9foAieCPO2/M0Q=",
		},
		{
			name: "single-line",
			in:   "infra/conf: invalid port",
			want: "infra/conf: invalid port",
		},
		{name: "empty", in: "", want: ""},
		{name: "banner-only", in: "Xray 26.3.27 (Xray, Penetrates Everything.) abc\nA unified platform for anti-censorship.", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := distillXrayTestError(tc.in); got != tc.want {
				t.Errorf("distillXrayTestError()\n got=%q\nwant=%q", got, tc.want)
			}
		})
	}
}
