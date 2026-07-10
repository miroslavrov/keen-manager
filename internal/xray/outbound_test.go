package xray

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// canonical is the RawURLEncoding form of a fixed 32-byte X25519 key; every
// other encoding of the SAME key must normalize to this.
const canonicalPBK = "z9foAieCPO2_F5ZYNutMqR-VvB15lXVNJsM93g0_M0Q"

func keyBytes(t *testing.T) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(canonicalPBK)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad canonical key: %v len=%d", err, len(b))
	}
	return b
}

// TestNormalizeRealityKey pins the session-18 fix for "xray config invalid" on
// REALITY servers: modern Xray-core accepts the publicKey ONLY as unpadded
// base64url, so every wire encoding a subscription might deliver (standard
// base64 with +//, padded variants, and the space-for-"+" mangling that
// net/url inflicts on share-link queries) must canonicalise to the RawURL form.
func TestNormalizeRealityKey(t *testing.T) {
	key := keyBytes(t)
	std := base64.RawStdEncoding.EncodeToString(key) // + and /
	cases := map[string]string{
		"already-rawurl":     canonicalPBK,
		"raw-std-plus-slash": std,
		"std-padded":         base64.StdEncoding.EncodeToString(key),
		"url-padded":         base64.URLEncoding.EncodeToString(key),
		"plus-as-space":      strings.ReplaceAll(std, "+", " "), // net/url damage
		"leading-space":      " " + canonicalPBK,
		"trailing-space":     canonicalPBK + "  ",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got := normalizeRealityKey(in)
			if got != canonicalPBK {
				t.Fatalf("normalizeRealityKey(%q) = %q, want %q", in, got, canonicalPBK)
			}
		})
	}
}

func TestNormalizeRealityKeyPassthrough(t *testing.T) {
	// Empty stays empty (OutboundFor turns that into a clear "requires pbk").
	if got := normalizeRealityKey("   "); got != "" {
		t.Errorf("blank key = %q, want empty", got)
	}
	// A value that is not a decodable 32-byte key is left untouched so Xray can
	// report its own precise error rather than us masking a broken credential.
	const notAKey = "definitely-not-a-valid-x25519-public-key"
	if got := normalizeRealityKey(notAKey); got != notAKey {
		t.Errorf("non-key = %q, want unchanged %q", got, notAKey)
	}
}

// TestOutboundRealityNormalizesKey proves the normalization is actually wired
// into config generation (not just the standalone helper) and that shortId is
// trimmed. This is the exact shape that reached Xray as "invalid password".
func TestOutboundRealityNormalizesKey(t *testing.T) {
	key := keyBytes(t)
	s := model.Server{
		Name: "🇸🇪 Sweden", Protocol: model.ProtoVLESS, Address: "sv.example.com", Port: 443,
		UUID: "11111111-1111-1111-1111-111111111111", Flow: "xtls-rprx-vision",
		Security: "reality", Network: "tcp", SNI: "www.microsoft.com",
		// standard-base64 key with "+" mangled to a space, plus a padded shortId
		PublicKey: strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(key), "+", " "),
		ShortID:   " 0123abcd ",
	}
	ob, err := OutboundFor(s, "srv-x")
	if err != nil {
		t.Fatalf("OutboundFor: %v", err)
	}
	rs := ob.StreamSettings.RealitySettings
	if rs == nil {
		t.Fatal("expected reality settings")
	}
	if rs.PublicKey != canonicalPBK {
		t.Errorf("publicKey = %q, want canonical %q", rs.PublicKey, canonicalPBK)
	}
	if rs.ShortID != "0123abcd" {
		t.Errorf("shortId = %q, want trimmed %q", rs.ShortID, "0123abcd")
	}
}
