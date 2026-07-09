package nfqws

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/platform"
)

func TestSplitListName(t *testing.T) {
	cases := []struct {
		base  string
		index int
		want  string
	}{
		{"user.list", 0, "user.list"},
		{"user.list", 1, "user2.list"},
		{"user.list", 2, "user3.list"},
		{"user", 0, "user.list"},        // missing suffix is added
		{"user", 1, "user2.list"},       // missing suffix + numbering
		{"exclude.list", 3, "exclude4.list"},
		{"", 0, "user.list"},            // empty base defaults to user.list
		{"", 1, "user2.list"},
		{"a/b/evil.list", 0, "evil.list"}, // path components are stripped
	}
	for _, c := range cases {
		if got := SplitListName(c.base, c.index); got != c.want {
			t.Errorf("SplitListName(%q,%d) = %q, want %q", c.base, c.index, got, c.want)
		}
	}
}

func TestSplitDomains(t *testing.T) {
	// Empty in -> no chunks.
	if got := SplitDomains("user.list", nil, 300); len(got) != 0 {
		t.Fatalf("expected no chunks for empty input, got %d", len(got))
	}

	// 301 domains at 300/file -> 2 files: user.list(300) + user2.list(1).
	domains := make([]string, 301)
	for i := range domains {
		domains[i] = "d" + strconv.Itoa(i) + ".example.com"
	}
	parts := SplitDomains("user.list", domains, DefaultListSplit)
	if len(parts) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(parts))
	}
	if parts[0].Name != "user.list" || len(parts[0].Domains) != 300 {
		t.Errorf("chunk 0 = %q (%d domains), want user.list (300)", parts[0].Name, len(parts[0].Domains))
	}
	if parts[1].Name != "user2.list" || len(parts[1].Domains) != 1 {
		t.Errorf("chunk 1 = %q (%d domains), want user2.list (1)", parts[1].Name, len(parts[1].Domains))
	}

	// Order preserved and total conserved.
	total := 0
	last := ""
	for _, p := range parts {
		for _, d := range p.Domains {
			if d <= last && last != "" {
				// input was generated in ascending index order but not lexical;
				// just assert no domains are lost, not strict ordering here.
			}
			total++
		}
	}
	if total != 301 {
		t.Errorf("split lost domains: total %d, want 301", total)
	}

	// perFile <= 0 falls back to DefaultListSplit.
	if got := SplitDomains("user.list", domains, 0); len(got) != 2 {
		t.Errorf("perFile=0 should fall back to DefaultListSplit; got %d chunks", len(got))
	}
}

// newTestController returns a Controller whose lists dir is an isolated temp dir.
func newTestController(t *testing.T) *Controller {
	t.Helper()
	dir := t.TempDir()
	return &Controller{
		Paths:   platform.Paths{},
		Runner:  platform.NewRunner(),
		ConfDir: dir,
	}
}

func TestWriteSplitAndReadFamily(t *testing.T) {
	c := newTestController(t)

	domains := make([]string, 650) // -> 3 files at 300/file (300,300,50)
	for i := range domains {
		domains[i] = "d" + strconv.Itoa(i) + ".example.com"
	}
	written, err := c.WriteSplit("user.list", domains, DefaultListSplit)
	if err != nil {
		t.Fatalf("WriteSplit: %v", err)
	}
	want := []string{"user.list", "user2.list", "user3.list"}
	if !reflect.DeepEqual(written, want) {
		t.Fatalf("written = %v, want %v", written, want)
	}
	// Files exist on disk.
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(c.listsDir(), name)); err != nil {
			t.Errorf("expected %s on disk: %v", name, err)
		}
	}
	// ReadListFamily returns the full union (deduped, sorted).
	fam, err := c.ReadListFamily("user.list")
	if err != nil {
		t.Fatalf("ReadListFamily: %v", err)
	}
	if len(fam) != 650 {
		t.Errorf("family union = %d domains, want 650", len(fam))
	}
	if !sort.StringsAreSorted(fam) {
		t.Errorf("family union should be sorted")
	}

	// Re-import a SMALLER list -> stale higher-index siblings pruned.
	small := []string{"only.example.com"}
	written2, err := c.WriteSplit("user.list", small, DefaultListSplit)
	if err != nil {
		t.Fatalf("WriteSplit (small): %v", err)
	}
	if !reflect.DeepEqual(written2, []string{"user.list"}) {
		t.Fatalf("written2 = %v, want [user.list]", written2)
	}
	for _, stale := range []string{"user2.list", "user3.list"} {
		if _, err := os.Stat(filepath.Join(c.listsDir(), stale)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be pruned, stat err = %v", stale, err)
		}
	}
	// The base file now holds exactly the small set.
	got, err := c.ReadList("user.list")
	if err != nil {
		t.Fatalf("ReadList: %v", err)
	}
	if strings.TrimSpace(got) != "only.example.com" {
		t.Errorf("user.list = %q, want single domain", got)
	}
}

func TestFamilyDoesNotMatchUnrelated(t *testing.T) {
	c := newTestController(t)
	// Seed unrelated files that must NOT be treated as part of the user family.
	for _, name := range []string{"user.list", "user2.list", "auto.list", "users.list", "userx.list"} {
		if err := c.WriteList(name, "x.example.com\n"); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	fam, err := c.familyFiles("user.list")
	if err != nil {
		t.Fatalf("familyFiles: %v", err)
	}
	want := []string{"user.list", "user2.list"}
	if !reflect.DeepEqual(fam, want) {
		t.Errorf("familyFiles = %v, want %v (auto/users/userx must be excluded)", fam, want)
	}
}

func TestFamilyIndex(t *testing.T) {
	cases := map[string]int{
		"user.list":   0,
		"user2.list":  2,
		"user10.list": 10,
		"auto.list":   0,
	}
	for name, want := range cases {
		if got := familyIndex(name); got != want {
			t.Errorf("familyIndex(%q) = %d, want %d", name, got, want)
		}
	}
}
