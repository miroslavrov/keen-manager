package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAcquireLockMutualExclusion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keen.lock")

	l1, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	// A second acquire on the same path (a distinct fd, even in-process) must be
	// denied with ErrLocked — this is the daemon single-instance guard.
	if _, err := AcquireLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second AcquireLock err = %v, want ErrLocked", err)
	}

	// The lock file records the holder's pid for humans.
	data, _ := os.ReadFile(path)
	if got, want := strings.TrimSpace(string(data)), strconv.Itoa(os.Getpid()); got != want {
		t.Fatalf("lock file pid = %q, want %q", got, want)
	}

	// After release the path is immediately re-acquirable.
	if err := l1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	l2, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("re-AcquireLock after release: %v", err)
	}
	if err := l2.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestAcquireLockCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "nested", "keen.lock")
	l, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock into a nonexistent dir: %v", err)
	}
	defer l.Release()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
}

func TestReleaseNilIsSafe(t *testing.T) {
	var l *Lock
	if err := l.Release(); err != nil {
		t.Fatalf("nil Release: %v", err)
	}
}
