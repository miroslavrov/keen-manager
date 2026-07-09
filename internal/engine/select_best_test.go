package engine

import (
	"reflect"
	"testing"
)

func TestRankByLatency(t *testing.T) {
	cases := []struct {
		name string
		in   []connLatency
		want []string
	}{
		{
			name: "fastest first",
			in: []connLatency{
				{id: "c", ms: 300, ok: true},
				{id: "a", ms: 40, ok: true},
				{id: "b", ms: 120, ok: true},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "unreachable dropped",
			in: []connLatency{
				{id: "a", ms: 50, ok: true},
				{id: "dead", ms: 0, ok: false},
				{id: "b", ms: 90, ok: true},
			},
			want: []string{"a", "b"},
		},
		{
			name: "ties keep input order (stable)",
			in: []connLatency{
				{id: "x", ms: 100, ok: true},
				{id: "y", ms: 100, ok: true},
				{id: "z", ms: 100, ok: true},
			},
			want: []string{"x", "y", "z"},
		},
		{
			name: "none reachable",
			in: []connLatency{
				{id: "a", ms: 0, ok: false},
				{id: "b", ms: 0, ok: false},
			},
			want: []string{},
		},
		{
			name: "empty",
			in:   nil,
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rankByLatency(tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("rankByLatency = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSelectVerifyCap keeps the per-candidate budget sane: capped low enough
// that trying maxSelectCandidates dead servers stays bounded, but above one
// probe cycle so a working server isn't cut off before it can verify.
func TestSelectVerifyCap(t *testing.T) {
	if selectVerifyCapS < minRollbackTimeoutS {
		t.Errorf("selectVerifyCapS=%d must be >= one probe cycle (%d)", selectVerifyCapS, minRollbackTimeoutS)
	}
	worstCaseS := maxSelectCandidates * (selectVerifyCapS + selectBringUpMarginS)
	if worstCaseS > 5*60 {
		t.Errorf("worst-case select-best %ds exceeds 5min; lower the cap or candidate count", worstCaseS)
	}
}
