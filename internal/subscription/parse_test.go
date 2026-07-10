package subscription

import (
	"encoding/base64"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// A real-shape VLESS Reality link (credentials are from the public example format).
const vlessReality = "vless://839d4028-2984-4e66-8e62-f4c127b52f49@109.163.239.98:443?security=reality&encryption=none&fp=firefox&headerType=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-87.yahoo.com&pbk=CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw&sid=07ddc43269d197c0#\U0001F1F3\U0001F1F1 Amsterdam, Extra"

func TestParseVLESSReality(t *testing.T) {
	s, err := ParseLink(vlessReality)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if s.Protocol != model.ProtoVLESS {
		t.Errorf("proto = %s, want vless", s.Protocol)
	}
	if s.Address != "109.163.239.98" || s.Port != 443 {
		t.Errorf("addr:port = %s:%d", s.Address, s.Port)
	}
	if s.UUID != "839d4028-2984-4e66-8e62-f4c127b52f49" {
		t.Errorf("uuid = %s", s.UUID)
	}
	if s.Security != "reality" {
		t.Errorf("security = %s, want reality", s.Security)
	}
	if s.Flow != "xtls-rprx-vision" {
		t.Errorf("flow = %s", s.Flow)
	}
	if s.SNI != "cdn3-87.yahoo.com" {
		t.Errorf("sni = %s", s.SNI)
	}
	if s.PublicKey != "CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw" {
		t.Errorf("pbk = %s", s.PublicKey)
	}
	if s.ShortID != "07ddc43269d197c0" {
		t.Errorf("sid = %s", s.ShortID)
	}
	if s.SpiderX != "/" {
		t.Errorf("spiderX default = %q, want /", s.SpiderX)
	}
	// Flag emoji must be stripped from Location but the raw Name may keep it.
	if s.Location != "Amsterdam, Extra" {
		t.Errorf("location = %q, want %q", s.Location, "Amsterdam, Extra")
	}
	if s.ID == "" {
		t.Error("expected non-empty stable ID")
	}
}

func TestParseBase64List(t *testing.T) {
	list := vlessReality + "\n" +
		"vless://839d4028-2984-4e66-8e62-f4c127b52f49@109.163.239.119:443?security=reality&encryption=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-42.yahoo.com&pbk=CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw&sid=04c9b77ed330452d#JP Tokyo\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(list))

	servers, format, err := ParseBody(b64)
	if err != nil {
		t.Fatalf("ParseBody error: %v", err)
	}
	if format != FormatBase64List {
		t.Errorf("format = %s, want base64-list", format)
	}
	if len(servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(servers))
	}
	if servers[1].Address != "109.163.239.119" {
		t.Errorf("server[1] addr = %s", servers[1].Address)
	}
}

func TestParseVMess(t *testing.T) {
	js := `{"v":"2","ps":"vm-node","add":"1.2.3.4","port":"32000","id":"1386f85e-657b-4d6e-9d56-78badb75e1fd","aid":"0","scy":"auto","net":"ws","type":"none","host":"a.example.com","path":"/ws","tls":"tls","sni":"a.example.com","alpn":"h2,http/1.1","fp":"chrome"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(js))
	s, err := ParseLink(link)
	if err != nil {
		t.Fatalf("vmess parse: %v", err)
	}
	if s.Protocol != model.ProtoVMess || s.Address != "1.2.3.4" || s.Port != 32000 {
		t.Errorf("bad vmess: %+v", s)
	}
	if s.Network != "ws" || s.Path != "/ws" || s.Host != "a.example.com" {
		t.Errorf("bad ws opts: net=%s path=%s host=%s", s.Network, s.Path, s.Host)
	}
	if s.Security != "tls" || len(s.ALPN) != 2 {
		t.Errorf("bad tls/alpn: sec=%s alpn=%v", s.Security, s.ALPN)
	}
}

func TestParseTrojan(t *testing.T) {
	s, err := ParseLink("trojan://secretpass@example.com:443?security=tls&sni=example.com&type=tcp#TR")
	if err != nil {
		t.Fatalf("trojan parse: %v", err)
	}
	if s.Protocol != model.ProtoTrojan || s.Password != "secretpass" || s.SNI != "example.com" {
		t.Errorf("bad trojan: %+v", s)
	}
}

func TestParseSSSIP002(t *testing.T) {
	// ss://base64url(method:password)@host:port#tag
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:test"))
	s, err := ParseLink("ss://" + userinfo + "@192.168.100.1:8888#Example1")
	if err != nil {
		t.Fatalf("ss parse: %v", err)
	}
	if s.Cipher != "aes-128-gcm" || s.Password != "test" || s.Address != "192.168.100.1" || s.Port != 8888 {
		t.Errorf("bad ss: %+v", s)
	}
}

func TestDetect(t *testing.T) {
	cases := map[string]Format{
		"proxies:\n  - name: a":      FormatClashYAML,
		"vless://x@a:1":              FormatPlainList,
		`{"version":1,"servers":[]}`: FormatSIP008,
		base64.StdEncoding.EncodeToString([]byte("vless://x@a:1")): FormatBase64List,
	}
	for in, want := range cases {
		if got := Detect(in); got != want {
			t.Errorf("Detect(%.20q) = %s, want %s", in, got, want)
		}
	}
}

// TestParseVLESSRealityCanonicalKey guards the session-18 fix: a subscription
// that delivers the reality pbk in standard base64 (with "+"/"/" or "=" padding,
// or — because net/url turns a query "+" into a space — with spaces) must be
// canonicalised to the unpadded base64url form Xray-core requires, so the
// generated config is not rejected as invalid "password".
func TestParseVLESSRealityCanonicalKey(t *testing.T) {
	// 32-byte key whose standard base64 contains "+" and "/".
	const wantRawURL = "z9foAieCPO2_F5ZYNutMqR-VvB15lXVNJsM93g0_M0Q"
	// The panel emits standard base64 (a literal "+"); net/url decodes that "+"
	// to a space when the query is parsed — exactly the corruption the fix
	// recovers from before re-encoding to base64url.
	const stdKey = "z9foAieCPO2/F5ZYNutMqR+VvB15lXVNJsM93g0/M0Q"
	link := "vless://839d4028-2984-4e66-8e62-f4c127b52f49@1.2.3.4:443?security=reality&type=tcp&flow=xtls-rprx-vision&sni=m.com&pbk=" +
		stdKey + "&sid=07ddc43269d197c0#Test"

	s, err := ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink: %v", err)
	}
	if s.PublicKey != wantRawURL {
		t.Errorf("pbk = %q, want canonical %q", s.PublicKey, wantRawURL)
	}
}
