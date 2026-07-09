package presets

import "testing"

func TestCatalogLoads(t *testing.T) {
	if err := Err(); err != nil {
		t.Fatalf("catalog failed to load: %v", err)
	}
	cat := Catalog()
	if len(cat) < 50 {
		t.Fatalf("expected a substantial catalog, got %d presets", len(cat))
	}
	for _, p := range cat {
		if p.ID == "" || p.Name == "" || p.Category == "" {
			t.Errorf("preset missing required fields: %+v", p)
		}
		if len(p.Domains) == 0 && len(p.Subnets) == 0 && p.SubscriptionURL == "" {
			t.Errorf("preset %q routes nothing (no domains, subnets or subscription)", p.ID)
		}
	}
}

func TestByID(t *testing.T) {
	// YouTube is a stable, always-present preset.
	yt, ok := ByID("youtube")
	if !ok {
		t.Fatal("expected a youtube preset")
	}
	if len(yt.Domains) == 0 {
		t.Fatal("youtube preset has no domains")
	}
	var hasYouTube bool
	for _, d := range yt.Domains {
		if d == "youtube.com" {
			hasYouTube = true
		}
	}
	if !hasYouTube {
		t.Errorf("youtube preset missing youtube.com: %v", yt.Domains)
	}
	if _, ok := ByID("does-not-exist"); ok {
		t.Error("ByID returned ok for a missing preset")
	}
}

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("no categories")
	}
	// social should sort first per the display order.
	if cats[0] != "social" {
		t.Errorf("expected social first, got %q (%v)", cats[0], cats)
	}
}
