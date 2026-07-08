package nfqws

import (
	"regexp"
	"strconv"
	"strings"
)

// Conf is the structured view of an nfqws2.conf. It models the shell keys the
// UI needs to edit as typed fields while preserving everything else (comments,
// ordering, and any keys keen-manager doesn't model) across a parse/Render
// round-trip. The big multiline strategy blocks (NFQWS_ARGS, …) are captured as
// strings so they can be shown, but are only rewritten when actually changed —
// so a form that only touches ports/interface never reflows the strategy text.
type Conf struct {
	ISPInterface    string `json:"isp_interface"`
	NfqwsBaseArgs   string `json:"nfqws_base_args"`
	NfqwsArgs       string `json:"nfqws_args"`
	NfqwsArgsQUIC   string `json:"nfqws_args_quic"`
	NfqwsArgsUDP    string `json:"nfqws_args_udp"`
	NfqwsArgsIPSet  string `json:"nfqws_args_ipset"`
	NfqwsArgsCustom string `json:"nfqws_args_custom"`
	// NfqwsExtraArgs is the active mode macro, e.g. "$MODE_AUTO".
	NfqwsExtraArgs string `json:"nfqws_extra_args"`
	TCPPorts       string `json:"tcp_ports"`
	UDPPorts       string `json:"udp_ports"`
	PolicyName     string `json:"policy_name"`
	PolicyExclude  int    `json:"policy_exclude"`
	NfqueueNum     int    `json:"nfqueue_num"`
	LogLevel       int    `json:"log_level"`
	IPv6Enabled    bool   `json:"ipv6_enabled"`

	// spans records the [start,end] (inclusive) line indices each known key
	// occupied in the parsed text, so Render can splice replacements precisely
	// (including multiline quoted values). Unexported: internal to round-trip.
	spans map[string]lineSpan
}

type lineSpan struct{ start, end int }

// knownKeys is the ordered set of shell keys Conf models (used for appends).
var knownKeys = []string{
	"ISP_INTERFACE", "NFQWS_BASE_ARGS", "NFQWS_ARGS", "NFQWS_ARGS_QUIC",
	"NFQWS_ARGS_UDP", "NFQWS_ARGS_IPSET", "NFQWS_ARGS_CUSTOM", "NFQWS_EXTRA_ARGS",
	"TCP_PORTS", "UDP_PORTS", "POLICY_NAME", "POLICY_EXCLUDE", "NFQUEUE_NUM",
	"LOG_LEVEL", "IPV6_ENABLED",
}

// quotedKeys are emitted as KEY="value"; the rest as bare KEY=value, matching
// the upstream nfqws2.conf convention.
var quotedKeys = map[string]bool{
	"ISP_INTERFACE": true, "NFQWS_BASE_ARGS": true, "NFQWS_ARGS": true,
	"NFQWS_ARGS_QUIC": true, "NFQWS_ARGS_UDP": true, "NFQWS_ARGS_IPSET": true,
	"NFQWS_ARGS_CUSTOM": true, "NFQWS_EXTRA_ARGS": true, "POLICY_NAME": true,
}

var assignRe = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$`)

// ParseConf parses nfqws2.conf text into a structured Conf. It is quote-aware:
// a value opened with " may span multiple lines until the closing ". Unknown
// keys, comments and blank lines are ignored for the typed fields but preserved
// by Render.
func ParseConf(raw string) (Conf, error) {
	c := Conf{spans: map[string]lineSpan{}}
	lines := strings.Split(raw, "\n")
	vals := map[string]string{}

	for i := 0; i < len(lines); i++ {
		m := assignRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		key := m[1]
		rest := strings.TrimLeft(m[2], " \t")
		start := i
		var val string

		if strings.HasPrefix(rest, `"`) {
			body := rest[1:]
			if idx := strings.IndexByte(body, '"'); idx >= 0 {
				val = body[:idx] // closes on the same line
			} else {
				var sb strings.Builder
				sb.WriteString(body)
				for j := i + 1; j < len(lines); j++ {
					if idx := strings.IndexByte(lines[j], '"'); idx >= 0 {
						sb.WriteString("\n")
						sb.WriteString(lines[j][:idx])
						i = j // consume through the closing-quote line
						break
					}
					sb.WriteString("\n")
					sb.WriteString(lines[j])
				}
				val = sb.String()
			}
		} else {
			val = strings.TrimSpace(rest)
		}

		vals[key] = val
		if _, seen := c.spans[key]; !seen {
			c.spans[key] = lineSpan{start: start, end: i}
		}
	}

	c.apply(vals)
	return c, nil
}

func (c *Conf) apply(v map[string]string) {
	c.ISPInterface = v["ISP_INTERFACE"]
	c.NfqwsBaseArgs = v["NFQWS_BASE_ARGS"]
	c.NfqwsArgs = v["NFQWS_ARGS"]
	c.NfqwsArgsQUIC = v["NFQWS_ARGS_QUIC"]
	c.NfqwsArgsUDP = v["NFQWS_ARGS_UDP"]
	c.NfqwsArgsIPSet = v["NFQWS_ARGS_IPSET"]
	c.NfqwsArgsCustom = v["NFQWS_ARGS_CUSTOM"]
	c.NfqwsExtraArgs = v["NFQWS_EXTRA_ARGS"]
	c.TCPPorts = v["TCP_PORTS"]
	c.UDPPorts = v["UDP_PORTS"]
	c.PolicyName = v["POLICY_NAME"]
	c.PolicyExclude = atoiDefault(v["POLICY_EXCLUDE"])
	c.NfqueueNum = atoiDefault(v["NFQUEUE_NUM"])
	c.LogLevel = atoiDefault(v["LOG_LEVEL"])
	c.IPv6Enabled = v["IPV6_ENABLED"] == "1" || strings.EqualFold(v["IPV6_ENABLED"], "true")
}

// values returns the canonical string form of every known key for the current
// struct (used both for change detection and for emission).
func (c Conf) values() map[string]string {
	return map[string]string{
		"ISP_INTERFACE":     c.ISPInterface,
		"NFQWS_BASE_ARGS":   c.NfqwsBaseArgs,
		"NFQWS_ARGS":        c.NfqwsArgs,
		"NFQWS_ARGS_QUIC":   c.NfqwsArgsQUIC,
		"NFQWS_ARGS_UDP":    c.NfqwsArgsUDP,
		"NFQWS_ARGS_IPSET":  c.NfqwsArgsIPSet,
		"NFQWS_ARGS_CUSTOM": c.NfqwsArgsCustom,
		"NFQWS_EXTRA_ARGS":  c.NfqwsExtraArgs,
		"TCP_PORTS":         c.TCPPorts,
		"UDP_PORTS":         c.UDPPorts,
		"POLICY_NAME":       c.PolicyName,
		"POLICY_EXCLUDE":    strconv.Itoa(c.PolicyExclude),
		"NFQUEUE_NUM":       strconv.Itoa(c.NfqueueNum),
		"LOG_LEVEL":         strconv.Itoa(c.LogLevel),
		"IPV6_ENABLED":      boolTo10(c.IPv6Enabled),
	}
}

// Render serialises the Conf back over original, updating in place ONLY the
// known keys whose value changed (comparing against a fresh parse of original),
// appending any known key that was set but absent. Every other line — comments,
// unknown keys, blank lines, ordering, and the exact text of unchanged multiline
// values — is preserved verbatim.
func (c Conf) Render(original string) string {
	base, _ := ParseConf(original)
	cur := c.values()
	prev := base.values()
	lines := strings.Split(original, "\n")

	type splice struct {
		end  int
		text string
	}
	splices := map[int]splice{}
	for key, span := range base.spans {
		if _, known := cur[key]; !known {
			continue // an unknown key that happens to have a span — leave it
		}
		if cur[key] != prev[key] {
			splices[span.start] = splice{end: span.end, text: formatAssign(key, cur[key])}
		}
	}

	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if s, ok := splices[i]; ok {
			out = append(out, s.text)
			i = s.end // skip the replaced span's original lines
			continue
		}
		out = append(out, lines[i])
	}

	// Append known keys that were absent from the original but are set now.
	for _, key := range knownKeys {
		if _, had := base.spans[key]; had {
			continue
		}
		if v := cur[key]; v != "" && v != "0" {
			out = append(out, formatAssign(key, v))
		}
	}
	return strings.Join(out, "\n")
}

func formatAssign(key, val string) string {
	if quotedKeys[key] {
		return key + `="` + val + `"`
	}
	return key + "=" + val
}

func atoiDefault(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func boolTo10(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
