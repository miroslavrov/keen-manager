package keenetic

import (
	"context"
	"net/http"
	"testing"
)

func TestProxyInterfaceNameAndParse(t *testing.T) {
	if got := ProxyInterfaceName(0); got != "Proxy0" {
		t.Errorf("ProxyInterfaceName(0) = %q, want Proxy0", got)
	}
	if got := ProxyInterfaceName(7); got != "Proxy7" {
		t.Errorf("ProxyInterfaceName(7) = %q, want Proxy7", got)
	}

	cases := map[string]struct {
		n  int
		ok bool
	}{
		"Proxy0":     {0, true},
		"Proxy12":    {12, true},
		"proxy3":     {3, true}, // case-insensitive prefix
		"Proxy":      {0, false},
		"ProxyX":     {0, false},
		"Wireguard0": {0, false},
		"":           {0, false},
	}
	for name, want := range cases {
		n, ok := parseProxyIndex(name)
		if ok != want.ok || (ok && n != want.n) {
			t.Errorf("parseProxyIndex(%q) = (%d,%v), want (%d,%v)", name, n, ok, want.n, want.ok)
		}
	}
}

func TestFindFreeProxyIndex(t *testing.T) {
	// Listing already has Proxy0 and Proxy2 (plus unrelated interfaces); the
	// first free index is 1.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"Proxy0": {"id":"Proxy0","type":"Proxy"},
			"Proxy2": {"id":"Proxy2","type":"Proxy"},
			"Wireguard0": {"id":"Wireguard0","type":"Wireguard"},
			"Bridge0": {"id":"Bridge0","type":"Bridge"}
		}`))
	})
	n, err := FindFreeProxyIndex(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("FindFreeProxyIndex = %d, want 1", n)
	}
}

// TestCreateProxyInterfaceBody asserts the RCI payload shape CreateProxyInterface
// sends, informed by the session-13 on-device read-back (see proxyiface.go's
// header). The critical anti-hijack fields — ip global off, ip name-servers off,
// and a LAN (non-"public") security zone — must be present so the managed proxy
// is a per-domain routing target, not the router's default internet uplink.
func TestCreateProxyInterfaceBody(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":[{"status":"message","message":"ok"}]}`))
	})

	err := CreateProxyInterface(context.Background(), c, "Proxy0", ProxyConfig{
		Upstream:      "127.0.0.1",
		Port:          10808,
		Protocol:      "socks5",
		SecurityLevel: "public",
		Description:   "keen-manager",
		Up:            true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := cs.lastBodyJSON(t)
	// interface.Proxy0.proxy.upstream.host == 127.0.0.1
	if got := dig(t, body, "interface", "Proxy0", "proxy", "upstream", "host"); got != "127.0.0.1" {
		t.Errorf("upstream host = %v, want 127.0.0.1", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "proxy", "upstream", "port"); got != float64(10808) {
		t.Errorf("upstream port = %v, want 10808", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "proxy", "protocol"); got != "socks5" {
		t.Errorf("protocol = %v, want socks5", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "proxy", "connect"); got != true {
		t.Errorf("connect = %v, want true", got)
	}
	// security-level is written in OBJECT form ({"public":true}); the bare-string
	// form is rejected by RCI with "no input" (confirmed on-device, session 13).
	if got := dig(t, body, "interface", "Proxy0", "security-level", "public"); got != true {
		t.Errorf("security-level.public = %v, want true (object form, not a bare string)", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "up"); got != true {
		t.Errorf("up = %v, want true", got)
	}
	// Anti-hijack invariant: never a default-route/internet-access uplink and
	// never a DNS source for the router (v4 AND v6).
	if got := dig(t, body, "interface", "Proxy0", "ip", "global"); got != false {
		t.Errorf("ip.global = %v, want false (must not join internet-access/default-route selection)", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "ip", "name-servers"); got != false {
		t.Errorf("ip.name-servers = %v, want false (must not route the router's v4 DNS through the proxy)", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "ipv6", "name-servers"); got != false {
		t.Errorf("ipv6.name-servers = %v, want false (must not route the router's v6 DNS through the proxy)", got)
	}
	// TCP-only by default: no socks5-udp key.
	proxy := dig(t, body, "interface", "Proxy0", "proxy").(map[string]any)
	if _, present := proxy["socks5-udp"]; present {
		t.Error("socks5-udp must be absent by default (TCP-only first cut)")
	}
}

// TestHardenProxyInterface asserts the heal-in-place payload used to fix an
// interface an earlier build left with the firmware's auto-assigned global
// (internet-access) priority. It must clear ip global + name-servers (v4 AND v6)
// WITHOUT touching the security zone (a proxy is legitimately public) and WITHOUT
// rewriting the proxy block (so the working connection + the dns-proxy routes
// bound to it survive).
func TestHardenProxyInterface(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":[{"status":"message","message":"ok"}]}`))
	})

	if err := HardenProxyInterface(context.Background(), c, "Proxy0"); err != nil {
		t.Fatal(err)
	}
	body := cs.lastBodyJSON(t)
	if got := dig(t, body, "interface", "Proxy0", "ip", "global"); got != false {
		t.Errorf("ip.global = %v, want false", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "ip", "name-servers"); got != false {
		t.Errorf("ip.name-servers = %v, want false", got)
	}
	if got := dig(t, body, "interface", "Proxy0", "ipv6", "name-servers"); got != false {
		t.Errorf("ipv6.name-servers = %v, want false", got)
	}
	iface := dig(t, body, "interface", "Proxy0").(map[string]any)
	// Hardening must NOT touch the security zone (leave it public) …
	if _, present := iface["security-level"]; present {
		t.Error("harden must not rewrite security-level (the zone is fine; only the priority was the bug)")
	}
	// … and must NOT rewrite the proxy upstream/protocol (leave the connection intact).
	if _, present := iface["proxy"]; present {
		t.Error("harden must not include a proxy block (it must not churn the upstream)")
	}
}

func TestCreateProxyInterface_RejectedIsError(t *testing.T) {
	// An RCI error envelope (HTTP 200 with a status:error) must surface as an
	// error so the engine can fall back to TPROXY.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"interface":{"status":[{"status":"error","message":"unknown command proxy"}]}}`))
	})
	err := CreateProxyInterface(context.Background(), c, "Proxy0", ProxyConfig{Upstream: "127.0.0.1", Port: 10808, Up: true})
	if err == nil {
		t.Fatal("expected an error when RCI rejects the proxy interface create")
	}
}

func TestListInterfacesRecognisesProxy(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"Proxy0": {"id":"Proxy0","type":"Proxy","description":"keen-manager","state":"up","connected":"yes"},
			"Wireguard0": {"id":"Wireguard0","type":"Wireguard","state":"up"}
		}`))
	})
	infos, err := ListInterfaces(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	var proxy *InterfaceInfo
	for i := range infos {
		if infos[i].Name == "Proxy0" {
			proxy = &infos[i]
		}
	}
	if proxy == nil {
		t.Fatal("Proxy0 missing from listing")
	}
	if !proxy.IsProxy {
		t.Error("Proxy0 should be flagged IsProxy")
	}
	if proxy.IsWireguard {
		t.Error("Proxy0 must not be flagged IsWireguard")
	}
}

func TestDetectCapabilitiesProxyClient(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"release":"5.01.C.0.0-1","ndw":{"components":"wireguard,proxy,schedule"}}`))
	})
	caps, err := DetectCapabilities(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if !caps.HasProxyClient {
		t.Error("expected HasProxyClient=true when a 'proxy' component is present")
	}
	if !caps.HasWireguard {
		t.Error("expected HasWireguard=true")
	}

	// No proxy component → false.
	c2, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"release":"5.01.C.0.0-1","ndw":{"components":"wireguard,schedule"}}`))
	})
	caps2, err := DetectCapabilities(context.Background(), c2)
	if err != nil {
		t.Fatal(err)
	}
	if caps2.HasProxyClient {
		t.Error("expected HasProxyClient=false with no proxy component")
	}
}
