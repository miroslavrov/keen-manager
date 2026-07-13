package platform

import (
	"os"
	"testing"
)

// elfHeader builds a minimal 20-byte ELF header with the given e_machine and
// endianness — enough for ELFArch, which only reads the identification bytes
// and the e_machine field.
func elfHeader(machine uint16, bigEndian bool) []byte {
	h := make([]byte, 20)
	h[0], h[1], h[2], h[3] = 0x7f, 'E', 'L', 'F'
	h[4] = 2 // EI_CLASS: 64-bit (irrelevant to machine parsing)
	if bigEndian {
		h[5] = 2
		h[18] = byte(machine >> 8)
		h[19] = byte(machine)
	} else {
		h[5] = 1
		h[18] = byte(machine)
		h[19] = byte(machine >> 8)
	}
	return h
}

func TestELFArch(t *testing.T) {
	cases := []struct {
		name      string
		machine   uint16
		bigEndian bool
		want      Arch
	}{
		{"arm64", 0xb7, false, ArchARM64},
		{"amd64", 0x3e, false, ArchAMD64},
		{"arm", 0x28, false, ArchARM},
		{"mipsle", 0x08, false, ArchMIPSLE},
		{"mips", 0x08, true, ArchMIPS},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeTemp(t, elfHeader(c.machine, c.bigEndian))
			got, isELF := ELFArch(p)
			if !isELF {
				t.Fatalf("ELFArch(%s): isELF=false, want true", c.name)
			}
			if got != c.want {
				t.Fatalf("ELFArch(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}

	// A valid ELF with an unmapped machine (x86 = 3) is still an ELF.
	if got, isELF := ELFArch(writeTemp(t, elfHeader(0x03, false))); !isELF || got != ArchUnknown {
		t.Fatalf("x86 ELF = (%q,%v), want (unknown,true)", got, isELF)
	}

	// Non-ELF payloads: a shell script, a truncated header, and a missing file.
	if got, isELF := ELFArch(writeTemp(t, []byte("#!/bin/sh\necho hi\n"))); isELF || got != ArchUnknown {
		t.Fatalf("script = (%q,%v), want (unknown,false)", got, isELF)
	}
	if got, isELF := ELFArch(writeTemp(t, []byte{0x7f, 'E'})); isELF || got != ArchUnknown {
		t.Fatalf("short = (%q,%v), want (unknown,false)", got, isELF)
	}
	if got, isELF := ELFArch("/no/such/file/at/all"); isELF || got != ArchUnknown {
		t.Fatalf("missing = (%q,%v), want (unknown,false)", got, isELF)
	}
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := t.TempDir() + "/bin"
	if err := os.WriteFile(p, data, 0o755); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}
