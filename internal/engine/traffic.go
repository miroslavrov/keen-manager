package engine

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Traffic returns a snapshot of per-interface byte counters read from
// /proc/net/dev. It is read-only and safe on- and off-device (in the sandbox it
// reports the container's interfaces; if the file is unreadable the list is
// simply empty). The dashboard polls this and diffs successive snapshots to
// show live WAN throughput, matching the WAN row by StateView.Wan.Interface.
func (e *Engine) Traffic() TrafficView {
	v := TrafficView{At: time.Now().UTC().Format(time.RFC3339)}
	if b, err := os.ReadFile(procNetDev); err == nil {
		v.Interfaces = parseProcNetDev(string(b))
	}
	return v
}

// procNetDev is the kernel's per-interface counter file (var for tests).
var procNetDev = "/proc/net/dev"

// parseProcNetDev extracts cumulative rx/tx byte counters from /proc/net/dev.
// Each data line is "  iface: rxbytes rxpkts ... txbytes txpkts ...": the name
// precedes the colon, receive bytes is the first field after it and transmit
// bytes the ninth. Header lines (no colon) and malformed rows are skipped.
// Pure, so it is unit-tested without touching the filesystem.
func parseProcNetDev(s string) []IfaceTrafficView {
	var out []IfaceTrafficView
	for _, line := range strings.Split(s, "\n") {
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue // header / blank line
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			continue // not the expected 8 rx + 8 tx columns
		}
		rx, err1 := strconv.ParseInt(fields[0], 10, 64)
		tx, err2 := strconv.ParseInt(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, IfaceTrafficView{Name: name, RxBytes: rx, TxBytes: tx})
	}
	return out
}
