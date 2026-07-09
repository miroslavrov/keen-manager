package platform

import (
	"os"
	"path/filepath"
)

// Paths holds all filesystem locations keen-manager uses. Everything lives under
// the Entware root (/opt) so we never touch firmware partitions.
type Paths struct {
	Root     string // Entware root, usually /opt
	DataDir  string // /opt/etc/keen-manager
	BackupDir string // /opt/etc/keen-manager/backups
	LogDir   string // /opt/var/log
	RunDir   string // /var/run (tmpfs) for pid/sockets
	InitDir  string // /opt/etc/init.d
	NdmDir   string // /opt/etc/ndm

	// Managed component config locations (driven, not owned)
	XrayConfDir  string // /opt/etc/keen-manager/xray
	NfqwsConfDir string // /opt/etc/nfqws2
	NfqwsInit    string // /opt/etc/init.d/S51nfqws2
	NfqwsBin     string // /opt/usr/bin/nfqws2
	TpwsBin      string // /opt/usr/bin/tpws (zapret socket desync proxy)
	XrayBin      string // /opt/sbin/xray
	AwgBin       string // /opt/sbin/awg
	IPBin        string // /opt/sbin/ip (ip-full)
}

// DefaultPaths returns the standard Entware layout, honoring KEEN_ROOT /
// KEEN_DATA_DIR overrides (useful for testing off-device).
func DefaultPaths() Paths {
	root := envOr("KEEN_ROOT", "/opt")
	data := envOr("KEEN_DATA_DIR", filepath.Join(root, "etc", "keen-manager"))
	return Paths{
		Root:         root,
		DataDir:      data,
		BackupDir:    filepath.Join(data, "backups"),
		LogDir:       filepath.Join(root, "var", "log"),
		RunDir:       envOr("KEEN_RUN_DIR", "/var/run"),
		InitDir:      filepath.Join(root, "etc", "init.d"),
		NdmDir:       filepath.Join(root, "etc", "ndm"),
		XrayConfDir:  filepath.Join(data, "xray"),
		NfqwsConfDir: filepath.Join(root, "etc", "nfqws2"),
		NfqwsInit:    filepath.Join(root, "etc", "init.d", "S51nfqws2"),
		NfqwsBin:     filepath.Join(root, "usr", "bin", "nfqws2"),
		TpwsBin:      filepath.Join(root, "usr", "bin", "tpws"),
		XrayBin:      filepath.Join(root, "sbin", "xray"),
		AwgBin:       filepath.Join(root, "sbin", "awg"),
		IPBin:        filepath.Join(root, "sbin", "ip"),
	}
}

// EnsureDirs creates the writable directories keen-manager needs.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.DataDir, p.BackupDir, p.LogDir, p.XrayConfDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// LogFile returns the path to a component's log file.
func (p Paths) LogFile(name string) string {
	return filepath.Join(p.LogDir, name+".log")
}

// PidFile returns the path to a component's pid file (on tmpfs).
func (p Paths) PidFile(name string) string {
	return filepath.Join(p.RunDir, name+".pid")
}

// LockFile returns the path to a component's single-instance flock (on tmpfs,
// so it can never survive a reboot as a stale lock).
func (p Paths) LockFile(name string) string {
	return filepath.Join(p.RunDir, name+".lock")
}

// StateFile is the main persisted config document.
func (p Paths) StateFile() string { return filepath.Join(p.DataDir, "state.json") }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
