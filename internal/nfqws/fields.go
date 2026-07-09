package nfqws

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// confFieldKind classifies how a settable field's CLI value is parsed/validated.
type confFieldKind int

const (
	fieldString confFieldKind = iota
	fieldInt
	fieldBool
)

// ConfField describes one user-editable nfqws2.conf field: the JSON key that
// SaveNfqwsConfigStructured expects (the same wire key as
// PUT /api/nfqws/config/structured) and a short help line for the CLI.
type ConfField struct {
	JSONKey string
	Help    string
	kind    confFieldKind
}

// confFields maps friendly CLI names to the structured field they set. These are
// exactly the names `keen-manager nfqws set <name> <value>` accepts. The mode
// macro (NFQWS_EXTRA_ARGS) is intentionally omitted here: `nfqws mode` owns it
// (it wraps the value as "$MODE_*"), so exposing it twice would be a footgun.
var confFields = map[string]ConfField{
	"isp-interface":  {"isp_interface", "WAN interface nfqws2 filters on (blank = auto-detect)", fieldString},
	"tcp-ports":      {"tcp_ports", "TCP ports to process, e.g. 80,443", fieldString},
	"udp-ports":      {"udp_ports", "UDP ports to process, e.g. 443,50000-50100", fieldString},
	"policy":         {"policy_name", "Keenetic policy to bind nfqws2 to", fieldString},
	"policy-exclude": {"policy_exclude", "1 to exclude the named policy instead of include it", fieldInt},
	"nfqueue":        {"nfqueue_num", "NFQUEUE number the netfilter hook enqueues to", fieldInt},
	"log-level":      {"log_level", "0=off, 1=info, 2=debug", fieldInt},
	"ipv6":           {"ipv6_enabled", "on|off — also process IPv6", fieldBool},
	"base-args":      {"nfqws_base_args", "base desync args prepended to every profile", fieldString},
	"args":           {"nfqws_args", "primary TCP/TLS desync strategy", fieldString},
	"args-quic":      {"nfqws_args_quic", "QUIC desync strategy", fieldString},
	"args-udp":       {"nfqws_args_udp", "generic UDP desync strategy", fieldString},
	"args-ipset":     {"nfqws_args_ipset", "ipset-scoped desync strategy", fieldString},
	"args-custom":    {"nfqws_args_custom", "custom strategy block", fieldString},
}

// ConfFieldNames returns the settable field names in sorted order.
func ConfFieldNames() []string {
	names := make([]string, 0, len(confFields))
	for n := range confFields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ConfFieldHelp returns aligned "  name   help" lines for CLI usage output.
func ConfFieldHelp() string {
	names := ConfFieldNames()
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, n, confFields[n].Help)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ParseConfField validates a `set <name> <value>` pair against the structured
// schema and returns the JSON key plus a typed value ready to hand to
// SaveNfqwsConfigStructured. Unknown names and malformed values are rejected
// with a message that names the accepted forms; this keeps main.go's CLI layer
// thin and lets the mapping be unit-tested without a device.
func ParseConfField(name, value string) (jsonKey string, val any, err error) {
	f, ok := confFields[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", nil, fmt.Errorf("unknown field %q (known: %s)", name, strings.Join(ConfFieldNames(), ", "))
	}
	switch f.kind {
	case fieldInt:
		n, convErr := strconv.Atoi(strings.TrimSpace(value))
		if convErr != nil {
			return "", nil, fmt.Errorf("%s expects an integer, got %q", name, value)
		}
		if n < 0 {
			return "", nil, fmt.Errorf("%s must be >= 0, got %d", name, n)
		}
		return f.JSONKey, n, nil
	case fieldBool:
		b, boolErr := parseOnOff(value)
		if boolErr != nil {
			return "", nil, fmt.Errorf("%s expects on|off, got %q", name, value)
		}
		return f.JSONKey, b, nil
	default:
		return f.JSONKey, value, nil
	}
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "yes", "enable", "1":
		return true, nil
	case "off", "false", "no", "disable", "0":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean")
}
