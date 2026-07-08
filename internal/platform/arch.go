// Package platform detects the host router environment (CPU architecture,
// Entware layout, KeeneticOS) and provides safe command execution helpers.
package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Arch is a normalized target architecture string used for binary selection.
type Arch string

const (
	ArchMIPSLE  Arch = "mipsle" // MIPS little-endian, softfloat (most Keenetics)
	ArchMIPS    Arch = "mips"   // MIPS big-endian, softfloat
	ArchARM64   Arch = "arm64"  // aarch64 (Peak/Titan/Hopper/Giga KN-1012)
	ArchARM     Arch = "arm"    // 32-bit ARM (rare)
	ArchAMD64   Arch = "amd64"  // dev/testing only
	ArchUnknown Arch = "unknown"
)

// GOMIPS returns the required GOMIPS value for the arch ("softfloat" for MIPS).
func (a Arch) GOMIPS() string {
	if a == ArchMIPSLE || a == ArchMIPS {
		return "softfloat"
	}
	return ""
}

// DetectArch resolves the target arch the way the installer should: prefer
// `opkg print-architecture`, then fall back to uname + ELF byte-order probe.
func DetectArch() Arch {
	if a := archFromOpkg(); a != ArchUnknown {
		return a
	}
	return archFromUname()
}

func archFromOpkg() Arch {
	out, err := exec.Command("opkg", "print-architecture").Output()
	if err != nil {
		return ArchUnknown
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "arch" {
			name := f[1]
			switch {
			case name == "all" || name == "noarch":
				continue
			case strings.HasPrefix(name, "aarch64"):
				return ArchARM64
			case strings.HasPrefix(name, "mipselsf"), strings.HasPrefix(name, "mipsel"):
				return ArchMIPSLE
			case strings.HasPrefix(name, "mipssf"), strings.HasPrefix(name, "mips"):
				return ArchMIPS
			case strings.HasPrefix(name, "armv7"), strings.HasPrefix(name, "arm"):
				return ArchARM
			}
		}
	}
	return ArchUnknown
}

func archFromUname() Arch {
	// When running on the build host (dev), just report the Go runtime arch.
	switch runtime.GOARCH {
	case "amd64":
		return ArchAMD64
	case "arm64":
		return ArchARM64
	}

	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return ArchUnknown
	}
	m := strings.TrimSpace(string(out))
	switch {
	case m == "aarch64" || m == "arm64":
		return ArchARM64
	case strings.HasPrefix(m, "armv7"), strings.HasPrefix(m, "arm"):
		return ArchARM
	case m == "mips":
		if bigEndianELF() {
			return ArchMIPS
		}
		return ArchMIPSLE
	}
	return ArchUnknown
}

// bigEndianELF inspects the EI_DATA byte (offset 5) of a system binary:
// 0x01 = little-endian, 0x02 = big-endian.
func bigEndianELF() bool {
	for _, p := range []string{"/bin/sh", "/bin/busybox", "/opt/bin/sh"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		var hdr [6]byte
		n, _ := f.Read(hdr[:])
		f.Close()
		if n >= 6 && hdr[0] == 0x7f && hdr[1] == 'E' && hdr[2] == 'L' && hdr[3] == 'F' {
			return hdr[5] == 0x02
		}
	}
	return false
}
