package keenetic

import "testing"

func TestSanitizeGroupName(t *testing.T) {
	cases := map[string]string{
		"youtube":       "km-youtube",
		"YouTube":       "km-youtube",
		"my route!!":    "km-my-route",
		"km-already":    "km-already",
		"  spaced  ":    "km-spaced",
		"":              "km-route",
		"...":           "km-route",
		"a/b\\c:d":      "km-a-b-c-d",
	}
	for in, want := range cases {
		if got := SanitizeGroupName(in); got != want {
			t.Errorf("SanitizeGroupName(%q) = %q, want %q", in, got, want)
		}
	}
	// Always prefixed and length-bounded.
	long := SanitizeGroupName("this-is-a-really-long-slug-that-exceeds-the-ndms-limit-by-a-lot")
	if len(long) > 40 {
		t.Errorf("expected name <= 40 chars, got %d (%q)", len(long), long)
	}
	if long[:3] != "km-" {
		t.Errorf("expected km- prefix, got %q", long)
	}
}

func TestChunkDomains(t *testing.T) {
	// Empty in, empty out.
	if got := ChunkDomains("x", nil); len(got) != 0 {
		t.Errorf("expected no chunks for empty input, got %d", len(got))
	}

	// 301 domains -> 2 chunks: base (300) + base-1 (1).
	domains := make([]string, 301)
	for i := range domains {
		domains[i] = "d" + itoa(i) + ".example.com"
	}
	chunks := ChunkDomains("big", domains)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks["km-big"]) != MaxDomainsPerGroup {
		t.Errorf("first chunk should be full (%d), got %d", MaxDomainsPerGroup, len(chunks["km-big"]))
	}
	if len(chunks["km-big-1"]) != 1 {
		t.Errorf("second chunk should hold the remainder (1), got %d", len(chunks["km-big-1"]))
	}

	total := 0
	for _, v := range chunks {
		total += len(v)
	}
	if total != 301 {
		t.Errorf("chunking lost domains: total %d, want 301", total)
	}
}

func TestOwnedDNSRoutes(t *testing.T) {
	in := []dnsRouteEntry{
		{Group: "km-youtube", Interface: "Wireguard0"},
		{Group: "user-group", Interface: "Wireguard0"},
		{Group: "km-netflix", Interface: "Wireguard1"},
	}
	owned := OwnedDNSRoutes(in)
	if len(owned) != 2 {
		t.Fatalf("expected 2 owned routes, got %d", len(owned))
	}
	for _, r := range owned {
		if r.Group[:3] != "km-" {
			t.Errorf("unexpected non-owned group %q", r.Group)
		}
	}
}

// itoa is a tiny local int->string to avoid importing strconv in a test file
// that otherwise needs nothing.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
