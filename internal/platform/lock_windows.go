//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrLocked is returned by AcquireLock when another live process already holds
// the lock. Callers distinguish "someone else is running" (fatal for a daemon)
// from an infrastructural failure (e.g. the lock dir is read-only) with
// errors.Is(err, ErrLocked).
var ErrLocked = errors.New("lock held by another process")

// Lock is an advisory whole-file lock held for the lifetime of a process.
// On Windows there is no flock(2); we use O_EXCL creation as a best-effort
// mutual-exclusion guard so tests can run on a Windows dev box. The production
// daemon runs on Linux where lock_unix.go uses kernel flock.
type Lock struct {
	f    *os.File
	path string
}

// AcquireLock creates the lock file exclusively. If it already exists, it
// reads the holder's pid and returns ErrLocked.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
	if err != nil {
		if os.IsExist(err) {
			existing, oerr := os.Open(path)
			if oerr == nil {
				holder := readPid(existing)
				_ = existing.Close()
				if holder != "" {
					return nil, fmt.Errorf("%w (pid %s)", ErrLocked, holder)
				}
			}
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	_ = f.Sync()
	return &Lock{f: f, path: path}, nil
}

// Release closes and removes the lock file. Safe to call on a nil Lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	_ = os.Remove(l.path)
	l.f = nil
	return err
}

// readPid reads the pid recorded in an already-open lock file (offset 0).
func readPid(f *os.File) string {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	return strings.TrimSpace(string(buf[:n]))
}
