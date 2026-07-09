package keenetic

import "testing"

func TestPickWANInterface(t *testing.T) {
	eth := InterfaceInfo{Name: "GigabitEthernet1", SysName: "eth3", Type: "GigabitEthernet", SecurityLevel: "public", Connected: true, Up: true, Priority: 700}
	ppp := InterfaceInfo{Name: "PPPoE0", SysName: "ppp0", Type: "PPPoE", SecurityLevel: "public", Connected: false, Up: false, Priority: 900}
	lan := InterfaceInfo{Name: "Bridge0", SysName: "br0", Type: "Bridge", SecurityLevel: "private", Connected: true, Up: true}
	wg := InterfaceInfo{Name: "Wireguard0", Type: "Wireguard", SecurityLevel: "public", Connected: true, Up: true, IsWireguard: true}
	proxy := InterfaceInfo{Name: "Proxy0", Type: "Proxy", SecurityLevel: "public", Connected: true, Up: true, IsProxy: true}

	t.Run("single public ethernet is picked", func(t *testing.T) {
		got, ok := PickWANInterface([]InterfaceInfo{lan, eth})
		if !ok || got.Name != "GigabitEthernet1" {
			t.Fatalf("got %q, ok=%v", got.Name, ok)
		}
	})

	t.Run("tunnels and LAN are excluded", func(t *testing.T) {
		if _, ok := PickWANInterface([]InterfaceInfo{lan, wg, proxy}); ok {
			t.Fatal("expected no WAN among LAN + tunnels")
		}
	})

	t.Run("connected beats higher-priority-but-down", func(t *testing.T) {
		// ppp has higher priority (900) but is down; eth is connected → eth wins.
		got, ok := PickWANInterface([]InterfaceInfo{ppp, eth})
		if !ok || got.Name != "GigabitEthernet1" {
			t.Fatalf("got %q, ok=%v", got.Name, ok)
		}
	})

	t.Run("among connected, higher priority wins", func(t *testing.T) {
		a := InterfaceInfo{Name: "WAN_A", SecurityLevel: "public", Connected: true, Up: true, Priority: 100}
		b := InterfaceInfo{Name: "WAN_B", SecurityLevel: "public", Connected: true, Up: true, Priority: 800}
		got, _ := PickWANInterface([]InterfaceInfo{a, b})
		if got.Name != "WAN_B" {
			t.Fatalf("expected higher-priority WAN_B, got %q", got.Name)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, ok := PickWANInterface(nil); ok {
			t.Fatal("expected false for empty input")
		}
	})
}
