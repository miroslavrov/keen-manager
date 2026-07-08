package platform

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner executes external commands. It supports a dry-run mode so device-side
// effects can be exercised safely off-device and in tests.
type Runner struct {
	DryRun bool
	// Log receives every command about to run (may be nil).
	Log func(cmd string)
	// Timeout is the default per-command timeout (0 = 30s).
	Timeout time.Duration
}

// NewRunner returns a Runner with sane defaults.
func NewRunner() *Runner { return &Runner{Timeout: 30 * time.Second} }

// Result holds the outcome of a command.
type Result struct {
	Cmd      string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// Run executes name+args and returns the captured result. In DryRun mode it logs
// and returns success without executing anything that would change device state.
func (r *Runner) Run(name string, args ...string) Result {
	full := name + " " + strings.Join(args, " ")
	if r.Log != nil {
		r.Log(full)
	}
	if r.DryRun {
		return Result{Cmd: full, Stdout: "[dry-run]", ExitCode: 0}
	}

	to := r.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()

	res := Result{Cmd: full, Stdout: out.String(), Stderr: errb.String()}
	if err != nil {
		res.Err = err
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res
}

// Output runs a read-only command and returns trimmed stdout (never dry-run
// gated, since reads are always safe).
func (r *Runner) Output(name string, args ...string) (string, error) {
	to := r.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

// Which reports whether a binary is available in PATH or at an absolute path.
func Which(name string) bool {
	if strings.Contains(name, "/") {
		_, err := exec.LookPath(name)
		if err == nil {
			return true
		}
		// absolute path that may not be +x-checked by LookPath on some FS
		return fileExecutable(name)
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// MustRun is a convenience that returns an error if the command failed.
func (r *Runner) MustRun(name string, args ...string) error {
	res := r.Run(name, args...)
	if res.Err != nil {
		return fmt.Errorf("%s: %v (%s)", res.Cmd, res.Err, strings.TrimSpace(res.Stderr))
	}
	return nil
}
