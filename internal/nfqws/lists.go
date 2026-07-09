package nfqws

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// KnownLists are the standard nfqws2 hostlist/ipset files.
var KnownLists = []string{"user.list", "auto.list", "exclude.list", "ipset.list", "ipset_exclude.list"}

// DefaultListSplit is the maximum number of domains keen-manager writes into a
// single hostlist file when importing a large remote list. It deliberately
// matches keenetic.MaxDomainsPerGroup (300): the same "one screen of a few
// hundred entries" ceiling the Keenetic native object-group path uses, so a
// domain set behaves consistently whether it is routed via dns-proxy or fed to
// nfqws2. Oversized lists are split across numbered siblings (user.list,
// user2.list, …) by SplitDomains so no single file grows unwieldy.
const DefaultListSplit = 300

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

// NamedList is one hostlist file's worth of domains: a filename and the exact
// (already deduped/ordered) domains that belong in it.
type NamedList struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

// normalizeBase sanitizes an arbitrary caller-supplied base list name into a
// bare "X.list" filename: path components are stripped (no traversal), a
// missing ".list" suffix is added, and an empty/degenerate name falls back to
// "user.list" (the default nfqws2 auto-hostlist).
func normalizeBase(base string) string {
	base = strings.TrimSpace(base)
	if base != "" {
		base = filepath.Base(base)
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "user.list"
	}
	if !strings.HasSuffix(base, ".list") {
		base += ".list"
	}
	return base
}

// SplitListName returns the file name for the index-th chunk of a hostlist
// family derived from base. The first chunk (index 0) keeps base itself; every
// later chunk gets a numeric suffix on the stem: base "user.list" yields
// "user.list", "user2.list", "user3.list", … — the convention Keenetic/nfqws2
// users already expect for oversized lists. base is sanitized to a bare
// *.list filename; a missing ".list" suffix is added.
func SplitListName(base string, index int) string {
	base = normalizeBase(base)
	if index <= 0 {
		return base
	}
	stem := strings.TrimSuffix(base, ".list")
	return fmt.Sprintf("%s%d.list", stem, index+1)
}

// SplitDomains partitions domains into <=perFile-entry chunks, one NamedList per
// chunk, named by SplitListName. Input order is preserved within and across
// chunks. perFile <= 0 falls back to DefaultListSplit. An empty domain slice
// yields no chunks (the caller decides whether that is an error).
func SplitDomains(base string, domains []string, perFile int) []NamedList {
	if perFile <= 0 {
		perFile = DefaultListSplit
	}
	out := make([]NamedList, 0, (len(domains)+perFile-1)/perFile)
	for i := 0; i*perFile < len(domains); i++ {
		start := i * perFile
		end := start + perFile
		if end > len(domains) {
			end = len(domains)
		}
		out = append(out, NamedList{
			Name:    SplitListName(base, i),
			Domains: append([]string(nil), domains[start:end]...),
		})
	}
	return out
}

// familyRe builds a matcher for the numbered-sibling family of a base list
// ("user.list" -> user.list, user2.list, user123.list). Used to read a whole
// family back and to prune stale siblings after a smaller re-import.
func familyRe(base string) *regexp.Regexp {
	stem := strings.TrimSuffix(normalizeBase(base), ".list")
	return regexp.MustCompile(`^` + regexp.QuoteMeta(stem) + `[0-9]*\.list$`)
}

// familyFiles returns the existing on-disk file names of a base list's family,
// sorted by chunk index (base first, then 2, 3, …). Missing directory yields an
// empty slice, not an error.
func (c *Controller) familyFiles(base string) ([]string, error) {
	re := familyRe(base)
	entries, err := os.ReadDir(c.listsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && re.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return familyIndex(names[i]) < familyIndex(names[j]) })
	return names, nil
}

// familyIndex extracts the trailing numeric chunk index from a family file name
// ("user.list" -> 0, "user2.list" -> 2). Non-numbered names sort first (0).
func familyIndex(name string) int {
	stem := strings.TrimSuffix(name, ".list")
	i := len(stem)
	for i > 0 && stem[i-1] >= '0' && stem[i-1] <= '9' {
		i--
	}
	if i == len(stem) {
		return 0
	}
	n, _ := strconv.Atoi(stem[i:])
	return n
}

// ReadListFamily reads every existing file in a base list's family and returns
// the union of their domain lines (trimmed, comments and blanks dropped),
// deduplicated and sorted. Used to implement append-on-import without losing
// what earlier chunks already held.
func (c *Controller) ReadListFamily(base string) ([]string, error) {
	names, err := c.familyFiles(base)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, name := range names {
		content, err := c.ReadList(name)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(content, "\n") {
			d := strings.TrimSpace(line)
			if d == "" || strings.HasPrefix(d, "#") {
				continue
			}
			d = strings.ToLower(d)
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// WriteSplit writes domains across the base list's numbered family, one file per
// <=perFile chunk, then prunes any stale higher-index siblings left over from a
// previous, larger import (so re-importing a shorter list never leaves orphaned
// entries live). It returns the file names actually written, in order. A backup
// of each overwritten file is taken by WriteList.
func (c *Controller) WriteSplit(base string, domains []string, perFile int) ([]string, error) {
	parts := SplitDomains(base, domains, perFile)
	written := make([]string, 0, len(parts))
	keep := map[string]bool{}
	for _, p := range parts {
		content := strings.Join(p.Domains, "\n")
		if content != "" {
			content += "\n"
		}
		if err := c.WriteList(p.Name, content); err != nil {
			return written, err
		}
		written = append(written, p.Name)
		keep[p.Name] = true
	}
	// Prune stale siblings from a previous, larger split.
	existing, err := c.familyFiles(base)
	if err != nil {
		return written, err
	}
	for _, name := range existing {
		if keep[name] {
			continue
		}
		_ = os.Remove(filepath.Join(c.listsDir(), name))
	}
	return written, nil
}
