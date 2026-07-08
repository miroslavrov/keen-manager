package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Capabilities captures the device facts that gate which AWG features this
// package may use against a given router.
type Capabilities struct {
	// Release is the raw firmware release string as reported by
	// "show version", e.g. "5.01.A.3", "5.01.B.1", "5.01.03", "5.02", "6.0".
	Release string

	// Components is the parsed "ndw.components" (or equivalent) list, e.g.
	// ["wireguard", "amneziawg", "schedule", ...]. Comma-separated in the raw
	// RCI response; DetectCapabilities splits it for you.
	Components []string

	// SupportsAWG2 is true when Release is new enough to accept the AWG2
	// "asc" fields s3/s4 (see isAtLeast501A3).
	SupportsAWG2 bool

	// HasWireguard reports whether the "wireguard" NDMS component is present
	// on the device at all. A false here means neither AWG1 nor AWG2 native
	// interfaces are available and Entware's awg-quick fallback (see the
	// sibling internal/awg package) should be used instead.
	HasWireguard bool
}

// versionResponse is the subset of "GET /show/version" fields we care about.
// KeeneticOS has used slightly different shapes for this across firmware
// generations, so several aliases are decoded defensively.
type versionResponse struct {
	Release string `json:"release"`
	Title   string `json:"title"`
	NDW     struct {
		Components string `json:"components"`
	} `json:"ndw"`
	// Some firmware exposes components at the top level instead of nested
	// under "ndw".
	Components string `json:"components"`
}

// DetectCapabilities queries "GET /show/version" and derives Capabilities
// from the reported release string and component list.
func DetectCapabilities(ctx context.Context, c *Client) (Capabilities, error) {
	raw, err := c.Get(ctx, "/show/version")
	if err != nil {
		return Capabilities{}, fmt.Errorf("keenetic: detect capabilities: %w", err)
	}

	var v versionResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return Capabilities{}, fmt.Errorf("keenetic: decode /show/version: %w", err)
	}

	release := v.Release
	if release == "" {
		// Older/alternate firmware sometimes only populates "title" with the
		// release baked in (e.g. "KeeneticOS 5.01.A.3.0-r1"); try to salvage
		// a version-looking token from it as a last resort.
		release = extractVersionToken(v.Title)
	}

	componentsRaw := v.NDW.Components
	if componentsRaw == "" {
		componentsRaw = v.Components
	}
	components := splitComponents(componentsRaw)

	hasWireguard := false
	for _, comp := range components {
		if strings.EqualFold(comp, "wireguard") {
			hasWireguard = true
			break
		}
	}

	return Capabilities{
		Release:      release,
		Components:   components,
		SupportsAWG2: isAtLeast501A3(release),
		HasWireguard: hasWireguard,
	}, nil
}

func splitComponents(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// extractVersionToken pulls the first dotted-numeric-looking token out of a
// free-form string such as a firmware title. It is a best-effort fallback
// only used when the structured "release" field is absent.
func extractVersionToken(s string) string {
	for _, field := range strings.Fields(s) {
		field = strings.Trim(field, "()[]v")
		if field == "" {
			continue
		}
		if field[0] >= '0' && field[0] <= '9' && strings.Contains(field, ".") {
			return field
		}
	}
	return ""
}

// keeneticVersion is a parsed Keenetic release string. KeeneticOS uses a
// distinctive scheme that mixes a numeric "major.minor" with either:
//
//   - an alpha/beta channel marker + build ("5.01.A.3", "5.01.B.1"), or
//   - a plain numeric patch for final releases ("5.01.03"), or
//   - just "major.minor" once the leading zero and channel are dropped
//     entirely by later branches ("5.02", "6.0").
//
// isAtLeast501A3 below is the only consumer; it composes a keeneticVersion
// and compares it against the "5.01.A.3" cutoff using a well-defined channel
// ordering (final > beta > alpha) so that any alpha build at/after A.3, any
// beta build at all, and any final/newer release all compare as "at least".
type keeneticVersion struct {
	major int
	minor int
	// channel: 0 = alpha, 1 = beta, 2 = final (no channel marker, or a
	// release so new it no longer carries one at all).
	channel int
	// build is the alpha/beta build number (the trailing ".3" in "5.01.A.3"),
	// or the numeric patch for a final release (the "03" in "5.01.03").
	build int
}

// parseKeeneticVersion parses the Keenetic release formats documented on
// isAtLeast501A3. It returns ok=false for anything it cannot confidently
// parse (including the empty string), and callers should treat that as "does
// not meet the cutoff" rather than erroring, since an undetectable version is
// most likely very old firmware that predates structured release strings.
func parseKeeneticVersion(release string) (keeneticVersion, bool) {
	release = strings.TrimSpace(release)
	if release == "" {
		return keeneticVersion{}, false
	}

	segs := strings.Split(release, ".")
	if len(segs) < 2 {
		return keeneticVersion{}, false
	}

	major, err := strconv.Atoi(segs[0])
	if err != nil {
		return keeneticVersion{}, false
	}
	minor, err := strconv.Atoi(strings.TrimLeft(segs[1], "0"))
	if err != nil {
		// A minor segment of exactly "00" trims to "" above; that's a valid
		// zero, not a parse failure.
		if segs[1] != "" && strings.Trim(segs[1], "0") == "" {
			minor = 0
		} else {
			return keeneticVersion{}, false
		}
	}

	v := keeneticVersion{major: major, minor: minor, channel: 2}

	switch {
	case len(segs) >= 4 && strings.EqualFold(segs[2], "A"):
		build, err := strconv.Atoi(segs[3])
		if err != nil {
			return keeneticVersion{}, false
		}
		v.channel, v.build = 0, build
	case len(segs) >= 4 && strings.EqualFold(segs[2], "B"):
		build, err := strconv.Atoi(segs[3])
		if err != nil {
			return keeneticVersion{}, false
		}
		v.channel, v.build = 1, build
	case len(segs) == 3:
		// Final release with a numeric patch segment, e.g. "5.01.03".
		build, err := strconv.Atoi(segs[2])
		if err != nil {
			return keeneticVersion{}, false
		}
		v.channel, v.build = 2, build
	case len(segs) == 2:
		// Bare "major.minor" final release, e.g. "5.02", "6.0".
		v.channel, v.build = 2, 0
	default:
		return keeneticVersion{}, false
	}

	return v, true
}

// compare returns -1, 0, or 1 as v is less than, equal to, or greater than
// other, ordering by (major, minor, channel, build) with channel ordered
// alpha < beta < final.
func (v keeneticVersion) compare(other keeneticVersion) int {
	if v.major != other.major {
		return cmpInt(v.major, other.major)
	}
	if v.minor != other.minor {
		return cmpInt(v.minor, other.minor)
	}
	if v.channel != other.channel {
		return cmpInt(v.channel, other.channel)
	}
	return cmpInt(v.build, other.build)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// awg2Cutoff is the minimum firmware version that accepts AWG2's extended asc
// fields (s3, s4).
var awg2Cutoff = keeneticVersion{major: 5, minor: 1, channel: 0, build: 3} // 5.01.A.3

// isAtLeast501A3 reports whether release is new enough to support native
// AWG2 (the s3/s4 obfuscation parameters), per the following rule:
//
//   - any 5.01.A.N alpha build with N >= 3                  -> true
//   - any 5.01.B.N beta build (channel newer than alpha)    -> true
//   - any 5.01.NN final release ("5.01.03" and up)          -> true
//   - any 5.02+ or 6.x release                              -> true
//   - everything else (including unparsable/empty strings)  -> false
//
// The comparison is channel-aware: within the same major.minor, alpha <
// beta < final, so e.g. "5.01.B.1" and "5.01.03" both compare as "at least
// 5.01.A.3" even though their build numbers alone wouldn't reach 3.
func isAtLeast501A3(release string) bool {
	v, ok := parseKeeneticVersion(release)
	if !ok {
		return false
	}
	return v.compare(awg2Cutoff) >= 0
}
