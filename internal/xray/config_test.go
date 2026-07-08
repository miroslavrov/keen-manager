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
