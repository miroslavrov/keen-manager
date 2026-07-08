package platform

import (
	"os"
	"os/exec"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// Detect gathers read-only facts about the device.
func Detect() model.Platform {
	p := model.Platform{
		Arch:        string(DetectArch()),
		EntwarePath: envOr("KEEN_ROOT", "/opt"),
	}
	p.OSVersion = keeneticVersion()
	p.Model = keeneticModel()
	return p
}

// keeneticVersion tries the RCI endpoint, then ndmc, for the firmware version.
func keeneticVersion() string {
	if v := rciField("show version", "title"); v != "" {
		return v
	}
	out, err := exec.Command("ndmc", "-c", "show version").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "title:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "title:"))
			}
		}
	}
	return ""
}

func keeneticModel() string {
	out, err := exec.Command("ndmc", "-c", "show version").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "device:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "device:"))
			}
		}
	}
	return ""
}

// rciField is a best-effort scrape of the local RCI JSON API. It intentionally
// avoids a JSON dependency for one field and returns "" on any error.
func rciField(cmd, field string) string {
	// The RCI HTTP endpoint is on localhost:79; querying it requires auth on
	// most firmwares, so this is only a hint. Left minimal on purpose.
	_ = cmd
	_ = field
	return ""
}

// HasHardwareOffload reports whether flow offload / hardware NAT may be active,
// which would bypass netfilter (breaking nfqws / tproxy). Best-effort.
func HasHardwareOffload() bool {
	// Common indicators across MediaTek/Realtek SoCs.
	candidates := []string{
		"/sys/kernel/debug/hnat",
		"/proc/net/nf_conntrack_hwnat",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

// KernelModuleDirs returns the directories to search for .ko modules, covering
// the KeeneticOS 5.x relocation to /lib/system-modules.
func KernelModuleDirs() []string {
	rel, _ := exec.Command("uname", "-r").Output()
	kr := strings.TrimSpace(string(rel))
	dirs := []string{}
	for _, base := range []string{"/lib/modules", "/lib/system-modules"} {
		if kr != "" {
			dirs = append(dirs, base+"/"+kr)
		}
		dirs = append(dirs, base)
	}
	return dirs
}
