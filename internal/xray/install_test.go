package xray

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

func TestAssetNameAndDownloadURL(t *testing.T) {
	cases := []struct {
		arch  platform.Arch
		asset string
	}{
		{platform.ArchMIPSLE, "Xray-linux-mips32le.zip"},
		{platform.ArchMIPS, "Xray-linux-mips32.zip"},
		{platform.ArchARM64, "Xray-linux-arm64-v8a.zip"},
		{platform.ArchARM, "Xray-linux-arm32-v7a.zip"},
		{platform.ArchAMD64, "Xray-linux-64.zip"},
	}
	for _, c := range cases {
		got, err := assetName(c.arch)
		if err != nil || got != c.asset {
			t.Fatalf("assetName(%s) = %q, %v; want %q", c.arch, got, err, c.asset)
		}
		url, err := downloadURL("v25.1.30", c.arch)
		if err != nil {
			t.Fatalf("downloadURL(%s): %v", c.arch, err)
		}
		want := "https://github.com/XTLS/Xray-core/releases/download/v25.1.30/" + c.asset
		if url != want {
			t.Fatalf("downloadURL(%s) = %q; want %q", c.arch, url, want)
		}
	}

	if _, err := assetName(platform.ArchUnknown); err == nil {
		t.Fatal("expected an error for an unknown architecture")
	}

	// A version without a leading "v" must be normalized.
	url, _ := downloadURL("25.1.30", platform.ArchAMD64)
	if !strings.Contains(url, "/download/v25.1.30/") {
		t.Fatalf("version not normalized in URL: %s", url)
	}
}

func TestInstallSource(t *testing.T) {
	// Default: the official GitHub release URL for the arch.
	t.Setenv("KEEN_XRAY_URL", "")
	got, err := installSource("v25.1.30", platform.ArchARM64)
	if err != nil || !strings.HasSuffix(got, "/download/v25.1.30/Xray-linux-arm64-v8a.zip") {
		t.Fatalf("default source = %q, %v", got, err)
	}
	// KEEN_XRAY_URL wins — the offline / DPI escape hatch (may be file://).
	t.Setenv("KEEN_XRAY_URL", "file:///opt/tmp/xray-arm64.zip")
	got, err = installSource("v25.1.30", platform.ArchARM64)
	if err != nil || got != "file:///opt/tmp/xray-arm64.zip" {
		t.Fatalf("override source = %q, %v", got, err)
	}
}

func TestExtractXrayBinary(t *testing.T) {
	// A raw ELF payload (KEEN_XRAY_URL pointing straight at the binary) passes
	// through untouched.
	raw := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("the-binary-body")...)
	got, err := extractXrayBinary(raw)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("raw ELF passthrough: got %d bytes, err=%v", len(got), err)
	}

	// A release .zip carrying an "xray" entry is unpacked.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xray")
	body := []byte("\x7fELF-zip-body")
	_, _ = w.Write(body)
	other, _ := zw.Create("README.md")
	_, _ = other.Write([]byte("ignore me"))
	_ = zw.Close()
	got, err = extractXrayBinary(buf.Bytes())
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("zip extract: got %q, err=%v", got, err)
	}

	// Neither an ELF nor a zip → a clear error, not a silent wrong binary.
	if _, err := extractXrayBinary([]byte("<html>blocked by DPI</html>")); err == nil {
		t.Fatal("expected an error for a non-ELF, non-zip payload")
	}
}

func TestUnusableBinaryReason(t *testing.T) {
	newCtrl := func(xrayBin string, dryRun bool) *Controller {
		r := platform.NewRunner()
		r.DryRun = dryRun
		return NewController(platform.Paths{XrayBin: xrayBin}, r)
	}

	// Dry-run / off-device: never probes, always "" (nothing to heal here).
	if reason := newCtrl("/opt/sbin/xray", true).unusableBinaryReason(); reason != "" {
		t.Fatalf("dry-run reason = %q, want empty", reason)
	}
	// Missing managed binary: nothing to heal (Install handles the fresh case).
	if reason := newCtrl(t.TempDir()+"/absent", false).unusableBinaryReason(); reason != "" {
		t.Fatalf("missing-binary reason = %q, want empty", reason)
	}
	// A non-ELF file (a corrupt/partial download) is flagged without executing.
	notELF := t.TempDir() + "/xray"
	if err := os.WriteFile(notELF, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if reason := newCtrl(notELF, false).unusableBinaryReason(); !strings.Contains(reason, "not a valid executable") {
		t.Fatalf("non-ELF reason = %q, want 'not a valid executable'", reason)
	}
	// A wrong-architecture ELF (arm64 header on the amd64 test host) is caught
	// by the header check alone — the exact case behind the user's report.
	wrongArch := t.TempDir() + "/xray"
	arm64ELF := make([]byte, 20)
	copy(arm64ELF, []byte{0x7f, 'E', 'L', 'F', 2, 1})
	arm64ELF[18] = 0xb7 // EM_AARCH64, little-endian
	if err := os.WriteFile(wrongArch, arm64ELF, 0o755); err != nil {
		t.Fatal(err)
	}
	if reason := newCtrl(wrongArch, false).unusableBinaryReason(); !strings.Contains(reason, "arm64") {
		t.Fatalf("wrong-arch reason = %q, want it to mention arm64", reason)
	}
}

func TestInitScript(t *testing.T) {
	s := InitScript("/opt/sbin/xray", "/opt/etc/keen-manager/xray", "/var/run/xray.pid", "/opt/var/log/xray.log")
	for _, want := range []string{
		"run -confdir",
		"/opt/etc/keen-manager/xray",
		"/opt/sbin/xray",
		"start)", "stop)", "restart)",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("init script missing %q:\n%s", want, s)
		}
	}
}
