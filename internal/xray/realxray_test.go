package xray

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// TestRealXrayValidatesGeneratedConfigs runs a real `xray -test` over configs
// BuildConfig produces for every supported profile, so a future change that
// emits something modern Xray-core rejects (the class of bug behind "xray
// config invalid") fails here loudly instead of only on the router.
//
// It is skipped unless KEEN_TEST_XRAY_BIN points at an xray binary, so the
// normal `go test ./...` stays hermetic:
//
//	KEEN_TEST_XRAY_BIN=/path/to/xray go test ./internal/xray/ -run RealXray -v
func TestRealXrayValidatesGeneratedConfigs(t *testing.T) {
	bin := os.Getenv("KEEN_TEST_XRAY_BIN")
	if bin == "" {
		t.Skip("set KEEN_TEST_XRAY_BIN to a real xray binary to run this test")
	}
	key, err := base64.RawURLEncoding.DecodeString(canonicalPBK)
	if err != nil {
		t.Fatalf("bad canonical key: %v", err)
	}
	stdKey := base64.StdEncoding.EncodeToString(key) // + / and =: the broken shape

	servers := map[string]model.Server{
		"vless-reality-vision": {
			Protocol: model.ProtoVLESS, Address: "sv.example.com", Port: 443,
			UUID: "11111111-1111-1111-1111-111111111111", Flow: "xtls-rprx-vision",
			Security: "reality", Network: "tcp", SNI: "www.microsoft.com",
			Fingerprint: "chrome", PublicKey: canonicalPBK, ShortID: "0123abcd",
		},
		"vless-reality-stdkey": { // the exact case that produced "invalid password"
			Protocol: model.ProtoVLESS, Address: "de.example.com", Port: 443,
			UUID: "22222222-2222-2222-2222-222222222222", Flow: "xtls-rprx-vision",
			Security: "reality", Network: "tcp", SNI: "www.google.com",
			PublicKey: stdKey, ShortID: "abcd",
		},
		"vless-ws-tls": {
			Protocol: model.ProtoVLESS, Address: "nl.example.com", Port: 443,
			UUID: "33333333-3333-3333-3333-333333333333", Security: "tls",
			Network: "ws", SNI: "cdn.example.com", Host: "cdn.example.com", Path: "/v",
		},
		"vmess-ws": {
			Protocol: model.ProtoVMess, Address: "vm.example.com", Port: 443,
			UUID: "44444444-4444-4444-4444-444444444444", Cipher: "auto",
			Security: "tls", Network: "ws", Host: "vm.example.com", Path: "/vm",
		},
		"trojan-tls": {
			Protocol: model.ProtoTrojan, Address: "tr.example.com", Port: 443,
			Password: "trojanpass", Security: "tls", Network: "tcp", SNI: "tr.example.com",
		},
		"shadowsocks": {
			Protocol: model.ProtoSS, Address: "ss.example.com", Port: 8388,
			Password: "sspassword", Cipher: "aes-256-gcm", Network: "tcp", Security: "none",
		},
	}

	dir := t.TempDir()
	for name, s := range servers {
		for _, proxyMode := range []bool{false, true} {
			mode := "tproxy"
			if proxyMode {
				mode = "proxyconn"
			}
			t.Run(name+"/"+mode, func(t *testing.T) {
				opts := Defaults()
				opts.LogError = filepath.Join(dir, "err.log")
				if proxyMode {
					opts.ProxyConnMode = true
				} else {
					opts.EnableTProxy = true
				}
				cfg, err := BuildConfig([]model.Server{s}, opts)
				if err != nil {
					t.Fatalf("BuildConfig: %v", err)
				}
				data, err := Marshal(cfg)
				if err != nil {
					t.Fatalf("Marshal: %v", err)
				}
				p := filepath.Join(dir, name+"-"+mode+".json")
				if err := os.WriteFile(p, data, 0o644); err != nil {
					t.Fatal(err)
				}
				out, err := exec.Command(bin, "-test", "-config", p, "-format", "json").CombinedOutput()
				if err != nil {
					tail := string(out)
					if i := strings.LastIndex(tail, " > "); i >= 0 {
						tail = tail[i+3:]
					}
					t.Fatalf("xray -test rejected %s/%s: %s", name, mode, strings.TrimSpace(tail))
				}
			})
		}
	}
}
