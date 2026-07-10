package subscription

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
	"gopkg.in/yaml.v3"
)

// Format identifies the detected subscription body format.
type Format string

const (
	FormatBase64List Format = "base64-list"
	FormatPlainList  Format = "plain-list"
	FormatClashYAML  Format = "clash-yaml"
	FormatSIP008     Format = "sip008"
	FormatUnknown    Format = "unknown"
)

// Detect determines the format of a subscription body.
func Detect(body string) Format {
	t := strings.TrimSpace(body)
	if t == "" {
		return FormatUnknown
	}
	if strings.HasPrefix(t, "proxies:") || strings.Contains(t, "\nproxies:") {
		return FormatClashYAML
	}
	if t[0] == '{' || t[0] == '[' {
		if strings.Contains(t, "\"servers\"") && strings.Contains(t, "\"version\"") {
			return FormatSIP008
		}
		// Could be sing-box/xray JSON; treated as unknown for now.
		return FormatUnknown
	}
	if strings.Contains(t, "://") {
		return FormatPlainList
	}
	if dec, err := b64lenient(t); err == nil && strings.Contains(string(dec), "://") {
		return FormatBase64List
	}
	return FormatUnknown
}

// ParseBody parses a subscription body into a list of servers, auto-detecting
// the format. Unparseable individual links are skipped (best-effort).
func ParseBody(body string) ([]model.Server, Format, error) {
	f := Detect(body)
	switch f {
	case FormatBase64List:
		dec, err := b64lenient(strings.TrimSpace(body))
		if err != nil {
			return nil, f, err
		}
		return parseLinkList(string(dec)), f, nil
	case FormatPlainList:
		return parseLinkList(body), f, nil
	case FormatClashYAML:
		srv, err := parseClash(body)
		return srv, f, err
	case FormatSIP008:
		srv, err := parseSIP008(body)
		return srv, f, err
	default:
		// Last resort: maybe it's a single link.
		if s, err := ParseLink(strings.TrimSpace(body)); err == nil {
			return []model.Server{*s}, FormatPlainList, nil
		}
		return nil, FormatUnknown, fmt.Errorf("unrecognized subscription format")
	}
}

func parseLinkList(text string) []model.Server {
	var out []model.Server
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if s, err := ParseLink(line); err == nil {
			out = append(out, *s)
		}
	}
	return out
}

// --- Clash / Clash.Meta YAML ---

type clashDoc struct {
	Proxies []map[string]any `yaml:"proxies"`
}

func parseClash(body string) ([]model.Server, error) {
	var doc clashDoc
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("clash yaml: %w", err)
	}
	var out []model.Server
	for _, p := range doc.Proxies {
		if s := clashProxyToServer(p); s != nil {
			out = append(out, *s)
		}
	}
	return out, nil
}

func clashProxyToServer(p map[string]any) *model.Server {
	typ := ystr(p["type"])
	s := &model.Server{
		Address:     ystr(p["server"]),
		Port:        yint(p["port"]),
		Name:        ystr(p["name"]),
		Network:     def(ystr(p["network"]), "tcp"),
		SNI:         firstNonEmpty(ystr(p["servername"]), ystr(p["sni"])),
		Fingerprint: ystr(p["client-fingerprint"]),
	}
	s.Location = stripFlags(s.Name)
	if b, ok := p["tls"].(bool); ok && b {
		s.Security = "tls"
	} else {
		s.Security = "none"
	}
	// reality-opts
	if ro, ok := p["reality-opts"].(map[string]any); ok {
		s.Security = "reality"
		s.PublicKey = canonRealityKey(ystr(ro["public-key"]))
		s.ShortID = strings.TrimSpace(ystr(ro["short-id"]))
		if s.SpiderX == "" {
			s.SpiderX = "/"
		}
	}
	// ws-opts / grpc-opts
	if wo, ok := p["ws-opts"].(map[string]any); ok {
		s.Path = ystr(wo["path"])
		if h, ok := wo["headers"].(map[string]any); ok {
			s.Host = ystr(h["Host"])
		}
	}
	if go_, ok := p["grpc-opts"].(map[string]any); ok {
		s.Path = ystr(go_["grpc-service-name"])
	}
	if alpn, ok := p["alpn"].([]any); ok {
		for _, a := range alpn {
			s.ALPN = append(s.ALPN, ystr(a))
		}
	}

	switch typ {
	case "vless":
		s.Protocol = model.ProtoVLESS
		s.UUID = ystr(p["uuid"])
		s.Flow = ystr(p["flow"])
	case "vmess":
		s.Protocol = model.ProtoVMess
		s.UUID = ystr(p["uuid"])
		s.AlterID = yint(p["alterId"])
		s.Cipher = def(ystr(p["cipher"]), "auto")
	case "trojan":
		s.Protocol = model.ProtoTrojan
		s.Password = ystr(p["password"])
		if s.Security == "none" {
			s.Security = "tls"
		}
	case "ss":
		s.Protocol = model.ProtoSS
		s.Password = ystr(p["password"])
		s.Cipher = ystr(p["cipher"])
	default:
		return nil // unsupported (e.g. hysteria2) — skipped
	}
	if s.Address == "" || s.Port == 0 {
		return nil
	}
	s.ID = shortID(typ + s.Address + strconv.Itoa(s.Port) + s.Name)
	return s
}

// --- SIP008 (shadowsocks JSON) ---

type sip008 struct {
	Version int `json:"version"`
	Servers []struct {
		ID         string `json:"id"`
		Remarks    string `json:"remarks"`
		Server     string `json:"server"`
		ServerPort int    `json:"server_port"`
		Password   string `json:"password"`
		Method     string `json:"method"`
	} `json:"servers"`
}

func parseSIP008(body string) ([]model.Server, error) {
	var doc sip008
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("sip008: %w", err)
	}
	var out []model.Server
	for _, srv := range doc.Servers {
		s := model.Server{
			Protocol: model.ProtoSS,
			Address:  srv.Server,
			Port:     srv.ServerPort,
			Password: srv.Password,
			Cipher:   srv.Method,
			Name:     firstNonEmpty(srv.Remarks, srv.Server),
			Network:  "tcp",
			Security: "none",
		}
		s.Location = stripFlags(s.Name)
		s.ID = firstNonEmpty(srv.ID, shortID(srv.Server+strconv.Itoa(srv.ServerPort)))
		if s.Address != "" && s.Port != 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

// --- yaml value helpers ---

func ystr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.Itoa(int(x))
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func yint(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(x))
		return i
	}
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
