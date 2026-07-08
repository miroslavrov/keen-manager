package nfqws

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// KnownLists are the standard nfqws2 hostlist/ipset files.
var KnownLists = []string{"user.list", "auto.list", "exclude.list", "ipset.list", "ipset_exclude.list"}

func (c *Controller) listsDir() string {
	return filepath.Join(c.ConfDir, "lists")
}

// Lists returns the available list file names in the lists directory.
func (c *Controller) Lists() ([]string, error) {
	entries, err := os.ReadDir(c.listsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return KnownLists, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".list") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return KnownLists, nil
	}
	return names, nil
}

// safeName prevents path traversal; only bare *.list filenames are allowed.
func safeName(name string) (string, error) {
	base := filepath.Base(name)
	if base != name || strings.Contains(name, "..") || !strings.HasSuffix(base, ".list") {
		return "", fmt.Errorf("invalid list name %q", name)
	}
	return base, nil
}

// ReadList returns the contents of a hostlist file.
func (c *Controller) ReadList(name string) (string, error) {
	base, err := safeName(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(c.listsDir(), base))
	if os.IsNotExist(err) {
		return "", nil // treat missing list as empty
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteList overwrites a hostlist file (a backup is written first).
func (c *Controller) WriteList(name, content string) error {
	base, err := safeName(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.listsDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(c.listsDir(), base)
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".keen.bak", old, 0o600)
	}
	// Normalize line endings.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return os.WriteFile(path, []byte(content), 0o644)
}
