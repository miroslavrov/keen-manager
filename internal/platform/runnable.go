package platform

import (
	"io"
	"os"
	"os/exec"
	"runtime"
)

// HostArch reports the architecture of the *running* keen-manager process.
//
// Because the daemon binary had to be a correct-arch ELF just to start, its own
// architecture is the ground truth for "what this CPU can actually exec" — more
// reliable for the exec-format decision than probing opkg/uname, which can
// disagree with the kernel's real ISA on some firmwares. It is the yardstick
// Runnable compares candidate binaries against.
func HostArch() Arch {
	switch runtime.GOARCH {
	case "amd64":
		return ArchAMD64
	case "arm64":
		return ArchARM64
	case "arm":
		return ArchARM
	case "mipsle":
		return ArchMIPSLE
	case "mips":
		// A big-endian MIPS build reports GOARCH=mips; distinguish it from the
		// little-endian variant the same way the rest of the package does.
		if bigEndianELF() {
			return ArchMIPS
		}
		return ArchMIPSLE
	}
	return ArchUnknown
}

// Runnable reports whether the file at path can actually be exec()'d on this
// CPU — WITHOUT forking it. It exists to pre-empt the classic Entware "exec
// format error" (ENOEXEC): a present-but-wrong-arch or truncated/corrupt binary
// (an /opt/sbin/ip left over from a mipsel install, an ip-full a DPI middlebox
// cut short mid-download, a wrapper script whose shebang got stripped) that
// otherwise reveals itself only the moment the engine forks it.
//
// Rules:
//   - must exist and carry an execute bit;
//   - a valid ELF must match the host architecture (a mismatched e_machine, or
//     an unmapped one, is not runnable here);
//   - a non-ELF is runnable only if it is a real "#!" script the kernel can
//     hand to an interpreter — anything else (an HTML error page a failed
//     download saved, a still-gzipped blob, a header-less binary) is not.
//
// When the host architecture cannot be determined it errs toward "runnable", so
// this check can never make an already-working setup worse than before.
func Runnable(path string) bool {
	if path == "" || !fileExecutable(path) {
		return false
	}
	if arch, isELF := ELFArch(path); isELF {
		host := HostArch()
		if host == ArchUnknown {
			return true
		}
		return arch == host
	}
	return hasShebang(path)
}

// hasShebang reports whether path begins with the two bytes "#!", i.e. it is a
// script the kernel's binfmt_script handler can exec (unlike a header-less
// binary, which fails with ENOEXEC).
func hasShebang(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [2]byte
	if n, _ := io.ReadFull(f, b[:]); n < 2 {
		return false
	}
	return b[0] == '#' && b[1] == '!'
}

// ipCandidates lists, in priority order, where a usable `ip` (iproute2) binary
// may live on a Keenetic + Entware box. The Entware "ip-full" at p.IPBin is
// preferred because the busybox `ip` applet frequently cannot perform the
// fwmark/table policy routing keen-manager relies on; the firmware's own ip and
// PATH are fallbacks that at least keep basic operations working.
func (p Paths) ipCandidates() []string {
	return []string{
		p.IPBin, // /opt/sbin/ip (ip-full, preferred)
		"/opt/sbin/ip",
		"/usr/sbin/ip",
		"/sbin/ip", // KeeneticOS native
		"/usr/bin/ip",
		"/bin/ip",
	}
}

// ResolveIP returns the path to a usable `ip` binary for the current CPU. It
// walks ipCandidates() and SKIPS any that exists yet is not runnable on this
// architecture — the wrong-arch/corrupt /opt/sbin/ip that otherwise surfaces
// only as a cryptic "fork/exec /opt/sbin/ip: exec format error" the instant
// `ip rule add` is forked. It falls back to the firmware's own ip and finally
// to PATH, always returning something to exec (bare "ip" as the last resort) so
// callers need no nil handling.
func ResolveIP(p Paths) string {
	seen := map[string]bool{}
	for _, c := range p.ipCandidates() {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if Runnable(c) {
			return c
		}
	}
	if pth, err := exec.LookPath("ip"); err == nil {
		return pth
	}
	return "ip"
}

// IPBinBroken reports whether the PREFERRED Entware ip (p.IPBin) is present but
// not runnable on this CPU — i.e. ResolveIP had to fall back to another binary.
// Callers use it to emit a one-time, actionable diagnostic (reinstall
// iproute2 / ip-full) without re-implementing the arch check.
func IPBinBroken(p Paths) bool {
	return p.IPBin != "" && FileExists(p.IPBin) && !Runnable(p.IPBin)
}
