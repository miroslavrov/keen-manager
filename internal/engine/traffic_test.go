package engine

import "testing"

func TestParseProcNetDev(t *testing.T) {
	// Real /proc/net/dev shape: two header lines, then "  name: rx... tx...".
	sample := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  123456     789    0    0    0     0          0         0   123456     789    0    0    0     0       0          0
  eth3: 1000000    5000    0    0    0     0          0         0   250000    3000    0    0    0     0       0          0
  nwg0:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
`
	got := parseProcNetDev(sample)
	if len(got) != 3 {
		t.Fatalf("expected 3 interfaces, got %d: %+v", len(got), got)
	}
	byName := map[string]IfaceTrafficView{}
	for _, i := range got {
		byName[i.Name] = i
	}
	if e := byName["eth3"]; e.RxBytes != 1000000 || e.TxBytes != 250000 {
		t.Errorf("eth3 rx/tx = %d/%d, want 1000000/250000", e.RxBytes, e.TxBytes)
	}
	if l := byName["lo"]; l.RxBytes != 123456 || l.TxBytes != 123456 {
		t.Errorf("lo rx/tx = %d/%d, want 123456/123456", l.RxBytes, l.TxBytes)
	}
	if _, ok := byName["nwg0"]; !ok {
		t.Error("nwg0 (zero counters) should still be listed")
	}
}

func TestParseProcNetDevGarbage(t *testing.T) {
	if got := parseProcNetDev(""); len(got) != 0 {
		t.Errorf("empty input -> %d rows, want 0", len(got))
	}
	// A colon line with too few fields must be skipped, not panic.
	if got := parseProcNetDev("weird: 1 2 3\n"); len(got) != 0 {
		t.Errorf("short row -> %d rows, want 0", len(got))
	}
}
