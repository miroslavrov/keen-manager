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

	// SupportsDNSRoute reports whether the firmware exposes the native
	// domain-routing stack (object-group fqdn + dns-proxy route) used by the
	// Routes/"Маршруты" feature. This landed with KeeneticOS 5.x, so it is
	// gated on a 5.0+ release.
	SupportsDNSRoute bool

	// HasProxyClient reports whether the "Proxy client" system component is
	// installed — the firmware feature (KeeneticOS 3.9+) that lets a proxy
	// server (HTTP/HTTPS/SOCKS5) be added as a first-class connection of
	// interface type "Proxy". keen-manager uses it to expose Xray as a single
	// visible Proxy connection (→ its local SOCKS inbound) instead of the
	// invisible TPROXY capture.
	//
	// Detection is best-effort from the component list (a component whose name
	// contains "proxy"). It is a HINT only: the authoritative gate is whether
	// the router accepts the create — the engine attempts proxy mode when this
	// is true and falls back to TPROXY if the RCI write is rejected. Component
	// naming varies across firmware, so do not treat a false here as definitive
	// without an on-device read-back.
	HasProxyClient bool
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
	hasProxyClient := false
	for _, comp := range components {
		if strings.EqualFold(comp, "wireguard") {
			hasWireguard = true
		}
		// The Proxy client component's NDMS id varies across firmware; match any
		// component whose name mentions "proxy" (best-effort — see HasProxyClient).
		if strings.Contains(strings.ToLower(comp), "proxy") {
			hasProxyClient = true
		}
	}

	return Capabilities{
		Release:          release,
		Components:       components,
		SupportsAWG2:     isAtLeast501A3(release),
		HasWireguard:     hasWireguard,
		SupportsDNSRoute: isAtLeast5(release),
		HasProxyClient:   hasProxyClient,
	}, nil
}

// isAtLeast5 reports whether release is KeeneticOS 5.0 or newer (the floor for
// the native domain-routing stack). Unparsable/empty strings are treated as
// "too old" (conservative: the feature simply won't be offered).
func isAtLeast5(release string) bool {
	v, ok := parseKeeneticVersion(release)
	if !ok {
		return false
	}
	return v.major >= 5
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
//   - a channel letter + build ("5.01.A.3" alpha, "5.01.B.1" beta,
//     "5.01.C.0.0-1" stable/release), or
//   - a plain numeric patch for final releases ("5.01.03"), or
//   - just "major.minor" once the leading zero and channel are dropped
//     entirely by later branches ("5.02", "6.0"), or
//   - a plain "5.1.0" as some firmware/RCI reports it.
//
// KeeneticOS release channels are lettered A = alpha (draft), B = beta
// (preview), C = stable (the shipping release). So the shipping "5.01.C.x"
// firmware is 5.1.0 stable — NEWER than any A/B build of the same major.minor,
// not older. The channel field below orders them accordingly, and any letter
// at or beyond C is treated as the stable tier. Release strings also carry a
// trailing revision ("-1", "-r2") which is stripped before parsing.
//
// isAtLeast501A3 (AWG2 gate) and isAtLeast5 (DNS-route gate) are the consumers;
// isAtLeast501A3 compares against the "5.01.A.3" cutoff using a well-defined
// channel ordering (stable > beta > alpha) so that any alpha build at/after
// A.3, any beta/stable build, and any final/newer release all compare as "at
// least".
type keeneticVersion struct {
	major int
	minor int
	// channel: 0 = alpha (A), 1 = beta (B), 2 = final/stable (C and later
	// letters, a plain numeric patch, or a release so new it carries no
	// channel marker at all).
	channel int
	// build is the alpha/beta build number (the trailing ".3" in "5.01.A.3"),
	// or the numeric patch for a final release (the "03" in "5.01.03"). It is
	// 0 for a stable-channel string whose build segment is absent or 0.
	build int
}

// parseKeeneticVersion parses the Keenetic release formats documented on
// keeneticVersion. It is deliberately lenient: it strips any trailing
// revision/build suffix ("-1", "-r2"), tolerates extra trailing segments, and
// accepts any single-letter channel marker (A/B/C/…) rather than only the
// A/B it historically knew — the shipping "5.01.C.0.0-1" stable firmware
// previously fell through to a parse failure, which silently disabled native
// AWG2 and DNS routing on current KeeneticOS.
//
// It returns ok=false only for genuinely unparseable strings (empty, or no
// numeric major.minor). Callers treat ok=false as "does not meet the cutoff",
// since an undetectable version is most likely very old firmware that predates
// structured release strings.
func parseKeeneticVersion(release string) (keeneticVersion, bool) {
	release = strings.TrimSpace(release)
	if release == "" {
		return keeneticVersion{}, false
	}
	// Drop a leading "v" and any trailing revision/build/space suffix so
	// "5.01.C.0.0-1", "v5.1.0" and "5.02 (AAB)" all reduce to the version core.
	release = strings.TrimPrefix(release, "v")
	if i := strings.IndexAny(release, "-+ \t"); i >= 0 {
		release = release[:i]
	}

	segs := strings.Split(release, ".")
	if len(segs) < 2 {
		return keeneticVersion{}, false
	}

	major, err := strconv.Atoi(segs[0])
	if err != nil {
		return keeneticVersion{}, false
	}
	// strconv.Atoi handles leading zeros ("01" -> 1, "00" -> 0) directly.
	minor, err := strconv.Atoi(segs[1])
	if err != nil {
		return keeneticVersion{}, false
	}

	// Default to the stable/final tier; a recognised channel letter or numeric
	// patch segment refines it below. Extra trailing segments are ignored.
	v := keeneticVersion{major: major, minor: minor, channel: 2}
	if len(segs) >= 3 {
		if ch, ok := channelForLetter(segs[2]); ok {
			v.channel = ch
			if len(segs) >= 4 {
				if b, err := strconv.Atoi(segs[3]); err == nil {
					v.build = b
				}
			}
		} else if b, err := strconv.Atoi(segs[2]); err == nil {
			// Final release with a numeric patch segment, e.g. "5.01.03".
			v.channel, v.build = 2, b
		}
		// An unrecognised third segment (neither a channel letter nor numeric)
		// leaves the default stable tier / build 0 — major.minor still decides.
	}

	return v, true
}

// channelForLetter maps a Keenetic channel marker to a comparable tier:
// A = alpha (0), B = beta (1), and C or any later letter = stable/final (2).
// It returns ok=false when s is not a single ASCII letter, so the caller can
// fall back to interpreting the segment as a numeric patch.
func channelForLetter(s string) (int, bool) {
	if len(s) != 1 {
		return 0, false
	}
	c := s[0]
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	switch {
	case c == 'A':
		return 0, true // alpha / draft
	case c == 'B':
		return 1, true // beta / preview
	case c >= 'C' && c <= 'Z':
		return 2, true // stable / release channel (C is the shipping channel)
	default:
		return 0, false
	}
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
