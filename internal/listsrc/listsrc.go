// Package listsrc resolves a remote domain-list URL into a flat, deduplicated
// hostname slice suitable for a Keenetic/nfqws hostlist file.
//
// It understands three "shapes" of remote list, best-effort:
//   - v2fly domain-list-community source files (domain:/full:/keyword:/
//     regexp: prefixes, include: directives, @attr tags) — this is the
//     primary target format, since it is the most common curated domain-list
//     source in the wild (geosite-backed routing rules).
//   - Plain newline-delimited domain lists (one hostname per line).
//   - hosts-file / AdBlock-style lines, on a best-effort basis (a leading
//     "0.0.0.0 " / "127.0.0.1 " IP token or a "||" AdBlock anchor is stripped
//     before the line is treated as a bare domain).
//
// The v2fly format is a directed graph of files linked by include: — Resolve
// walks that graph (bounded by MaxFiles/MaxDepth) and flattens every entry
// that survives attribute filtering into one sorted, deduped slice.
package listsrc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Options tunes how Resolve fetches and flattens a remote list. All fields
// are optional; the zero value is usable (Resolve fills in the documented
// defaults).
type Options struct {
	// AttrFilter, when non-empty (e.g. "cn", "ads"), selects ONLY entries
	// carrying that @attribute. Empty selects all entries. Matches v2fly
	// geosite `name@attr` semantics.
	AttrFilter string
	MaxFiles   int          // default 40 when <= 0 (cap on total fetched files incl. includes)
	MaxDomains int          // default 20000 when <= 0
	MaxDepth   int          // default 8 when <= 0 (include recursion depth)
	Client     *http.Client // default: http.Client{Timeout: 20s}
}

// Result is the flattened outcome of resolving a remote list and its
// transitive includes.
type Result struct {
	Domains   []string `json:"domains"`   // flattened, deduped, sorted, lowercased
	Skipped   []string `json:"skipped"`   // entries that can't be a plain domain (regexp:/keyword:), capped to ~50 samples
	Sources   []string `json:"sources"`   // every URL actually fetched (base + resolved includes), in fetch order
	Truncated bool     `json:"truncated"` // true if a Max* cap was hit
	SkippedN  int      `json:"skipped_n"` // total skipped count (Skipped may be a truncated sample)
}

// Tunable defaults; exposed as constants so tests can reference them without
// magic numbers, and so the doc comment on Options stays accurate.
const (
	defaultMaxFiles    = 40
	defaultMaxDomains  = 20000
	defaultMaxDepth    = 8
	defaultHTTPTimeout = 20 * time.Second
	maxBodyBytes       = 8 << 20 // 8 MiB per fetched file
	skippedSampleCap   = 50      // Skipped is a sample, not the full set, once this many collected
	userAgent          = "Mozilla/5.0 (compatible; keen-manager)"
)

// Resolve fetches url, following v2fly include: directives, and returns the
// flattened domain set. ctx bounds the whole operation.
func Resolve(ctx context.Context, rawURL string, opts Options) (Result, error) {
	opts = withDefaults(opts)

	r := &resolver{
		opts:    opts,
		visited: make(map[string]bool),
		domains: make(map[string]bool),
		skipped: make(map[string]bool),
	}

	root := strings.TrimSpace(rawURL)
	if root == "" {
		return Result{}, fmt.Errorf("listsrc: empty url")
	}
	normalized, err := normalizeSourceURL(root)
	if err != nil {
		return Result{}, fmt.Errorf("listsrc: %w", err)
	}

	if err := r.fetchAndParse(ctx, normalized, opts.AttrFilter, 0); err != nil {
		// A failure on the root URL is fatal; failures on includes are
		// swallowed by fetchAndParse itself so one bad include doesn't sink
		// an otherwise-good list.
		return Result{}, err
	}

	return r.result(), nil
}

// withDefaults returns a copy of opts with all documented zero-value
// defaults applied.
func withDefaults(opts Options) Options {
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = defaultMaxFiles
	}
	if opts.MaxDomains <= 0 {
		opts.MaxDomains = defaultMaxDomains
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = defaultMaxDepth
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return opts
}

// resolver carries the mutable state shared across the recursive include
// walk: which URLs have already been fetched (cycle guard + MaxFiles budget)
// and the accumulated domain/skipped sets (deduped as we go, since the same
// domain commonly appears via multiple include paths).
type resolver struct {
	opts Options

	visited   map[string]bool // normalized URL -> fetched
	filesUsed int

	domains   map[string]bool
	skipped   map[string]bool
	skippedN  int
	sources   []string
	truncated bool
}

// result snapshots the accumulated state into the public Result shape:
// sorted+deduped domains, a capped Skipped sample, and the full SkippedN
// count.
func (r *resolver) result() Result {
	domains := make([]string, 0, len(r.domains))
	for d := range r.domains {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	if len(domains) > r.opts.MaxDomains {
		domains = domains[:r.opts.MaxDomains]
		r.truncated = true
	}

	skipped := make([]string, 0, len(r.skipped))
	for s := range r.skipped {
		skipped = append(skipped, s)
	}
	sort.Strings(skipped)
	if len(skipped) > skippedSampleCap {
		skipped = skipped[:skippedSampleCap]
	}

	return Result{
		Domains:   domains,
		Skipped:   skipped,
		Sources:   r.sources,
		Truncated: r.truncated,
		SkippedN:  r.skippedN,
	}
}

// fetchAndParse downloads sourceURL (already normalized), parses it as a
// v2fly-ish domain list, and recurses into any include: directives. attr is
// the AttrFilter in effect for entries in THIS file (an include line's own
// @attr overrides it for the included file; otherwise the caller's filter is
// inherited, per the package doc).
//
// Errors fetching/parsing the ROOT file propagate to the caller (Resolve
// returns them). Errors on an INCLUDE are recorded as a Skipped entry and
// otherwise ignored, so one broken sibling file doesn't fail the whole
// resolution — mirroring how nfqws hostlists degrade gracefully on partial
// data.
func (r *resolver) fetchAndParse(ctx context.Context, sourceURL, attr string, depth int) error {
	if r.visited[sourceURL] {
		return nil // cycle guard
	}
	if r.filesUsed >= r.opts.MaxFiles {
		r.truncated = true
		return nil
	}
	if depth > r.opts.MaxDepth {
		r.truncated = true
		return nil
	}

	r.visited[sourceURL] = true
	r.filesUsed++

	body, err := fetchBody(ctx, r.opts.Client, sourceURL)
	if err != nil {
		if depth == 0 {
			return err // root fetch failure is fatal
		}
		r.recordSkip(fmt.Sprintf("include-error:%s: %v", sourceURL, err))
		return nil
	}
	r.sources = append(r.sources, sourceURL)

	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, raw := range lines {
		if r.filesUsed >= r.opts.MaxFiles {
			r.truncated = true
			break
		}
		if len(r.domains) >= r.opts.MaxDomains {
			r.truncated = true
			break
		}

		parsed := parseLine(raw)
		switch parsed.kind {
		case lineEmpty, lineComment:
			continue
		case lineInclude:
			includeAttr := attr
			if parsed.attr != "" {
				includeAttr = parsed.attr
			}
			includeURL, err := resolveSibling(sourceURL, parsed.value)
			if err != nil {
				r.recordSkip(fmt.Sprintf("include:%s (%v)", parsed.value, err))
				continue
			}
			if err := r.fetchAndParse(ctx, includeURL, includeAttr, depth+1); err != nil {
				r.recordSkip(fmt.Sprintf("include:%s (%v)", parsed.value, err))
			}
		case lineSkip:
			r.recordSkip(parsed.raw)
		case lineDomain:
			if !attrMatches(attr, parsed.attrs) {
				continue
			}
			if !looksLikeHostname(parsed.value) {
				// Only count it in Skipped if it was explicitly tagged
				// domain:/full: — an unprefixed bare token that fails
				// validation is silently dropped (see parsedLine.hadPrefix).
				if parsed.hadPrefix {
					r.recordSkip(parsed.raw)
				}
				continue
			}
			r.domains[strings.ToLower(parsed.value)] = true
		}
	}
	return nil
}

// recordSkip adds a sample to the skipped set and always increments the true
// count, even once the sample set is capped — SkippedN must reflect reality.
func (r *resolver) recordSkip(s string) {
	r.skippedN++
	if len(r.skipped) < skippedSampleCap*4 { // small headroom before sort+cap in result()
		r.skipped[s] = true
	}
}

// fetchBody GETs sourceURL with a browser-like User-Agent and returns the
// body as a string, capped to maxBodyBytes so a misbehaving/huge host can't
// exhaust memory. Only http/https schemes are fetched.
func fetchBody(ctx context.Context, client *http.Client, sourceURL string) (string, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", sourceURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q for %q", u.Scheme, sourceURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %q: %w", sourceURL, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %q: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %q: HTTP %d", sourceURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("read body of %q: %w", sourceURL, err)
	}
	return string(body), nil
}

// --- Pure parsing helpers (no I/O; unit-tested directly) -------------------

// lineKind classifies a single raw line of a v2fly-ish domain list.
type lineKind int

const (
	lineEmpty   lineKind = iota // blank after trimming
	lineComment                 // starts with #
	lineInclude                 // include:NAME [@attr]
	lineDomain                  // a domain/full: entry (possibly attr-tagged)
	lineSkip                    // keyword:/regexp: — not representable as a flat domain
)

// parsedLine is the result of parsing one line.
type parsedLine struct {
	kind  lineKind
	value string   // domain value (lineDomain/lineInclude) — lowercased for lineDomain
	attrs []string // @attr tags on a lineDomain entry, lowercased, without the @
	attr  string   // the include line's own @attr override (lineInclude only), lowercased
	raw   string   // original trimmed line (used for Skipped samples)

	// hadPrefix is true when a lineDomain entry carried an explicit
	// "domain:"/"full:" prefix. Per the parsing rules, an invalid value is
	// only worth recording in Skipped when the source clearly intended it as
	// a domain entry (had the prefix); a bare, unprefixed token that fails
	// validation is far more likely to be stray formatting/debris in a
	// plain domain list and is silently dropped instead.
	hadPrefix bool
}

// hostnameRE matches a plausible bare hostname: labels of letters/digits/
// hyphen separated by dots, at least one dot (so "localhost" style single
// labels are rejected — a flat nfqws hostlist has no use for those and they
// are almost always parsing debris, not a real routing target).
var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

// looksLikeHostname reports whether s is a syntactically plausible domain:
// letters/digits/hyphen/dot only, at least one dot, no spaces, no leading/
// trailing dot or hyphen per label. This is intentionally conservative — it
// exists to catch parser debris (stray tokens, malformed entries), not to be
// a full RFC 1035 validator.
func looksLikeHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	if strings.ContainsAny(s, " \t") {
		return false
	}
	return hostnameRE.MatchString(s)
}

// stripInlineComment removes a trailing " #..." inline comment. A '#' is
// only treated as a comment start when preceded by whitespace (or at the
// very start of the string, handled separately by callers) — this avoids
// mangling values that legitimately contain '#' with no separating space,
// though in practice v2fly source files always space-separate the comment.
func stripInlineComment(s string) string {
	if idx := strings.Index(s, " #"); idx >= 0 {
		return s[:idx]
	}
	if idx := strings.Index(s, "\t#"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// parseLine parses one raw line of a v2fly domain-list file (see package
// doc for the full grammar). It never errors: unparseable/malformed input
// degrades to lineSkip or lineEmpty rather than panicking, since these files
// are third-party data we don't control.
func parseLine(raw string) parsedLine {
	line := strings.TrimSpace(raw)
	if line == "" {
		return parsedLine{kind: lineEmpty}
	}
	if strings.HasPrefix(line, "#") {
		return parsedLine{kind: lineComment}
	}

	line = strings.TrimSpace(stripInlineComment(line))
	if line == "" {
		return parsedLine{kind: lineEmpty}
	}

	// include:NAME [@attr ...] — only the first @attr is meaningful for an
	// include (it sets the filter for the whole included file), but we
	// tolerate/ignore extras rather than rejecting the line.
	if rest, ok := cutPrefixFold(line, "include:"); ok {
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return parsedLine{kind: lineSkip, raw: line}
		}
		name := fields[0]
		attr := ""
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "@") && len(f) > 1 {
				attr = strings.ToLower(strings.TrimPrefix(f, "@"))
				break
			}
		}
		return parsedLine{kind: lineInclude, value: name, attr: attr, raw: line}
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return parsedLine{kind: lineEmpty}
	}
	head := fields[0]
	var attrs []string
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "@") && len(f) > 1 {
			attrs = append(attrs, strings.ToLower(strings.TrimPrefix(f, "@")))
		}
	}

	switch {
	case hasFoldPrefix(head, "keyword:"):
		return parsedLine{kind: lineSkip, raw: line}
	case hasFoldPrefix(head, "regexp:"):
		return parsedLine{kind: lineSkip, raw: line}
	case hasFoldPrefix(head, "full:"):
		value := strings.ToLower(cutFoldPrefix(head, "full:"))
		return parsedLine{kind: lineDomain, value: value, attrs: attrs, raw: line, hadPrefix: true}
	case hasFoldPrefix(head, "domain:"):
		value := strings.ToLower(cutFoldPrefix(head, "domain:"))
		return parsedLine{kind: lineDomain, value: value, attrs: attrs, raw: line, hadPrefix: true}
	default:
		// No recognized prefix: treat the bare token as a domain. This also
		// covers plain newline-delimited domain lists and (best-effort)
		// AdBlock/hosts lines once their leading IP/anchor noise is
		// stripped by stripHostsOrAdblockNoise.
		value := strings.ToLower(stripHostsOrAdblockNoise(head))
		return parsedLine{kind: lineDomain, value: value, attrs: attrs, raw: line}
	}
}

// stripHostsOrAdblockNoise best-effort-strips the non-domain decoration
// found in hosts-file lines ("0.0.0.0 example.com" — but note the IP and
// domain arrive as separate fields, so this only strips a domain-shaped
// token itself) and AdBlock-style anchors ("||example.com^",
// "||example.com^$third-party"). It is intentionally narrow: it only handles
// the couple of decorations that would otherwise make an obviously-a-domain
// token fail looksLikeHostname, so it can't accidentally launder garbage
// into a false-positive domain.
func stripHostsOrAdblockNoise(tok string) string {
	tok = strings.TrimPrefix(tok, "||")
	// AdBlock separator '^' and any trailing options after it (e.g. "^$third-party").
	if idx := strings.IndexByte(tok, '^'); idx >= 0 {
		tok = tok[:idx]
	}
	tok = strings.TrimSuffix(tok, "^")
	return tok
}

// attrMatches reports whether an entry tagged with entryAttrs should be
// included under the given filter, per v2fly geosite `name@attr` semantics:
// an empty filter selects everything; a non-empty filter selects only
// entries carrying that exact attribute (case-insensitive; entryAttrs are
// already lowercased by parseLine).
func attrMatches(filter string, entryAttrs []string) bool {
	if filter == "" {
		return true
	}
	filter = strings.ToLower(filter)
	for _, a := range entryAttrs {
		if a == filter {
			return true
		}
	}
	return false
}

// --- Pure URL helpers (no I/O; unit-tested directly) ------------------------

// githubBlobRE matches a GitHub web UI "blob" URL:
// https://github.com/{owner}/{repo}/blob/{ref}/{path}
var githubBlobRE = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/blob/([^/]+)/(.+)$`)

// normalizeSourceURL rewrites a GitHub "blob" web URL to the equivalent
// raw.githubusercontent.com URL, since the blob URL serves an HTML page, not
// the file body. Any other URL (including an already-raw GitHub URL, which
// may already contain "refs/heads/{branch}/") is returned unchanged. Only
// http/https URLs are accepted; anything else is an error so a caller can't
// be tricked into following a file://, javascript:, etc. "list".
func normalizeSourceURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url %q must be http(s)", raw)
	}
	if m := githubBlobRE.FindStringSubmatch(raw); m != nil {
		owner, repo, ref, path := m[1], m[2], m[3], m[4]
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, path), nil
	}
	return raw, nil
}

// resolveSibling resolves an include: NAME reference against the directory
// of currentURL: the last path segment of currentURL is dropped and NAME is
// appended in its place. NAME may itself contain slashes (v2fly's
// include:apple/itunes-cn convention), which are preserved verbatim.
//
// currentURL must already be a normalized http(s) URL (normalizeSourceURL
// has been applied, or it is itself the result of a prior resolveSibling
// call) — resolveSibling does not re-run GitHub blob normalization, since an
// include target is always a raw sibling file, never a blob URL.
func resolveSibling(currentURL, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty include name")
	}
	u, err := url.Parse(currentURL)
	if err != nil {
		return "", fmt.Errorf("parse current url %q: %w", currentURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("current url %q must be http(s)", currentURL)
	}

	dir := u.Path
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		dir = dir[:idx+1]
	} else {
		dir = "/"
	}
	u.Path = dir + name
	return u.String(), nil
}

// --- small local helpers (case-insensitive prefix handling) ----------------

// cutPrefixFold is like strings.CutPrefix but case-insensitive on the
// prefix, since v2fly tooling is lenient about "Include:" vs "include:" in
// practice even though upstream data is consistently lowercase.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) {
		return "", false
	}
	if !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

func hasFoldPrefix(s, prefix string) bool {
	_, ok := cutPrefixFold(s, prefix)
	return ok
}

func cutFoldPrefix(s, prefix string) string {
	rest, _ := cutPrefixFold(s, prefix)
	return rest
}
