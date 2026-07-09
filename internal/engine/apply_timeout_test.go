package engine

import (
	"context"
	"testing"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

func TestSleepCtx(t *testing.T) {
	if !sleepCtx(context.Background(), 5*time.Millisecond) {
		t.Fatal("sleepCtx should return true when it completes normally")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if sleepCtx(ctx, time.Hour) {
		t.Fatal("sleepCtx should return false when the context is already cancelled")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("sleepCtx honoured cancellation too slowly: %v", d)
	}
}

// A cancelled context (as produced by activateWithin's per-attempt deadline)
// must abort the verify/rollback deadman promptly instead of blocking the
// shared failover goroutine for the full rollback budget.
func TestVerifyActiveBailsOnCancelledContext(t *testing.T) {
	e := newTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := model.Connection{ID: "x", Name: "probe", Type: model.ConnXray}
	start := time.Now()
	ok, detail := e.verifyActive(ctx, c)
	if ok {
		t.Fatal("verifyActive should report failure on a cancelled context")
	}
	if detail == "" {
		t.Fatal("verifyActive should return a non-empty failure detail")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("verifyActive took %v on a cancelled context; should return promptly", d)
	}
}
