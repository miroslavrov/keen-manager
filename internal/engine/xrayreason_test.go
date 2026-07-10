package engine

import (
	"strings"
	"testing"
)

func TestNormalizeXrayMSS(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0},       // default → OFF (no clamp; XKeen never clamps the MSS)
		{-1, 0},      // negative → OFF
		{-999, 0},    // any negative → OFF
		{1360, 1360}, // explicit positive value passes through
		{1452, 1452},
	}
	for _, c := range cases {
		if got := normalizeXrayMSS(c.in); got != c.want {
			t.Errorf("normalizeXrayMSS(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNormalizeXrayLogLevel(t *testing.T) {
	cases := map[string]string{
		"":        "warning",
		"warning": "warning",
		"DEBUG":   "debug",
		" info ":  "info",
		"error":   "error",
		"none":    "none",
		"chatty":  "warning", // unknown → default
		"verbose": "warning",
	}
	for in, want := range cases {
		if got := normalizeXrayLogLevel(in); got != want {
			t.Errorf("normalizeXrayLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCondenseLogLine(t *testing.T) {
	in := "2026/07/10 11:12:13.456 [Warning] proxy/vless/outbound: failed to find an available destination > dial tcp 109.163.239.98:443: i/o timeout"
	got := condenseLogLine(in)
	if strings.HasPrefix(got, "2026/") {
		t.Errorf("timestamp not stripped: %q", got)
	}
	if !strings.Contains(got, "i/o timeout") {
		t.Errorf("expected the reason to survive: %q", got)
	}
	long := strings.Repeat("x", 400)
	if got := condenseLogLine(long); len([]rune(got)) > 241 {
		t.Errorf("long line not capped: len=%d", len([]rune(got)))
	}
}

func TestDistillXrayFailure(t *testing.T) {
	tail := strings.Join([]string{
		"2026/07/10 11:12:10 [Info] transport: something benign",
		"2026/07/10 11:12:11 [Info] app/proxyman: connection opened",
		"2026/07/10 11:12:12 [Warning] REALITY: processed invalid connection",
	}, "\n")
	got := distillXrayFailure(tail)
	if !strings.Contains(strings.ToLower(got), "reality") {
		t.Errorf("expected the REALITY line to be picked, got %q", got)
	}
	if distillXrayFailure("") != "" {
		t.Error("empty tail should distil to empty")
	}
	// No signature match → falls back to the last line.
	plain := "2026/07/10 11:12:12 [Info] all quiet"
	if got := distillXrayFailure(plain); !strings.Contains(got, "all quiet") {
		t.Errorf("fallback to last line failed: %q", got)
	}
}
