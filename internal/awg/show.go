package awg

import (
	"bufio"
	"strconv"
	"strings"
	"time"
)

// Health is the parsed live state of an AWG interface from `awg show`.
type Health struct {
	Interface       string
	Endpoint        string
	HandshakeAgeSec int  // seconds since latest handshake; -1 if never
	RxBytes         int64
	TxBytes         int64
	Up              bool // handshake within a healthy window
}

// ParseShow parses the human-readable output of `awg show [iface]`.
func ParseShow(out string) Health {
	h := Health{HandshakeAgeSec: -1}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "interface:"):
			h.Interface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		case strings.HasPrefix(line, "endpoint:"):
			h.Endpoint = strings.TrimSpace(strings.TrimPrefix(line, "endpoint:"))
		case strings.HasPrefix(line, "latest handshake:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "latest handshake:"))
			h.HandshakeAgeSec = parseHandshakeAge(v)
		case strings.HasPrefix(line, "transfer:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "transfer:"))
			h.RxBytes, h.TxBytes = parseTransfer(v)
		}
	}
	// A tunnel is considered up if it handshook within ~3 minutes.
	if h.HandshakeAgeSec >= 0 && h.HandshakeAgeSec <= 180 {
		h.Up = true
	}
	return h
}

// parseHandshakeAge turns "1 minute, 5 seconds ago" / "Now" / "never" into secs.
func parseHandshakeAge(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || strings.Contains(s, "never") {
		return -1
	}
	if strings.Contains(s, "now") {
		return 0
	}
	s = strings.TrimSuffix(s, " ago")
	total := 0
	for _, part := range strings.Split(s, ",") {
		f := strings.Fields(strings.TrimSpace(part))
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.Atoi(f[0])
		unit := f[1]
		switch {
		case strings.HasPrefix(unit, "day"):
			total += n * 86400
		case strings.HasPrefix(unit, "hour"):
			total += n * 3600
		case strings.HasPrefix(unit, "minute"):
			total += n * 60
		case strings.HasPrefix(unit, "second"):
			total += n
		}
	}
	return total
}

// parseTransfer turns "1.23 MiB received, 456 KiB sent" into (rx, tx) bytes.
func parseTransfer(s string) (rx, tx int64) {
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if strings.HasSuffix(part, "received") {
			rx = parseSize(strings.TrimSuffix(part, "received"))
		} else if strings.HasSuffix(part, "sent") {
			tx = parseSize(strings.TrimSuffix(part, "sent"))
		}
	}
	return
}

func parseSize(s string) int64 {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return 0
	}
	val, _ := strconv.ParseFloat(f[0], 64)
	mult := 1.0
	switch strings.ToUpper(f[1]) {
	case "B":
		mult = 1
	case "KIB":
		mult = 1 << 10
	case "MIB":
		mult = 1 << 20
	case "GIB":
		mult = 1 << 30
	case "TIB":
		mult = 1 << 40
	}
	return int64(val * mult)
}

// HandshakeTime returns the absolute time of the latest handshake, if known.
func (h Health) HandshakeTime() (time.Time, bool) {
	if h.HandshakeAgeSec < 0 {
		return time.Time{}, false
	}
	return time.Now().Add(-time.Duration(h.HandshakeAgeSec) * time.Second), true
}
