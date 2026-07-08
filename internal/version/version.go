// Package version holds build-time metadata, injected via -ldflags.
package version

import (
	"fmt"
	"runtime"
)

// These are overridden at build time:
//
//	-ldflags "-X github.com/miroslavrov/keen-manager/internal/version.Version=1.2.3 ..."
var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a human-readable version line.
func String() string {
	return fmt.Sprintf("keen-manager %s (commit %s, built %s, %s/%s, %s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Short returns just the semantic version.
func Short() string { return Version }
