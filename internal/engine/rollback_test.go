package engine

import "testing"

func TestNormalizeRollbackTimeout(t *testing.T) {
	cases := []struct {
		name   string
		stored int
		want   int
	}{
		{"zero means default", 0, defaultRollbackTimeoutS},
		{"negative means default", -5, defaultRollbackTimeoutS},
		{"tiny is clamped up to the floor", 3, minRollbackTimeoutS},
		{"exactly the floor is kept", minRollbackTimeoutS, minRollbackTimeoutS},
		{"just below the floor is clamped", minRollbackTimeoutS - 1, minRollbackTimeoutS},
		{"a sane value passes through", 45, 45},
		{"the default passes through", 90, 90},
		{"a large value passes through", 600, 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeRollbackTimeout(tc.stored); got != tc.want {
				t.Fatalf("normalizeRollbackTimeout(%d) = %d, want %d", tc.stored, got, tc.want)
			}
		})
	}
}
