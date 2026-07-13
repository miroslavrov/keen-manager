package xray

import (
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

func sampleReality(id, addr string) model.Server {
	return model.Server{
		ID: id, Name: "node-" + id, Protocol: model.ProtoVLESS,
		Address: addr, Port: 443, UUID: "839d4028-2984-4e66-8e62-f4c127b52f49",
		Flow: "xtls-rprx-vision", Security: "reality", Network: "tcp",
		SNI: "cdn3-87.yahoo.com", Fingerprint: "firefox",
		PublicKey: "CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw", ShortID: "07ddc43269d197c0",
	}
}

func TestOutboundReality(t *testing.T) {
	ob, err := OutboundFor(sampleReality("a", "1.1.1.1"), "srv-a")
	if err != nil {
		t.Fatal(err)
	}
	if ob.StreamSettings.Security != "reality" || ob.StreamSettings.RealitySettings == nil {
		t.Fatal("expected reality settings")
	}
	if ob.StreamSettings.RealitySettings.PublicKey == "" {
		t.Error("missing publicKey")
	}
	if ob.StreamSettings.Sockopt == nil || ob.StreamSettings.Sockopt.Mark != 255 {
		t.Error("expected sockopt mark 255")
	}
	if !strings.Contains(string(ob.Settings), "xtls-rprx-vision") {
		t.Error("expected flow in settings")
	}
}

func TestBuildConfigMSSClampAndLog(t *testing.T) {
	servers := []model.Server{sampleReality("a", "1.1.1.1")}
	opts := Defaults()
	opts.TCPMaxSeg = 1360
	opts.LogError = "/opt/etc/keen-manager/xray/xray-error.log"
	opts.LogLevel = "debug"
	cfg, err := BuildConfig(servers, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log == nil || cfg.Log.Error != opts.LogError || cfg.Log.Loglevel != "debug" {
		t.Fatalf("log block = %+v, want error=%q loglevel=debug", cfg.Log, opts.LogError)
	}
	// The server outbound (first) must carry the MSS clamp; direct/block must not.
	so := cfg.Outbounds[0].StreamSettings.Sockopt
	if so == nil || so.TCPMaxSeg != 1360 {
		t.Fatalf("server outbound sockopt = %+v, want tcpMaxSeg 1360", so)
	}
	if so.Mark != 255 {
		t.Errorf("expected mark 255 preserved alongside the clamp, got %d", so.Mark)
	}
	data, _ := Marshal(cfg)
	if !strings.Contains(string(data), "\"tcpMaxSeg\": 1360") {
		t.Errorf("expected tcpMaxSeg in marshalled config:\n%s", data)
	}
}

func TestBuildConfigMSSClampProxyMode(t *testing.T) {
	opts := Defaults()
	opts.ProxyConnMode = true
	opts.TCPMaxSeg = 1400
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if so := cfg.Outbounds[0].StreamSettings.Sockopt; so == nil || so.TCPMaxSeg != 1400 {
		t.Fatalf("proxy-mode outbound sockopt = %+v, want tcpMaxSeg 1400", so)
	}
}

func TestBuildConfigNoMSSClampByDefault(t *testing.T) {
	// TCPMaxSeg unset (0) must leave the field omitted — no behaviour change for
	// callers that don't opt in.
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if so := cfg.Outbounds[0].StreamSettings.Sockopt; so != nil && so.TCPMaxSeg != 0 {
		t.Errorf("expected no tcpMaxSeg by default, got %d", so.TCPMaxSeg)
	}
	data, _ := Marshal(cfg)
	if strings.Contains(string(data), "tcpMaxSeg") {
		t.Errorf("tcpMaxSeg must be omitted when unset:\n%s", data)
	}
}

// TestTProxyInboundRouteOnly guards the session-17 canon fix: the transparent
// dokodemo-door inbound must sniff with routeOnly:true (like socks-in and
// XKeen), so the sniffed domain only informs routing and the dial target stays
// the original captured destination IP.
func TestTProxyInboundRouteOnly(t *testing.T) {
	opts := Defaults()
	opts.EnableTProxy = true
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var tproxy *Inbound
	for i := range cfg.Inbounds {
		if cfg.Inbounds[i].Protocol == "dokodemo-door" {
			tproxy = &cfg.Inbounds[i]
		}
	}
	if tproxy == nil {
		t.Fatal("expected a dokodemo-door (tproxy-in) inbound when EnableTProxy is set")
	}
	if tproxy.Sniffing == nil || !tproxy.Sniffing.Enabled {
		t.Fatal("tproxy-in must sniff")
	}
	if !tproxy.Sniffing.RouteOnly {
		t.Error("tproxy-in sniffing must set routeOnly:true (XKeen canon)")
	}
	// The marshalled config must actually carry the field.
	data, _ := Marshal(cfg)
	if !strings.Contains(string(data), "\"routeOnly\": true") {
		t.Errorf("expected routeOnly:true in marshalled tproxy inbound:\n%s", data)
	}
}

func TestBuildConfigBalancer(t *testing.T) {
	servers := []model.Server{sampleReality("a", "1.1.1.1"), sampleReality("b", "2.2.2.2")}
	opts := Defaults()
	opts.EnableBalancer = true
	cfg, err := BuildConfig(servers, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 4 { // 2 servers + direct + block
		t.Errorf("outbounds = %d, want 4", len(cfg.Outbounds))
	}
	if cfg.BurstObservatory == nil {
		t.Error("expected burstObservatory")
	}
	if len(cfg.Routing.Balancers) != 1 || cfg.Routing.Balancers[0].Strategy.Type != "leastPing" {
		t.Error("expected leastPing balancer")
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\"balancerTag\": \"auto\"") {
		t.Error("expected balancer rule in JSON")
	}
}

func TestBuildConfigSplitTunnel(t *testing.T) {
	opts := Defaults()
	// A plain domain must gain the "domain:" matcher; an already-prefixed one
	// must be preserved verbatim. A subnet must land in the rule's IP list.
	opts.SplitDomains = []string{"youtube.com", "domain:googlevideo.com"}
	opts.SplitSubnets = []string{"203.0.113.0/24"}
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, opts)
	if err != nil {
		t.Fatal(err)
	}

	var tunnelIdx, directIdx = -1, -1
	for i := range cfg.Routing.Rules {
		r := cfg.Routing.Rules[i]
		if r.OutboundTag == "srv-a" && (len(r.Domain) > 0 || len(r.IP) > 0) {
			tunnelIdx = i
		}
		if r.OutboundTag == "direct" {
			directIdx = i
		}
	}
	if tunnelIdx < 0 {
		t.Fatalf("expected a matched tunnel rule → srv-a, rules=%+v", cfg.Routing.Rules)
	}
	if directIdx < 0 {
		t.Fatal("expected a direct catch-all rule in split mode")
	}
	if tunnelIdx > directIdx {
		t.Errorf("tunnel rule must precede the direct catch-all (tunnel=%d direct=%d)", tunnelIdx, directIdx)
	}
	tr := cfg.Routing.Rules[tunnelIdx]
	joined := strings.Join(tr.Domain, ",")
	if !strings.Contains(joined, "domain:youtube.com") {
		t.Errorf("plain domain should map to domain:youtube.com, got %v", tr.Domain)
	}
	if strings.Count(joined, "domain:googlevideo.com") != 1 || strings.Contains(joined, "domain:domain:") {
		t.Errorf("already-prefixed matcher should be preserved once, got %v", tr.Domain)
	}
	if len(tr.IP) != 1 || tr.IP[0] != "203.0.113.0/24" {
		t.Errorf("subnet should land in rule IP, got %v", tr.IP)
	}
}

// TestAPIServicesMatchFeatures guards the session-7 "not all dependencies are
// resolved" crash: a pinned single-server config emits no observatory, so it
// must NOT advertise ObservatoryService; the balancer config emits a
// burstObservatory, so it may.
func TestAPIServicesMatchFeatures(t *testing.T) {
	has := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}

	// Single pinned server (what activation / select-best builds).
	single, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if single.API == nil {
		t.Fatal("expected an api block")
	}
	if has(single.API.Services, "ObservatoryService") {
		t.Errorf("single-server config must not advertise ObservatoryService (no observatory feature): %v", single.API.Services)
	}
	for _, must := range []string{"HandlerService", "StatsService", "RoutingService"} {
		if !has(single.API.Services, must) {
			t.Errorf("api.services missing %q: %v", must, single.API.Services)
		}
	}
	if single.BurstObservatory != nil {
		t.Error("single-server config should not carry a burstObservatory")
	}

	// Balancer over 2 servers: observatory present → service may be advertised.
	optsB := Defaults()
	optsB.EnableBalancer = true
	bal, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1"), sampleReality("b", "2.2.2.2")}, optsB)
	if err != nil {
		t.Fatal(err)
	}
	if bal.BurstObservatory == nil {
		t.Fatal("balancer config should carry a burstObservatory")
	}
	if !has(bal.API.Services, "ObservatoryService") {
		t.Errorf("balancer config should advertise ObservatoryService: %v", bal.API.Services)
	}
}

func TestBuildConfigFullTunnelHasNoDirectCatchAll(t *testing.T) {
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Routing.Rules {
		if r.OutboundTag == "direct" {
			t.Error("a full tunnel (no split domains/subnets) must not add a direct catch-all")
		}
	}
}

// TestBuildConfigProxyConnMode covers the SOCKS-only profile used when Xray is
// wired as a single KeeneticOS Proxy connection: exactly one SOCKS inbound (no
// tproxy dokodemo-door), the single server pinned, no observatory/balancer, no
// api block (so it can never hit "not all dependencies are resolved"), and no
// split/direct catch-all even when Split* would otherwise apply.
func TestBuildConfigProxyConnMode(t *testing.T) {
	opts := Defaults()
	opts.ProxyConnMode = true
	opts.EnableTProxy = true // must be ignored in proxy-conn mode
	opts.EnableBalancer = true
	opts.SplitDomains = []string{"youtube.com"} // must be ignored (router routes instead)
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Inbounds) != 1 {
		t.Fatalf("proxy-conn config must have exactly one inbound, got %d", len(cfg.Inbounds))
	}
	in := cfg.Inbounds[0]
	if in.Protocol != "socks" || in.Listen != "127.0.0.1" || in.Port != opts.SocksPort {
		t.Errorf("expected a loopback socks inbound on %d, got %+v", opts.SocksPort, in)
	}
	for _, in := range cfg.Inbounds {
		if in.Protocol == "dokodemo-door" {
			t.Error("proxy-conn config must not carry a tproxy inbound")
		}
	}
	if cfg.API != nil || cfg.Stats != nil {
		t.Error("proxy-conn config should omit the api/stats block entirely")
	}
	if cfg.Observatory != nil || cfg.BurstObservatory != nil {
		t.Error("proxy-conn config must not carry an observatory")
	}
	if len(cfg.Outbounds) != 3 { // server + direct + block
		t.Errorf("outbounds = %d, want 3 (server+direct+block)", len(cfg.Outbounds))
	}
	if cfg.Outbounds[0].Tag != "srv-a" {
		t.Errorf("first outbound should be the pinned server srv-a, got %q", cfg.Outbounds[0].Tag)
	}
	if len(cfg.Routing.Rules) != 1 || cfg.Routing.Rules[0].OutboundTag != "srv-a" {
		t.Errorf("expected a single socks-in→srv-a rule, got %+v", cfg.Routing.Rules)
	}
	for _, r := range cfg.Routing.Rules {
		if r.OutboundTag == "direct" {
			t.Error("proxy-conn mode routes at the router, so no in-Xray direct catch-all")
		}
	}
	if _, err := Marshal(cfg); err != nil {
		t.Fatalf("proxy-conn config must marshal: %v", err)
	}
}

func TestBuildConfigSinglePin(t *testing.T) {
	cfg, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	// No balancer -> a direct outboundTag rule to the single server.
	found := false
	for _, r := range cfg.Routing.Rules {
		if r.OutboundTag == "srv-a" {
			found = true
		}
	}
	if !found {
		t.Error("expected pinned outboundTag rule srv-a")
	}
}

// TestDirectBlockCarrySelfMark guards the rc.8 TPROXY blackhole fix: the direct
// (freedom) and block (blackhole) outbounds must carry SO_MARK == selfMark so
// traffic Xray routes to them egresses on a socket the capture chain excludes,
// instead of being re-captured/mis-routed and blackholed. Applies in both the
// full/TPROXY build and the proxy-conn build.
func TestDirectBlockCarrySelfMark(t *testing.T) {
	check := func(name string, cfg *Config) {
		for _, tag := range []string{"direct", "block"} {
			var found bool
			for _, ob := range cfg.Outbounds {
				if ob.Tag != tag {
					continue
				}
				found = true
				if ob.StreamSettings == nil || ob.StreamSettings.Sockopt == nil || ob.StreamSettings.Sockopt.Mark != selfMark {
					t.Errorf("%s: %s outbound must carry sockopt mark %d, got %+v", name, tag, selfMark, ob.StreamSettings)
				}
			}
			if !found {
				t.Errorf("%s: no %s outbound found", name, tag)
			}
		}
	}

	full, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	check("full", full)

	opts := Defaults()
	opts.ProxyConnMode = true
	pc, err := BuildConfig([]model.Server{sampleReality("a", "1.1.1.1")}, opts)
	if err != nil {
		t.Fatal(err)
	}
	check("proxy-conn", pc)
}
