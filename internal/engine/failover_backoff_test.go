package engine

import (
	"testing"
	"time"
)

func TestBackoffDelayNoJitter(t *testing.T) {
	base := 30 * time.Second
	max := 5 * time.Minute
	// frac=0 => exactly half the (capped) exponential value.
	cases := []struct {
		streak int
		want   time.Duration
	}{
		{0, 15 * time.Second},  // clamped to streak 1
		{1, 15 * time.Second},  // base/2
		{2, 30 * time.Second},  // 2*base/2
		{3, 60 * time.Second},  // 4*base/2
		{4, 120 * time.Second}, // 8*base/2
		{5, 150 * time.Second}, // 16*base=8m -> cap 5m, /2 = 2m30s
		{50, 150 * time.Second}, // stays capped
	}
	for _, c := range cases {
		if got := backoffDelay(c.streak, base, max, 0); got != c.want {
			t.Errorf("backoffDelay(streak=%d) = %v, want %v", c.streak, got, c.want)
		}
	}
}

func TestBackoffDelayJitterBounds(t *testing.T) {
	base := 30 * time.Second
	max := 5 * time.Minute
	for streak := 1; streak <= 8; streak++ {
		lo := backoffDelay(streak, base, max, 0)         // floor (d/2)
		hi := backoffDelay(streak, base, max, 0.9999999) // ~ceiling (d)
		if hi < lo {
			t.Fatalf("streak %d: hi %v < lo %v", streak, hi, lo)
		}
		// The ceiling must be at most the full (un-jittered) delay = 2*floor.
		if hi > 2*lo {
			t.Fatalf("streak %d: jittered hi %v exceeds full delay %v", streak, hi, 2*lo)
		}
		// Monotonic non-decreasing floor as the streak grows.
		if streak > 1 {
			prev := backoffDelay(streak-1, base, max, 0)
			if lo < prev {
				t.Fatalf("streak %d floor %v < streak %d floor %v (not monotonic)", streak, lo, streak-1, prev)
			}
		}
	}
}

func TestEngineBackoffStreakAndReset(t *testing.T) {
	e := &Engine{}
	// Fresh engine: not in backoff.
	if e.inBackoff(time.Now()) {
		t.Fatal("new engine should not start in backoff")
	}
	// Bumping arms a window and grows the streak.
	d1 := e.bumpBackoff()
	if d1 <= 0 {
		t.Fatalf("bumpBackoff returned non-positive delay %v", d1)
	}
	if !e.inBackoff(time.Now()) {
		t.Fatal("engine should be in backoff immediately after a bump")
	}
	if e.foBackoffStreak != 1 {
		t.Fatalf("streak = %d, want 1", e.foBackoffStreak)
	}
	e.bumpBackoff()
	if e.foBackoffStreak != 2 {
		t.Fatalf("streak = %d, want 2 after a second bump", e.foBackoffStreak)
	}
	// A time far in the future is outside any window (delays are bounded by max).
	if e.inBackoff(time.Now().Add(time.Hour)) {
		t.Fatal("a far-future instant must be outside the backoff window")
	}
	// Reset clears both the window and the streak.
	e.foResetBackoff()
	if e.inBackoff(time.Now()) || e.foBackoffStreak != 0 {
		t.Fatalf("after reset: inBackoff=%v streak=%d, want false/0", e.inBackoff(time.Now()), e.foBackoffStreak)
	}
}
