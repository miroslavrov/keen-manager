package platform

import "os"

// fileExecutable reports whether path exists and has any execute bit set.
func fileExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !fi.IsDir() && fi.Mode()&0o111 != 0
}

// FileExists is a small convenience used across packages.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
