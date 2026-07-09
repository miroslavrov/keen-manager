package engine

import (
	"reflect"
	"testing"
)

func TestNfqwsDaemonUnhealthy(t *testing.T) {
	cases := []struct {
		running, kernelReady bool
		wantBad              bool
		wantReasonContains   string
	}{
		{running: true, kernelReady: true, wantBad: false},
		{running: false, kernelReady: true, wantBad: true, wantReasonContains: "not running"},
		{running: true, kernelReady: false, wantBad: true, wantReasonContains: "kernel modules"},
		{running: false, kernelReady: false, wantBad: true, wantReasonContains: "not running"}, // daemon reported first
	}
	for _, c := range cases {
		bad, reason := nfqwsDaemonUnhealthy(c.running, c.kernelReady)
		if bad != c.wantBad {
			t.Errorf("nfqwsDaemonUnhealthy(%v,%v) bad=%v, want %v", c.running, c.kernelReady, bad, c.wantBad)
		}
		if c.wantReasonContains != "" && !contains(reason, c.wantReasonContains) {
			t.Errorf("reason %q should contain %q", reason, c.wantReasonContains)
		}
		if !bad && reason != "" {
			t.Errorf("healthy case should have empty reason, got %q", reason)
		}
	}
}

func TestCleanProbeDomains(t *testing.T) {
	in := []string{"https://Rutracker.ORG/path", "rutracker.org", " ", "example.com:443", "example.com"}
	got := cleanProbeDomains(in)
	want := []string{"rutracker.org", "example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cleanProbeDomains(%v) = %v, want %v", in, got, want)
	}
}

func TestMergeDomains(t *testing.T) {
	a := []string{"b.com", "a.com"}
	b := []string{"A.com", "c.com", "  "}
	got := mergeDomains(a, b)
	want := []string{"a.com", "b.com", "c.com"} // deduped (case-folded), sorted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeDomains = %v, want %v", got, want)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOfStr(s, sub) >= 0)
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
