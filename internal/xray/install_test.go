package xray

import (
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
