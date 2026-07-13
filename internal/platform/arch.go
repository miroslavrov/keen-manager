// Package platform detects the host router environment (CPU architecture,
// Entware layout, KeeneticOS) and provides safe command execution helpers.
package platform

import (
	"io"
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

// ELFArch reads the ELF header of the file at path and reports which CPU
// architecture the binary targets, plus whether the file is a valid ELF at all.
// It NEVER executes the file — it only inspects header bytes — so it is safe to
// call on a binary of the wrong architecture. That is exactly the case it exists
// to diagnose: a present-but-unrunnable /opt/sbin/xray (left by an earlier
// install, copied from the wrong build, or a download the ISP's DPI cut short)
// otherwise only reveals itself as a cryptic "exec format error" the moment the
// engine forks it.
//
// The bool is false when path is missing or is not an ELF (e.g. a shell script,
// an HTML error page a failed download saved, or a still-compressed archive); in
// that case Arch is ArchUnknown. A valid ELF whose e_machine we don't map (say a
// plain x86 build) yields (ArchUnknown, true).
func ELFArch(path string) (Arch, bool) {
	f, err := os.Open(path)
	if err != nil {
		return ArchUnknown, false
	}
	defer f.Close()
	var h [20]byte
	if _, err := io.ReadFull(f, h[:]); err != nil {
		return ArchUnknown, false
	}
	if h[0] != 0x7f || h[1] != 'E' || h[2] != 'L' || h[3] != 'F' {
		return ArchUnknown, false
	}
	bigEndian := h[5] == 0x02 // EI_DATA: 0x01 = LE, 0x02 = BE
	// e_machine is a 2-byte field at offset 18, in the file's byte order.
	machine := uint16(h[18]) | uint16(h[19])<<8
	if bigEndian {
		machine = uint16(h[19]) | uint16(h[18])<<8
	}
	switch machine {
	case 0x08: // EM_MIPS — endianness distinguishes the two MIPS targets
		if bigEndian {
			return ArchMIPS, true
		}
		return ArchMIPSLE, true
	case 0x28: // EM_ARM (32-bit)
		return ArchARM, true
	case 0x3e: // EM_X86_64
		return ArchAMD64, true
	case 0xb7: // EM_AARCH64
		return ArchARM64, true
	}
	return ArchUnknown, true
}

// ELFArchBytes is like ELFArch but takes a byte slice instead of a file path.
// Used by the updater to verify a downloaded binary without writing it to disk.
func ELFArchBytes(data []byte) (Arch, bool) {
	if len(data) < 20 {
		return ArchUnknown, false
	}
	h := data[:20]
	if h[0] != 0x7f || h[1] != 'E' || h[2] != 'L' || h[3] != 'F' {
		return ArchUnknown, false
	}
	bigEndian := h[5] == 0x02
	machine := uint16(h[18]) | uint16(h[19])<<8
	if bigEndian {
		machine = uint16(h[19]) | uint16(h[18])<<8
	}
	switch machine {
	case 0x08:
		if bigEndian {
			return ArchMIPS, true
		}
		return ArchMIPSLE, true
	case 0x28:
		return ArchARM, true
	case 0x3e:
		return ArchAMD64, true
	case 0xb7:
		return ArchARM64, true
	}
	return ArchUnknown, true
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
