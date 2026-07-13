package platform

import (
	"os"
	"testing"
)

// machineFor maps a host Arch back to the ELF e_machine byte the arch_test.go
// elfHeader helper expects, so these tests build a "correct-arch" binary for
// whatever CPU the test happens to run on. 0 means "no mapping" → skip.
func machineFor(a Arch) uint16 {
	switch a {
	case ArchAMD64:
		return 0x3e
	case ArchARM64:
		return 0xb7
	case ArchARM:
		return 0x28
	case ArchMIPSLE:
		return 0x08
	}
	return 0
}

func TestRunnable(t *testing.T) {
	host := HostArch()
	m := machineFor(host)
	if m == 0 {
		t.Skipf("no ELF machine mapping for host arch %q", host)
	}

	// A correct-arch ELF is runnable.
	if !Runnable(writeTemp(t, elfHeader(m, false))) {
		t.Fatal("host-arch ELF: Runnable=false, want true")
	}

	// A wrong-arch ELF is NOT runnable — this is exactly the "exec format error"
	// case the resolver exists to skip.
	wrong := uint16(0xb7) // arm64
	if host == ArchARM64 {
		wrong = 0x3e // amd64
	}
	if Runnable(writeTemp(t, elfHeader(wrong, false))) {
		t.Fatal("wrong-arch ELF: Runnable=true, want false")
	}

	// A real "#!" script is runnable (the kernel hands it to an interpreter);
	// a header-less blob (e.g. an HTML error page a failed download saved) is not.
	if !Runnable(writeTemp(t, []byte("#!/bin/sh\nexit 0\n"))) {
		t.Fatal("shebang script: Runnable=false, want true")
	}
	if Runnable(writeTemp(t, []byte("<html>not a binary</html>"))) {
		t.Fatal("non-ELF non-script: Runnable=true, want false")
	}

	// A valid ELF without an execute bit is not runnable.
	noexec := t.TempDir() + "/noexec"
	if err := os.WriteFile(noexec, elfHeader(m, false), 0o644); err != nil {
		t.Fatalf("write noexec: %v", err)
	}
	if Runnable(noexec) {
		t.Fatal("non-executable ELF: Runnable=true, want false")
	}

	// A missing file is not runnable.
	if Runnable("/no/such/ip/binary") {
		t.Fatal("missing file: Runnable=true, want false")
	}
}

func TestResolveIPPrefersRunnableIPBin(t *testing.T) {
	host := HostArch()
	m := machineFor(host)
	if m == 0 {
		t.Skipf("no ELF machine mapping for host arch %q", host)
	}
	good := writeTemp(t, elfHeader(m, false))
	if got := ResolveIP(Paths{IPBin: good}); got != good {
		t.Fatalf("ResolveIP with a runnable IPBin = %q, want %q", got, good)
	}
	if IPBinBroken(Paths{IPBin: good}) {
		t.Fatal("IPBinBroken=true for a runnable IPBin, want false")
	}
}

func TestResolveIPSkipsWrongArchIPBin(t *testing.T) {
	host := HostArch()
	if host == ArchUnknown {
		t.Skip("host arch unknown; arch check not meaningful")
	}
	wrong := uint16(0xb7) // arm64
	if host == ArchARM64 {
		wrong = 0x3e // amd64
	}
	bad := writeTemp(t, elfHeader(wrong, false))
	p := Paths{IPBin: bad}

	if !IPBinBroken(p) {
		t.Fatal("IPBinBroken=false for a wrong-arch IPBin, want true")
	}
	if got := ResolveIP(p); got == bad {
		t.Fatalf("ResolveIP returned the wrong-arch IPBin %q, want a fallback", got)
	}
}
