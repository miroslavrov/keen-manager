// Package awg parses and generates AmneziaWG configurations and manages AWG
// tunnels on the device. Parse/generate are pure and unit-tested; control
// functions perform device-side effects through platform.Runner (dry-run aware).
package awg

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// Parse reads a wg-quick / AmneziaWG .conf into an AWGConfig.
func Parse(text string) (*model.AWGConfig, error) {
	cfg := &model.AWGConfig{}
	section := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])

		switch section {
		case "interface":
			applyInterface(cfg, key, val)
		case "peer":
			applyPeer(&cfg.Peer, key, val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, Validate(cfg)
}

func applyInterface(cfg *model.AWGConfig, key, val string) {
	switch strings.ToLower(key) {
	case "privatekey":
		cfg.PrivateKey = val
	case "address":
		cfg.Address = splitList(val)
	case "dns":
		cfg.DNS = splitList(val)
	case "mtu":
		cfg.MTU = atoi(val)
	case "listenport":
		cfg.ListenPort = atoi(val)
	case "jc":
		cfg.Jc = atoi(val)
	case "jmin":
		cfg.Jmin = atoi(val)
	case "jmax":
		cfg.Jmax = atoi(val)
	case "s1":
		cfg.S1 = atoi(val)
	case "s2":
		cfg.S2 = atoi(val)
	case "s3":
		cfg.S3 = atoi(val)
	case "s4":
		cfg.S4 = atoi(val)
	case "h1":
		cfg.H1 = atoi64(val)
	case "h2":
		cfg.H2 = atoi64(val)
	case "h3":
		cfg.H3 = atoi64(val)
	case "h4":
		cfg.H4 = atoi64(val)
	case "i1":
		cfg.I1 = val
	case "i2":
		cfg.I2 = val
	case "i3":
		cfg.I3 = val
	case "i4":
		cfg.I4 = val
	case "i5":
		cfg.I5 = val
	}
}

func applyPeer(p *model.AWGPeer, key, val string) {
	switch strings.ToLower(key) {
	case "publickey":
		p.PublicKey = val
	case "presharedkey":
		p.PresharedKey = val
	case "endpoint":
		p.Endpoint = val
	case "allowedips":
		p.AllowedIPs = splitList(val)
	case "persistentkeepalive":
		p.PersistentKeepalive = atoi(val)
	}
}

// Validate checks the minimum fields required to bring a tunnel up.
func Validate(cfg *model.AWGConfig) error {
	if cfg.PrivateKey == "" {
		return fmt.Errorf("interface: missing PrivateKey")
	}
	if len(cfg.Address) == 0 {
		return fmt.Errorf("interface: missing Address")
	}
	if cfg.Peer.PublicKey == "" {
		return fmt.Errorf("peer: missing PublicKey")
	}
	if cfg.Peer.Endpoint == "" {
		return fmt.Errorf("peer: missing Endpoint")
	}
	return nil
}

// Generate renders an AWGConfig back to .conf text. The obfuscation params are
// always emitted together (Keenetic's NDMS import rejects partial AWG configs).
func Generate(cfg *model.AWGConfig) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", cfg.PrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", strings.Join(cfg.Address, ", "))
	if len(cfg.DNS) > 0 {
		fmt.Fprintf(&b, "DNS = %s\n", strings.Join(cfg.DNS, ", "))
	}
	if cfg.MTU > 0 {
		fmt.Fprintf(&b, "MTU = %d\n", cfg.MTU)
	}
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&b, "ListenPort = %d\n", cfg.ListenPort)
	}
	// AWG obfuscation (base set always present)
	fmt.Fprintf(&b, "Jc = %d\n", cfg.Jc)
	fmt.Fprintf(&b, "Jmin = %d\n", cfg.Jmin)
	fmt.Fprintf(&b, "Jmax = %d\n", cfg.Jmax)
	fmt.Fprintf(&b, "S1 = %d\n", cfg.S1)
	fmt.Fprintf(&b, "S2 = %d\n", cfg.S2)
	fmt.Fprintf(&b, "H1 = %d\n", cfg.H1)
	fmt.Fprintf(&b, "H2 = %d\n", cfg.H2)
	fmt.Fprintf(&b, "H3 = %d\n", cfg.H3)
	fmt.Fprintf(&b, "H4 = %d\n", cfg.H4)
	// AWG v2 extended (only when set)
	if cfg.S3 > 0 {
		fmt.Fprintf(&b, "S3 = %d\n", cfg.S3)
	}
	if cfg.S4 > 0 {
		fmt.Fprintf(&b, "S4 = %d\n", cfg.S4)
	}
	for k, v := range map[string]string{"I1": cfg.I1, "I2": cfg.I2, "I3": cfg.I3, "I4": cfg.I4, "I5": cfg.I5} {
		if v != "" {
			fmt.Fprintf(&b, "%s = %s\n", k, v)
		}
	}

	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", cfg.Peer.PublicKey)
	if cfg.Peer.PresharedKey != "" {
		fmt.Fprintf(&b, "PresharedKey = %s\n", cfg.Peer.PresharedKey)
	}
	fmt.Fprintf(&b, "Endpoint = %s\n", cfg.Peer.Endpoint)
	allowed := cfg.Peer.AllowedIPs
	if len(allowed) == 0 {
		allowed = []string{"0.0.0.0/0", "::/0"}
	}
	fmt.Fprintf(&b, "AllowedIPs = %s\n", strings.Join(allowed, ", "))
	ka := cfg.Peer.PersistentKeepalive
	if ka == 0 {
		ka = 25
	}
	fmt.Fprintf(&b, "PersistentKeepalive = %d\n", ka)
	return b.String()
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
