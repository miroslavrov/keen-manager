// Package subscription fetches and parses Xray subscription URLs and individual
// share links (vless/vmess/trojan/ss) into model.Server values. All parsing is
// pure and unit-tested; no device state is touched here.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// ParseLink parses a single share link into a Server. Supported schemes:
// vless, vmess, trojan, ss.
func ParseLink(raw string) (*model.Server, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "vless://"):
		return parseVLESS(raw)
	case strings.HasPrefix(raw, "vmess://"):
		return parseVMess(raw)
	case strings.HasPrefix(raw, "trojan://"):
		return parseTrojan(raw)
	case strings.HasPrefix(raw, "ss://"):
		return parseSS(raw)
	default:
		return nil, fmt.Errorf("unsupported share link scheme")
	}
}

func parseVLESS(raw string) (*model.Server, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("vless parse: %w", err)
	}
	port, _ := strconv.Atoi(u.Port())
	q := u.Query()
	s := &model.Server{
		Protocol:    model.ProtoVLESS,
		Address:     u.Hostname(),
		Port:        port,
		UUID:        u.User.Username(),
		Flow:        q.Get("flow"),
		Security:    def(q.Get("security"), "none"),
		Network:     def(q.Get("type"), "tcp"),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		PublicKey:   q.Get("pbk"),
		ShortID:     q.Get("sid"),
		SpiderX:     q.Get("spx"),
		Host:        q.Get("host"),
		Raw:         raw,
	}
	// path / serviceName depending on network
	if sn := q.Get("serviceName"); sn != "" {
		s.Path = sn
	} else {
		s.Path = q.Get("path")
	}
	if alpn := q.Get("alpn"); alpn != "" {
		s.ALPN = splitComma(alpn)
	}
	if s.Security == "reality" && s.SpiderX == "" {
		s.SpiderX = "/"
	}
	applyLabel(s, u.Fragment)
	return s, validate(s)
}

// vmessJSON is the 2dust/v2rayN vmess share-link JSON body.
type vmessJSON struct {
	V    any    `json:"v"`
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	ID   string `json:"id"`
	Aid  any    `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
	FP   string `json:"fp"`
}

func parseVMess(raw string) (*model.Server, error) {
	body := strings.TrimPrefix(raw, "vmess://")
	dec, err := b64lenient(body)
	if err != nil {
		return nil, fmt.Errorf("vmess base64: %w", err)
	}
	var v vmessJSON
	if err := json.Unmarshal(dec, &v); err != nil {
		return nil, fmt.Errorf("vmess json: %w", err)
	}
	s := &model.Server{
		Protocol:    model.ProtoVMess,
		Address:     v.Add,
		Port:        toInt(v.Port),
		UUID:        v.ID,
		AlterID:     toInt(v.Aid),
		Cipher:      def(v.Scy, "auto"),
		Network:     def(v.Net, "tcp"),
		Host:        v.Host,
		Path:        v.Path,
		SNI:         v.SNI,
		Fingerprint: v.FP,
		Raw:         raw,
	}
	if v.TLS == "tls" {
		s.Security = "tls"
	} else {
		s.Security = "none"
	}
	if v.ALPN != "" {
		s.ALPN = splitComma(v.ALPN)
	}
	applyLabel(s, v.PS)
	return s, validate(s)
}

func parseTrojan(raw string) (*model.Server, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("trojan parse: %w", err)
	}
	port, _ := strconv.Atoi(u.Port())
	q := u.Query()
	s := &model.Server{
		Protocol:    model.ProtoTrojan,
		Address:     u.Hostname(),
		Port:        port,
		Password:    u.User.Username(),
		Security:    def(q.Get("security"), "tls"),
		Network:     def(q.Get("type"), "tcp"),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		Host:        q.Get("host"),
		Path:        q.Get("path"),
		Raw:         raw,
	}
	if alpn := q.Get("alpn"); alpn != "" {
		s.ALPN = splitComma(alpn)
	}
	applyLabel(s, u.Fragment)
	return s, validate(s)
}

func parseSS(raw string) (*model.Server, error) {
	// Two forms:
	//  legacy:  ss://base64(method:password@host:port)#tag
	//  SIP002:  ss://base64url(method:password)@host:port#tag
	rest := strings.TrimPrefix(raw, "ss://")
	frag := ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		frag, rest = rest[i+1:], rest[:i]
	}
	// strip any query (plugin=...) we don't support yet
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest = rest[:i]
	}

	var method, password, host string
	var port int

	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		// SIP002: userinfo before @, host:port after
		userinfo := rest[:at]
		hp := rest[at+1:]
		if dec, err := b64lenient(userinfo); err == nil && strings.Contains(string(dec), ":") {
			userinfo = string(dec)
		}
		mp := strings.SplitN(userinfo, ":", 2)
		if len(mp) != 2 {
			return nil, fmt.Errorf("ss userinfo malformed")
		}
		method, password = mp[0], mp[1]
		host, port = splitHostPort(hp)
	} else {
		// legacy: whole thing is base64(method:password@host:port)
		dec, err := b64lenient(rest)
		if err != nil {
			return nil, fmt.Errorf("ss base64: %w", err)
		}
		body := string(dec)
		at := strings.LastIndexByte(body, '@')
		if at < 0 {
			return nil, fmt.Errorf("ss legacy malformed")
		}
		mp := strings.SplitN(body[:at], ":", 2)
		if len(mp) != 2 {
			return nil, fmt.Errorf("ss legacy creds malformed")
		}
		method, password = mp[0], mp[1]
		host, port = splitHostPort(body[at+1:])
	}

	s := &model.Server{
		Protocol: model.ProtoSS,
		Address:  host,
		Port:     port,
		Cipher:   method,
		Password: password,
		Network:  "tcp",
		Security: "none",
		Raw:      raw,
	}
	applyLabel(s, frag)
	return s, validate(s)
}

// --- helpers ---

func validate(s *model.Server) error {
	if s.Address == "" || s.Port == 0 {
		return fmt.Errorf("%s: missing address/port", s.Protocol)
	}
	switch s.Protocol {
	case model.ProtoVLESS, model.ProtoVMess:
		if s.UUID == "" {
			return fmt.Errorf("%s: missing uuid", s.Protocol)
		}
	case model.ProtoTrojan, model.ProtoSS:
		if s.Password == "" {
			return fmt.Errorf("%s: missing password", s.Protocol)
		}
	}
	return nil
}

func applyLabel(s *model.Server, frag string) {
	label, _ := url.QueryUnescape(frag)
	label = strings.TrimSpace(label)
	if label == "" {
		label = fmt.Sprintf("%s:%d", s.Address, s.Port)
	}
	s.Name = label
	s.Location = stripFlags(label)
	s.ID = shortID(s.Raw)
}

// stripFlags removes leading Unicode regional-indicator (flag) symbols so the UI
// can render clean location text without emoji.
func stripFlags(str string) string {
	out := make([]rune, 0, len(str))
	for _, r := range str {
		if r >= 0x1F1E6 && r <= 0x1F1FF { // regional indicators
			continue
		}
		if r == 0xFE0F || r == 0x200D { // variation selector / ZWJ
			continue
		}
		out = append(out, r)
	}
	return strings.TrimSpace(string(out))
}

func shortID(seed string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return "s" + strconv.FormatUint(uint64(h.Sum32()), 16)
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitHostPort(hp string) (string, int) {
	hp = strings.Trim(hp, "/")
	if i := strings.LastIndexByte(hp, ':'); i >= 0 {
		host := strings.Trim(hp[:i], "[]")
		port, _ := strconv.Atoi(hp[i+1:])
		return host, port
	}
	return hp, 0
}

func def(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

// b64lenient decodes standard/url-safe base64 with or without padding.
func b64lenient(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	encs := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	var lastErr error
	for _, e := range encs {
		if b, err := e.DecodeString(s); err == nil {
			return b, nil
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}
