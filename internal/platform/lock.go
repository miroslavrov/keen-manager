//go:build !windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrLocked is returned by AcquireLock when another live process already holds
// the lock. Callers distinguish "someone else is running" (fatal for a daemon)
// from an infrastructural failure (e.g. the lock dir is read-only) with
// errors.Is(err, ErrLocked).
var ErrLocked = errors.New("lock held by another process")

// Lock is an advisory whole-file flock held for the lifetime of a process. The
// kernel releases it automatically when the process exits or the fd closes, so
// — unlike a bare pid file — it never goes stale after a crash or SIGKILL, and
// there is no TOCTOU window where a dead pid looks alive.
type Lock struct {
	f    *os.File
	path string
}

// AcquireLock takes an exclusive, non-blocking flock on path (creating the file
// and its parent directory), then records the current pid in the file for
// humans reading it. It returns ErrLocked (wrapped with the holder's pid when
// readable) if another process already holds the lock.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		holder := readPid(f)
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if holder != "" {
				return nil, fmt.Errorf("%w (pid %s)", ErrLocked, holder)
			}
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("flock: %w", err)
	}
	// We own it. Record our pid (best-effort — a write failure does not void the
	// lock the kernel already granted us).
	if err := f.Truncate(0); err == nil {
		_, _ = f.Seek(0, 0)
		_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
		_ = f.Sync()
	}
	return &Lock{f: f, path: path}, nil
}

// Release unlocks and closes the lock, removing the file (best-effort). Safe to
// call on a nil Lock so a `defer lock.Release()` after a failed acquire is fine.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
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
