package subscription

import "testing"

func TestTitleFromHeader(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "My Servers", "My Servers"},
		{"trimmed", "  Spaced  ", "Spaced"},
		{"base64-blancvpn", "base64:QmxhbmNWUE4=", "BlancVPN"},
		{"base64-cyrillic", "base64:0J/RgNC+0LrRgdC4", "Прокси"},
		{"base64-raw-nopad", "base64:QmxhbmNWUE4", "BlancVPN"},
		{"base64-garbage", "base64:!!!not-base64!!!", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := titleFromHeader(c.in); got != c.want {
				t.Fatalf("titleFromHeader(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
