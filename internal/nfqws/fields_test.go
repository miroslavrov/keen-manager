package nfqws

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// confJSONTags returns the set of json tags declared on the Conf struct.
func confJSONTags() map[string]bool {
	tags := map[string]bool{}
	t := reflect.TypeOf(Conf{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		tags[strings.Split(tag, ",")[0]] = true
	}
	return tags
}

// TestConfFieldsMapToRealKeys guards the CLI field table against a typo'd
// JSONKey: every settable field must name an actual Conf json tag, otherwise
// SaveNfqwsConfigStructured would silently drop the value.
func TestConfFieldsMapToRealKeys(t *testing.T) {
	tags := confJSONTags()
	for name := range confFields {
		key := confFields[name].JSONKey
		if !tags[key] {
			t.Errorf("field %q -> JSONKey %q is not a Conf json tag", name, key)
		}
	}
}

func TestParseConfField(t *testing.T) {
	cases := []struct {
		name, value string
		wantKey     string
		want        any
		wantErr     bool
	}{
		{"tcp-ports", "80,443", "tcp_ports", "80,443", false},
		{"ISP-Interface", "eth3", "isp_interface", "eth3", false}, // case-insensitive name
		{"nfqueue", "300", "nfqueue_num", 300, false},
		{"log-level", "2", "log_level", 2, false},
		{"ipv6", "on", "ipv6_enabled", true, false},
		{"ipv6", "off", "ipv6_enabled", false, false},
		{"args", "--dpi-desync=split2", "nfqws_args", "--dpi-desync=split2", false},
		{"nfqueue", "abc", "", nil, true},   // not an int
		{"nfqueue", "-1", "", nil, true},    // negative
		{"ipv6", "maybe", "", nil, true},    // not a bool
		{"bogus", "x", "", nil, true},       // unknown field
	}
	for _, tc := range cases {
		key, val, err := ParseConfField(tc.name, tc.value)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s=%q: expected error, got key=%q val=%v", tc.name, tc.value, key, val)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s=%q: unexpected error %v", tc.name, tc.value, err)
			continue
		}
		if key != tc.wantKey || val != tc.want {
			t.Errorf("%s=%q: got (%q,%v), want (%q,%v)", tc.name, tc.value, key, val, tc.wantKey, tc.want)
		}
	}
}

// TestParseConfFieldMergeRoundTrip proves a parsed field actually lands on the
// right typed struct member through the same JSON overlay
// engine.SaveNfqwsConfigStructured performs (marshal current -> map -> overlay
// -> unmarshal into Conf).
func TestParseConfFieldMergeRoundTrip(t *testing.T) {
	overlay := func(base Conf, name, value string) Conf {
		key, val, err := ParseConfField(name, value)
		if err != nil {
			t.Fatalf("ParseConfField(%q,%q): %v", name, value, err)
		}
		b, _ := json.Marshal(base)
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		m[key] = val
		mb, _ := json.Marshal(m)
		var out Conf
		if err := json.Unmarshal(mb, &out); err != nil {
			t.Fatalf("unmarshal merged: %v", err)
		}
		return out
	}

	c := overlay(Conf{}, "tcp-ports", "80,443")
	if c.TCPPorts != "80,443" {
		t.Errorf("tcp-ports did not land: %q", c.TCPPorts)
	}
	c = overlay(c, "nfqueue", "301")
	if c.NfqueueNum != 301 {
		t.Errorf("nfqueue did not land: %d", c.NfqueueNum)
	}
	c = overlay(c, "ipv6", "on")
	if !c.IPv6Enabled {
		t.Error("ipv6 did not land")
	}
	// Earlier overlays must survive later ones (independent keys).
	if c.TCPPorts != "80,443" || c.NfqueueNum != 301 {
		t.Errorf("prior fields clobbered: %+v", c)
	}
}
