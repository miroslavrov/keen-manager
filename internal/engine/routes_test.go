package engine

import (
	"strings"
	"testing"
)

// TestUpdateRouteAndDetail confirms editing a route replaces its domain/subnet
// membership (deduped, lower-cased), preserves the target on a partial edit,
// rejects an empty membership, and that RouteDetail returns the full domain
// list the editor needs.
func TestUpdateRouteAndDetail(t *testing.T) {
	e := newTestEngine(t)
	addXrayConn(t, e, "conn-x", "Amsterdam")

	rv, err := e.CreateRoute("Media", "", []string{"youtube.com"}, nil, "conn-x", "")
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if rv.DomainCount != 1 {
		t.Fatalf("created domain count = %d, want 1", rv.DomainCount)
	}

	// Edit: rename, expand the domain list (with a dupe + mixed case), add a
	// subnet, and leave the target blank — a partial edit must keep the target.
	got, err := e.UpdateRoute(rv.ID, "Media+", []string{"youtube.com", "YouTube.com", "googlevideo.com"}, []string{"203.0.113.0/24"}, "", "")
	if err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}
	if got.Name != "Media+" {
		t.Errorf("name = %q, want Media+", got.Name)
	}
	if got.DomainCount != 2 {
		t.Errorf("domain count = %d, want 2 (deduped, case-insensitive)", got.DomainCount)
	}
	if got.SubnetCount != 1 {
		t.Errorf("subnet count = %d, want 1", got.SubnetCount)
	}
	if got.TargetConnID != "conn-x" {
		t.Errorf("target lost on partial edit: %q", got.TargetConnID)
	}

	// Detail carries the full, lower-cased membership for the editor.
	d, ok := e.RouteDetail(rv.ID)
	if !ok {
		t.Fatal("RouteDetail not found")
	}
	if len(d.Domains) != 2 {
		t.Errorf("detail domains = %v, want 2", d.Domains)
	}
	for _, dom := range d.Domains {
		if dom != strings.ToLower(dom) {
			t.Errorf("domain not lower-cased: %q", dom)
		}
	}
	if len(d.Subnets) != 1 || d.Subnets[0] != "203.0.113.0/24" {
		t.Errorf("detail subnets = %v, want [203.0.113.0/24]", d.Subnets)
	}

	// Clearing all domains AND subnets is rejected (a route must route something).
	if _, err := e.UpdateRoute(rv.ID, "", nil, nil, "", ""); err == nil {
		t.Error("expected an error when clearing all domains and subnets")
	}

	// Unknown id errors.
	if _, err := e.UpdateRoute("nope", "x", []string{"a.com"}, nil, "", ""); err == nil {
		t.Error("expected an error for an unknown route id")
	}
}
