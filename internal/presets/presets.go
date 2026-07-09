// Package presets provides the built-in service-routing catalog: curated
// domain (and subnet) lists for popular services — YouTube, Instagram,
// Telegram, Netflix, OpenAI, Steam, and 80+ more — grouped by category.
//
// keen-manager applies these through Keenetic's native domain-routing stack
// (an `object-group fqdn` bound to a `dns-proxy route`, the mechanism behind
// the router's "Маршруты/DNS" section on KeeneticOS 5.x), so a user can send
// exactly the traffic for a chosen set of services through a VPN connection
// without routing everything.
//
// The catalog is embedded at build time from data/presets.json, which is
// generated from the upstream awg-manager preset set (see data/README.md and
// tools/genpresets). Because the router-native routing engine consumes flat
// domain/subnet lists, only those are kept here; heavier sing-box rule-set
// references from the source are intentionally dropped.
package presets

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed data/presets.json
var catalogJSON []byte

// Preset is one routable service (or a composite of several).
type Preset struct {
	// ID is a stable slug, e.g. "youtube". Safe for use in an object-group name.
	ID string `json:"id"`
	// Name is the human label, e.g. "YouTube".
	Name string `json:"name"`
	// Category groups presets in the UI: social, media, ai, gaming, developer,
	// cloud, block.
	Category string `json:"category"`
	// Icon is a lucide/brand icon slug the UI can resolve (best-effort).
	Icon string `json:"icon,omitempty"`
	// Notice is an optional human note (e.g. what a composite preset covers).
	Notice string `json:"notice,omitempty"`
	// Domains are the FQDNs to route (subdomains are matched by the router's
	// object-group resolver automatically).
	Domains []string `json:"domains,omitempty"`
	// Subnets are CIDR ranges to route via static routes (some services, e.g.
	// Cloudflare/Discord voice, are IP- rather than name-addressed).
	Subnets []string `json:"subnets,omitempty"`
	// SubscriptionURL, when set, is a remote hosts/AdBlock/plain-domain list
	// fetched at apply time instead of (or in addition to) the inline Domains
	// (e.g. the itdoginfo "blocked-in-RU" master list).
	SubscriptionURL string `json:"subscription_url,omitempty"`
	// Covers names the member preset IDs a composite bundles (informational;
	// Domains is already the pre-expanded union).
	Covers []string `json:"covers,omitempty"`
}

// DomainCount / SubnetCount are convenience counts for the catalog view.
func (p Preset) DomainCount() int { return len(p.Domains) }
func (p Preset) SubnetCount() int { return len(p.Subnets) }

var (
	loadOnce sync.Once
	catalog  []Preset
	byID     map[string]Preset
	loadErr  error
)

func load() {
	loadOnce.Do(func() {
		if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
			loadErr = fmt.Errorf("presets: decode embedded catalog: %w", err)
			return
		}
		byID = make(map[string]Preset, len(catalog))
		for _, p := range catalog {
			byID[p.ID] = p
		}
	})
}

// Catalog returns the full preset list in display order (category, then name).
func Catalog() []Preset {
	load()
	out := make([]Preset, len(catalog))
	copy(out, catalog)
	return out
}

// ByID returns the preset with the given id.
func ByID(id string) (Preset, bool) {
	load()
	p, ok := byID[strings.TrimSpace(id)]
	return p, ok
}

// Categories returns the distinct categories present, in display order.
func Categories() []string {
	load()
	order := []string{"social", "media", "ai", "gaming", "developer", "cloud", "block"}
	seen := map[string]bool{}
	for _, p := range catalog {
		seen[p.Category] = true
	}
	var out []string
	for _, c := range order {
		if seen[c] {
			out = append(out, c)
			delete(seen, c)
		}
	}
	// Any category not in the known order, appended alphabetically.
	var rest []string
	for c := range seen {
		rest = append(rest, c)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// Err reports a catalog load/parse error (nil once loaded successfully).
func Err() error {
	load()
	return loadErr
}
