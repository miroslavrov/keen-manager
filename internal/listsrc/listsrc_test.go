package listsrc

import (
	"reflect"
	"testing"
)

// --- URL normalization / include resolution --------------------------------

func TestNormalizeSourceURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "github blob rewritten to raw",
			in:   "https://github.com/v2fly/domain-list-community/blob/master/data/cloudflare",
			want: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare",
		},
		{
			name: "github blob with nested path segments",
			in:   "https://github.com/v2fly/domain-list-community/blob/master/data/apple/itunes-cn",
			want: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/apple/itunes-cn",
		},
		{
			name: "already-raw url untouched",
			in:   "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare",
			want: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare",
		},
		{
			name: "raw url with refs/heads untouched",
			in:   "https://raw.githubusercontent.com/v2fly/domain-list-community/refs/heads/master/data/cloudflare",
			want: "https://raw.githubusercontent.com/v2fly/domain-list-community/refs/heads/master/data/cloudflare",
		},
		{
			name: "unrelated plain url untouched",
			in:   "https://example.com/lists/mylist.txt",
			want: "https://example.com/lists/mylist.txt",
		},
		{
			name:    "non-http scheme rejected",
			in:      "ftp://example.com/list.txt",
			wantErr: true,
		},
		{
			name:    "file scheme rejected",
			in:      "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "unparseable url rejected",
			in:      "://not a url",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeSourceURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeSourceURL(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSourceURL(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("normalizeSourceURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveSibling(t *testing.T) {
	cases := []struct {
		name       string
		currentURL string
		include    string
		want       string
		wantErr    bool
	}{
		{
			name:       "simple sibling in same directory",
			currentURL: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare",
			include:    "cloudflare-cn",
			want:       "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare-cn",
		},
		{
			name:       "include name containing slashes is preserved",
			currentURL: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/cloudflare",
			include:    "apple/itunes-cn",
			want:       "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/apple/itunes-cn",
		},
		{
			name:       "current file already inside a subdirectory",
			currentURL: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/apple/itunes-cn",
			include:    "itunes",
			want:       "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/apple/itunes",
		},
		{
			name:       "generic https host also resolved relatively",
			currentURL: "https://example.com/lists/base.txt",
			include:    "extra.txt",
			want:       "https://example.com/lists/extra.txt",
		},
		{
			name:       "root-level current path (no directory segment)",
			currentURL: "https://example.com/base",
			include:    "extra",
			want:       "https://example.com/extra",
		},
		{
			// A bare host with NO path at all (u.Path == "") is a distinct
			// case from "/base" above: LastIndex finds no '/' at all, so
			// the fallback dir "/" kicks in rather than the found-slash
			// branch. Both must produce the same kind of result (sibling
			// placed directly under the host root).
			name:       "current url has no path at all",
			currentURL: "https://example.com",
			include:    "extra.txt",
			want:       "https://example.com/extra.txt",
		},
		{
			name:       "empty include name rejected",
			currentURL: "https://example.com/lists/base.txt",
			include:    "",
			wantErr:    true,
		},
		{
			name:       "whitespace-only include name rejected",
			currentURL: "https://example.com/lists/base.txt",
			include:    "   ",
			wantErr:    true,
		},
		{
			name:       "non-http current url rejected",
			currentURL: "ftp://example.com/lists/base.txt",
			include:    "extra.txt",
			wantErr:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSibling(tc.currentURL, tc.include)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveSibling(%q, %q) = %q, want error", tc.currentURL, tc.include, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSibling(%q, %q) unexpected error: %v", tc.currentURL, tc.include, err)
			}
			if got != tc.want {
				t.Errorf("resolveSibling(%q, %q) = %q, want %q", tc.currentURL, tc.include, got, tc.want)
			}
		})
	}
}

// --- Hostname validation -----------------------------------------------------

func TestLooksLikeHostname(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"www.example.com", true},
		{"a.b.c.example.co.uk", true},
		{"xn--80ak6aa92e.com", true}, // punycode label, still hostname-shaped
		{"123.example.com", true},
		{"single-label", false}, // no dot
		{"", false},             // empty
		{"example .com", false}, // internal space
		{" example.com", false}, // handled upstream by TrimSpace, but defensively rejected too since it contains a space
		{"example.com ", false}, // trailing space
		{"exa mple.com", false}, // space mid-token
		{"regexp:^.*\\.cn$", false},
		{"keyword:facebook", false},
		{"-example.com", false},  // leading hyphen label (regex requires alnum start)
		{"example-.com", false},  // trailing hyphen label (regex requires alnum end)
		{"exa..mple.com", false}, // empty label between dots
		{".example.com", false},  // leading dot
		{"example.com.", false},  // trailing dot
		{"example.c0m", true},    // digits in TLD-ish label are fine for this shape check
		{"日本.jp", false},         // non-ASCII rejected (regex is ASCII-only; callers should pass punycode)
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := looksLikeHostname(tc.in)
			if got != tc.want {
				t.Errorf("looksLikeHostname(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- Attribute filtering -----------------------------------------------------

func TestAttrMatches(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		attrs  []string
		want   bool
	}{
		{"empty filter includes untagged entry", "", nil, true},
		{"empty filter includes tagged entry", "", []string{"cn"}, true},
		{"cn filter matches cn-tagged entry", "cn", []string{"cn"}, true},
		{"cn filter rejects untagged entry", "cn", nil, false},
		{"cn filter rejects differently-tagged entry", "cn", []string{"ads"}, false},
		{"cn filter matches when entry carries multiple attrs", "cn", []string{"ads", "cn"}, true},
		{"filter is case-insensitive", "CN", []string{"cn"}, true},
		{"entry attrs already lowercased still match mixed-case filter", "Cn", []string{"cn"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := attrMatches(tc.filter, tc.attrs)
			if got != tc.want {
				t.Errorf("attrMatches(%q, %v) = %v, want %v", tc.filter, tc.attrs, got, tc.want)
			}
		})
	}
}

// --- Line parsing -------------------------------------------------------------

func TestParseLine_EmptyAndComment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want lineKind
	}{
		{"empty string", "", lineEmpty},
		{"whitespace only", "   \t  ", lineEmpty},
		{"comment line", "# this is a comment", lineComment},
		{"comment line with leading whitespace", "   # indented comment", lineComment},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.kind != tc.want {
				t.Errorf("parseLine(%q).kind = %v, want %v", tc.in, got.kind, tc.want)
			}
		})
	}
}

func TestParseLine_Include(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantValue string
		wantAttr  string
	}{
		{"plain include", "include:cloudflare-cn", "cloudflare-cn", ""},
		{"include with attr", "include:category-ads @ads", "category-ads", "ads"},
		{"include with attr and trailing whitespace", "  include:apple/itunes-cn @cn  ", "apple/itunes-cn", "cn"},
		{"include with inline comment", "include:cloudflare-cn # cloudflare cn ranges", "cloudflare-cn", ""},
		{"include case-insensitive prefix", "Include:cloudflare-cn", "cloudflare-cn", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.kind != lineInclude {
				t.Fatalf("parseLine(%q).kind = %v, want lineInclude", tc.in, got.kind)
			}
			if got.value != tc.wantValue {
				t.Errorf("parseLine(%q).value = %q, want %q", tc.in, got.value, tc.wantValue)
			}
			if got.attr != tc.wantAttr {
				t.Errorf("parseLine(%q).attr = %q, want %q", tc.in, got.attr, tc.wantAttr)
			}
		})
	}
}

func TestParseLine_DomainPrefixes(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantValue string
		wantAttrs []string
	}{
		{"bare domain no prefix", "example.com", "example.com", nil},
		{"domain: prefix", "domain:example.com", "example.com", nil},
		{"full: prefix", "full:www.example.com", "www.example.com", nil},
		{"mixed-case value lowercased", "domain:Example.COM", "example.com", nil},
		{"full with single attr", "full:www.example.com @cn", "www.example.com", []string{"cn"}},
		{"full with multiple attrs", "full:www.example.com @cn @ads", "www.example.com", []string{"cn", "ads"}},
		{"domain with attr and inline comment", "domain:example.com @cn # some comment", "example.com", []string{"cn"}},
		{"bare domain with inline comment", "example.com # a plain comment", "example.com", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.kind != lineDomain {
				t.Fatalf("parseLine(%q).kind = %v, want lineDomain", tc.in, got.kind)
			}
			if got.value != tc.wantValue {
				t.Errorf("parseLine(%q).value = %q, want %q", tc.in, got.value, tc.wantValue)
			}
			if !reflect.DeepEqual(got.attrs, tc.wantAttrs) {
				t.Errorf("parseLine(%q).attrs = %v, want %v", tc.in, got.attrs, tc.wantAttrs)
			}
		})
	}
}

func TestParseLine_HadPrefixFlag(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"domain: prefix sets hadPrefix", "domain:example.com", true},
		{"full: prefix sets hadPrefix", "full:example.com", true},
		{"bare token does not set hadPrefix", "example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.hadPrefix != tc.want {
				t.Errorf("parseLine(%q).hadPrefix = %v, want %v", tc.in, got.hadPrefix, tc.want)
			}
		})
	}
}

func TestParseLine_KeywordAndRegexpSkip(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"keyword prefix", "keyword:facebook"},
		{"regexp prefix", `regexp:^.*\.cn$`},
		{"keyword with attr", "keyword:facebook @ads"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.kind != lineSkip {
				t.Errorf("parseLine(%q).kind = %v, want lineSkip", tc.in, got.kind)
			}
		})
	}
}

func TestParseLine_InvalidDomainValues(t *testing.T) {
	// These exercise the *value* the parser extracts; validity is checked
	// separately by looksLikeHostname in fetchAndParse, but parseLine itself
	// must still classify the line as lineDomain (never panic) and must set
	// hadPrefix correctly so the caller knows whether an invalid value should
	// be recorded in Skipped.
	cases := []struct {
		name          string
		in            string
		wantHadPrefix bool
	}{
		{"unprefixed single label", "not-a-domain", false},
		{"prefixed invalid value with space debris", "domain:foo bar.com", true}, // "foo" is head; attrs parsing won't find "@..", but this documents behavior
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLine(tc.in)
			if got.kind != lineDomain {
				t.Fatalf("parseLine(%q).kind = %v, want lineDomain", tc.in, got.kind)
			}
			if got.hadPrefix != tc.wantHadPrefix {
				t.Errorf("parseLine(%q).hadPrefix = %v, want %v", tc.in, got.hadPrefix, tc.wantHadPrefix)
			}
		})
	}
}

func TestParseLine_NeverPanics(t *testing.T) {
	// Defensive fuzz-lite pass: none of these well-formed-ish adversarial
	// inputs should panic, regardless of what they classify as.
	inputs := []string{
		"",
		"#",
		"##",
		"include:",
		"include:   ",
		"domain:",
		"full:",
		"keyword:",
		"regexp:",
		"@cn",
		"   @cn   ",
		"#####include:foo",
		"domain:example.com@cn", // no space before @, so not parsed as a separate attr token
		strings_Repeat("a.", 200) + "com",
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parseLine(%q) panicked: %v", in, r)
				}
			}()
			_ = parseLine(in)
		}()
	}
}

// strings_Repeat avoids importing "strings" solely for one helper in the test file's
// panic-safety fixture; kept trivially local to make the intent obvious at the call site.
func strings_Repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// --- Hosts/AdBlock best-effort stripping -------------------------------------

func TestStripHostsOrAdblockNoise(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"example.com", "example.com"},
		{"||example.com^", "example.com"},
		{"||example.com^$third-party", "example.com"},
		{"||sub.example.com", "sub.example.com"},
		{"example.com^", "example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := stripHostsOrAdblockNoise(tc.in)
			if got != tc.want {
				t.Errorf("stripHostsOrAdblockNoise(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseLine_AdblockStyleLine(t *testing.T) {
	got := parseLine("||ads.example.com^$third-party")
	if got.kind != lineDomain {
		t.Fatalf("kind = %v, want lineDomain", got.kind)
	}
	if got.value != "ads.example.com" {
		t.Errorf("value = %q, want %q", got.value, "ads.example.com")
	}
	if !looksLikeHostname(got.value) {
		t.Errorf("expected stripped adblock value to look like a hostname: %q", got.value)
	}
}

// --- End-to-end pure-parsing integration (no network) ------------------------
//
// resolver.fetchAndParse is not directly unit-testable without I/O (it always
// fetches over HTTP), so the following test drives the parse-line loop body
// via a tiny local reimplementation of just the dedup/attr-filter/validate
// pipeline, proving the pure helpers compose correctly end-to-end on a
// realistic multi-line body. This mirrors exactly what fetchAndParse does
// per-line, without needing a network fetch.
func TestParsePipeline_RealisticBody(t *testing.T) {
	body := `# Cloudflare CN ranges
domain:cloudflare.com
full:www.cloudflare.com
domain:cloudflare.com # duplicate of line 2, same domain
keyword:cloudflare
regexp:^.*\.cf$
domain:ads.example.com @ads
domain:china.example.com @cn
china.example.net @cn
domain:not a domain value
include:cloudflare-extra
`

	filterCN := "cn"
	domains, skipped := runPipeline(body, filterCN)

	wantDomains := []string{"china.example.com", "china.example.net"}
	if !reflect.DeepEqual(domains, wantDomains) {
		t.Errorf("cn-filtered domains = %v, want %v", domains, wantDomains)
	}
	// keyword:/regexp: lines are always skipped regardless of attr filter,
	// and the malformed "domain:not a domain value" line is skipped too
	// (hadPrefix is true, value fails hostname validation).
	if len(skipped) == 0 {
		t.Errorf("expected some skipped entries, got none")
	}

	// With an empty filter, both @cn and @ads entries (and untagged ones)
	// should be included, deduped, and sorted.
	domainsAll, _ := runPipeline(body, "")
	wantAll := []string{
		"ads.example.com",
		"china.example.com",
		"china.example.net",
		"cloudflare.com",
		"www.cloudflare.com",
	}
	if !reflect.DeepEqual(domainsAll, wantAll) {
		t.Errorf("unfiltered domains = %v, want %v", domainsAll, wantAll)
	}
}

// runPipeline replicates the per-line body of resolver.fetchAndParse for a
// single (non-recursive) file body, so the composition of parseLine +
// attrMatches + looksLikeHostname + dedup/sort can be verified without any
// HTTP fetch. include: lines are intentionally left unresolved here (no
// sibling body available) and simply excluded from the domain/skip output,
// mirroring how a real fetch failure on an include is swallowed rather than
// failing the whole parse.
func runPipeline(body, attrFilter string) (domains []string, skipped []string) {
	domainSet := make(map[string]bool)
	skipSet := make(map[string]bool)

	for _, raw := range splitLines(body) {
		parsed := parseLine(raw)
		switch parsed.kind {
		case lineEmpty, lineComment, lineInclude:
			continue
		case lineSkip:
			skipSet[parsed.raw] = true
		case lineDomain:
			if !attrMatches(attrFilter, parsed.attrs) {
				continue
			}
			if !looksLikeHostname(parsed.value) {
				if parsed.hadPrefix {
					skipSet[parsed.raw] = true
				}
				continue
			}
			domainSet[parsed.value] = true
		}
	}

	for d := range domainSet {
		domains = append(domains, d)
	}
	sortStrings(domains)
	for s := range skipSet {
		skipped = append(skipped, s)
	}
	sortStrings(skipped)
	return domains, skipped
}

// splitLines is a tiny local newline splitter (avoids importing strings just
// for this in the test file, mirroring strings_Repeat above); production
// code's real splitting (with \r\n normalization) lives in fetchAndParse and
// is exercised indirectly via parseLine's own TrimSpace handling of stray \r.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// sortStrings is a tiny local insertion sort to avoid pulling in "sort" just
// for two small test slices; production dedup/sort correctness (using the
// real sort.Strings) is covered by TestResolverResult_DedupAndSort below.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// --- resolver.result(): dedup + sort + truncation ----------------------------

func TestResolverResult_DedupAndSort(t *testing.T) {
	r := &resolver{
		opts: withDefaults(Options{}),
		// Insertion order here is deliberately not sorted, and "apple.com"
		// is written twice (as fetchAndParse would if the same domain
		// appeared on two different lines, or via two different include
		// paths) — the map itself is the dedup, so this simulates that
		// collapse already having happened before result() ever sorts.
		domains: map[string]bool{
			"zebra.com": true,
			"apple.com": true,
			"mango.com": true,
		},
		skipped: map[string]bool{},
	}
	r.domains["apple.com"] = true // re-assert: still a single key, no duplicate in output

	got := r.result()
	want := []string{"apple.com", "mango.com", "zebra.com"}
	if !reflect.DeepEqual(got.Domains, want) {
		t.Errorf("Domains = %v, want %v", got.Domains, want)
	}
	if got.Truncated {
		t.Errorf("Truncated = true, want false (under cap)")
	}
}

func TestResolverResult_MaxDomainsTruncation(t *testing.T) {
	domains := map[string]bool{}
	for _, d := range []string{"a.com", "b.com", "c.com", "d.com", "e.com"} {
		domains[d] = true
	}
	r := &resolver{
		opts:    withDefaults(Options{MaxDomains: 3}),
		domains: domains,
		skipped: map[string]bool{},
	}
	got := r.result()
	if len(got.Domains) != 3 {
		t.Fatalf("len(Domains) = %d, want 3", len(got.Domains))
	}
	if !got.Truncated {
		t.Errorf("Truncated = false, want true (MaxDomains exceeded)")
	}
	// The kept domains must be the first 3 in sorted order.
	want := []string{"a.com", "b.com", "c.com"}
	if !reflect.DeepEqual(got.Domains, want) {
		t.Errorf("Domains = %v, want %v", got.Domains, want)
	}
}

func TestResolverResult_SkippedSampleCapVsSkippedN(t *testing.T) {
	r := &resolver{
		opts:    withDefaults(Options{}),
		domains: map[string]bool{},
		skipped: map[string]bool{},
	}
	// recordSkip is the real accumulation path used by fetchAndParse; drive
	// it directly to verify SkippedN counts every call while the sample
	// (Skipped) is capped.
	for i := 0; i < skippedSampleCap+20; i++ {
		r.recordSkip(uniqueSkipLabel(i))
	}
	got := r.result()
	if got.SkippedN != skippedSampleCap+20 {
		t.Errorf("SkippedN = %d, want %d", got.SkippedN, skippedSampleCap+20)
	}
	if len(got.Skipped) != skippedSampleCap {
		t.Errorf("len(Skipped) = %d, want %d (sample cap)", len(got.Skipped), skippedSampleCap)
	}
}

func uniqueSkipLabel(i int) string {
	// Deterministic, distinct labels without pulling in fmt/strconv here.
	digits := "0123456789"
	if i == 0 {
		return "skip-0"
	}
	buf := make([]byte, 0, 8)
	n := i
	for n > 0 {
		buf = append(buf, digits[n%10])
		n /= 10
	}
	// reverse
	for l, r := 0, len(buf)-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}
	return "skip-" + string(buf)
}

// --- withDefaults --------------------------------------------------------------

func TestWithDefaults(t *testing.T) {
	got := withDefaults(Options{})
	if got.MaxFiles != defaultMaxFiles {
		t.Errorf("MaxFiles = %d, want %d", got.MaxFiles, defaultMaxFiles)
	}
	if got.MaxDomains != defaultMaxDomains {
		t.Errorf("MaxDomains = %d, want %d", got.MaxDomains, defaultMaxDomains)
	}
	if got.MaxDepth != defaultMaxDepth {
		t.Errorf("MaxDepth = %d, want %d", got.MaxDepth, defaultMaxDepth)
	}
	if got.Client == nil {
		t.Fatal("Client = nil, want a default client")
	}
	if got.Client.Timeout != defaultHTTPTimeout {
		t.Errorf("Client.Timeout = %v, want %v", got.Client.Timeout, defaultHTTPTimeout)
	}

	// Explicit non-zero values must be preserved untouched.
	custom := Options{MaxFiles: 5, MaxDomains: 10, MaxDepth: 1}
	got2 := withDefaults(custom)
	if got2.MaxFiles != 5 || got2.MaxDomains != 10 || got2.MaxDepth != 1 {
		t.Errorf("withDefaults did not preserve explicit values: %+v", got2)
	}
}

// --- stripInlineComment -------------------------------------------------------

func TestStripInlineComment(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"domain:example.com # a comment", "domain:example.com"},
		{"domain:example.com\t# tab-separated comment", "domain:example.com"},
		{"domain:example.com", "domain:example.com"},
		// A leading "#" with no preceding " #"/"\t#" substring anywhere else
		// in the line is left untouched by this function — parseLine itself
		// special-cases a line starting with "#" before ever calling this
		// helper, so stripInlineComment only ever sees the trailing-comment
		// case in production. This fixture avoids embedding a stray " #"
		// later in the sentence, which would (correctly, per this helper's
		// narrow contract) get treated as the comment start.
		{"#standalone-no-space-comment", "#standalone-no-space-comment"},
		{"domain:exa#mple.com", "domain:exa#mple.com"}, // no space before '#': not treated as a comment
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := stripInlineComment(tc.in)
			if got != tc.want {
				t.Errorf("stripInlineComment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
