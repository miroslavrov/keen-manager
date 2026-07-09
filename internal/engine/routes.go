package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/presets"
)

// This file implements the Routes / "Маршруты" feature: sending a chosen set of
// service domains (and/or subnets) through a specific connection's native
// interface, using Keenetic's own domain-routing stack (object-group fqdn +
// dns-proxy route). It mirrors the per-service routing of awg-manager, built on
// the built-in preset catalog in internal/presets.
//
// SAFETY: every device mutation is dry-run aware (inert off-device), only ever
// touches object-groups keen-manager owns (the "km-" prefix), and never fails a
// route save because of a transient RCI error — a route can always be retried.

// RoutePresets returns the built-in service catalog for the UI.
func (e *Engine) RoutePresets() PresetCatalogView {
	cat := presets.Catalog()
	items := make([]PresetView, 0, len(cat))
	for _, p := range cat {
		items = append(items, PresetView{
			ID:          p.ID,
			Name:        p.Name,
			Category:    p.Category,
			Icon:        p.Icon,
			Notice:      p.Notice,
			DomainCount: p.DomainCount(),
			SubnetCount: p.SubnetCount(),
			HasSub:      p.SubscriptionURL != "",
		})
	}
	return PresetCatalogView{Categories: presets.Categories(), Presets: items}
}

// Routes returns the configured service routes.
func (e *Engine) Routes() []RouteView {
	st := e.store.Get()
	out := make([]RouteView, 0, len(st.Routes))
	for _, r := range st.Routes {
		out = append(out, e.routeView(st, r))
	}
	return out
}

func (e *Engine) routeView(st model.State, r model.ServiceRoute) RouteView {
	v := RouteView{
		ID:           r.ID,
		Name:         r.Name,
		PresetID:     r.PresetID,
		Category:     r.Category,
		Icon:         r.Icon,
		DomainCount:  len(r.Domains),
		SubnetCount:  len(r.Subnets),
		TargetConnID: r.TargetConnID,
		Enabled:      r.Enabled,
		Applied:      r.Applied,
	}
	if c, ok := findConn(st, r.TargetConnID); ok {
		v.TargetName = c.Name
	}
	// Surface why a route isn't live so the UI can guide the user.
	if iface, ok := e.resolveRouteIface(r); ok {
		v.TargetIface = iface
		if v.TargetName == "" {
			v.TargetName = iface
		}
	} else if r.Enabled {
		v.Note = "target has no native interface yet — activate its AmneziaWG connection"
	}
	if r.Enabled && !e.dnsRoutingAvailable() {
		v.Note = "firmware has no native DNS routing (needs KeeneticOS 5.x)"
	}
	return v
}

// resolveRouteIface returns the KeeneticOS interface name a route binds to.
// A directly-pinned TargetIface wins; otherwise it resolves the target
// connection's native interface. The bool is false when neither yields an
// interface (e.g. a connection-targeted route whose AWG tunnel isn't up yet).
func (e *Engine) resolveRouteIface(r model.ServiceRoute) (string, bool) {
	if name := strings.TrimSpace(r.TargetIface); name != "" {
		return name, true
	}
	return e.nativeIface(r.TargetConnID)
}

// dnsRoutingAvailable reports whether the device can apply native DNS routes.
func (e *Engine) dnsRoutingAvailable() bool {
	return e.keenetic != nil && !e.runner.DryRun && e.caps.SupportsDNSRoute
}

// CreateRoute builds a route from a preset (presetID) and/or explicit custom
// domains/subnets, targeting either a keen-manager connection (targetConnID)
// or a router interface directly (targetIface, e.g. "Wireguard0" picked from
// the live interface list), then applies it when enabled.
func (e *Engine) CreateRoute(name, presetID string, domains, subnets []string, targetConnID, targetIface string) (RouteView, error) {
	st := e.store.Get()
	targetConnID = strings.TrimSpace(targetConnID)
	targetIface = strings.TrimSpace(targetIface)
	if targetConnID == "" && targetIface == "" {
		return RouteView{}, fmt.Errorf("a route needs a target interface or connection")
	}
	if targetConnID != "" {
		if _, ok := findConn(st, targetConnID); !ok {
			return RouteView{}, fmt.Errorf("target connection %s not found", targetConnID)
		}
	}

	r := model.ServiceRoute{
		ID:           newID("route"),
		TargetConnID: targetConnID,
		TargetIface:  targetIface,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}

	// Snapshot the preset lists so the route is self-contained.
	if presetID != "" {
		p, ok := presets.ByID(presetID)
		if !ok {
			return RouteView{}, fmt.Errorf("unknown preset %q", presetID)
		}
		r.PresetID = p.ID
		r.Category = p.Category
		r.Icon = p.Icon
		r.Name = firstNonEmpty(strings.TrimSpace(name), p.Name)
		r.Domains = dedupeLower(append(append([]string{}, p.Domains...), domains...))
		r.Subnets = dedupe(append(append([]string{}, p.Subnets...), subnets...))
	} else {
		r.Name = firstNonEmpty(strings.TrimSpace(name), "Custom route")
		r.Domains = dedupeLower(domains)
		r.Subnets = dedupe(subnets)
		r.Category = "custom"
	}
	if len(r.Domains) == 0 && len(r.Subnets) == 0 {
		return RouteView{}, fmt.Errorf("route %q has no domains or subnets to route", r.Name)
	}

	if err := e.store.Mutate(func(s *model.State) error {
		s.Routes = append(s.Routes, r)
		return nil
	}); err != nil {
		return RouteView{}, err
	}
	e.Logf("route created: %s (%d domains, %d subnets) -> %s", r.Name, len(r.Domains), len(r.Subnets), targetConnID)

	// Best-effort apply; a failure here is surfaced in the view Note, not fatal.
	if err := e.applyRoute(r.ID); err != nil {
		e.Logf("route %s not yet live: %v", r.Name, err)
	}
	e.publishState()
	st = e.store.Get()
	rr, _ := findRoute(st, r.ID)
	return e.routeView(st, rr), nil
}

// SetRouteEnabled toggles a route on/off, applying or removing it on the router.
func (e *Engine) SetRouteEnabled(id string, on bool) error {
	r, ok := findRoute(e.store.Get(), id)
	if !ok {
		return fmt.Errorf("route %s not found", id)
	}
	if err := e.store.Mutate(func(s *model.State) error {
		for i := range s.Routes {
			if s.Routes[i].ID == id {
				s.Routes[i].Enabled = on
			}
		}
		return nil
	}); err != nil {
		return err
	}
	var err error
	if on {
		err = e.applyRoute(id)
	} else {
		err = e.unapplyRoute(r)
	}
	e.publishState()
	return err
}

// DeleteRoute removes a route, tearing it down on the router first.
func (e *Engine) DeleteRoute(id string) error {
	r, ok := findRoute(e.store.Get(), id)
	if !ok {
		return fmt.Errorf("route %s not found", id)
	}
	_ = e.unapplyRoute(r)
	if err := e.store.Mutate(func(s *model.State) error {
		out := s.Routes[:0]
		for _, x := range s.Routes {
			if x.ID != id {
				out = append(out, x)
			}
		}
		s.Routes = out
		return nil
	}); err != nil {
		return err
	}
	e.Logf("route deleted: %s", r.Name)
	e.publishState()
	return nil
}

// applyRoute pushes a route to the router: one object-group per <=300-domain
// chunk bound to the target's native interface via dns-proxy route, plus a
// static route per subnet. It records the created group names for teardown.
func (e *Engine) applyRoute(id string) error {
	r, ok := findRoute(e.store.Get(), id)
	if !ok {
		return fmt.Errorf("route %s not found", id)
	}
	if !r.Enabled {
		return nil
	}
	if e.runner.DryRun {
		// Off-device: record as applied so the UI reflects intent.
		e.markRouteApplied(id, nil, true)
		return nil
	}
	if !e.dnsRoutingAvailable() {
		return fmt.Errorf("native DNS routing unavailable (needs KeeneticOS 5.x)")
	}
	iface, ok := e.resolveRouteIface(r)
	if !ok || iface == "" {
		return fmt.Errorf("route target has no native interface (activate its AmneziaWG connection first, or pick a router interface)")
	}

	ctx, cancel := context.WithTimeout(e.baseCtx(), 45*time.Second)
	defer cancel()

	var created []string
	if len(r.Domains) > 0 {
		chunks := keenetic.ChunkDomains(routeSlug(r), r.Domains)
		for group, domains := range chunks {
			if err := keenetic.SetObjectGroupFQDN(ctx, e.keenetic, group, domains); err != nil {
				e.rollbackRoute(ctx, created, iface)
				return err
			}
			if err := keenetic.AddDNSRoute(ctx, e.keenetic, group, iface, true); err != nil {
				e.rollbackRoute(ctx, append(created, group), iface)
				return err
			}
			created = append(created, group)
		}
	}
	for _, cidr := range r.Subnets {
		if err := keenetic.AddStaticRoute(ctx, e.keenetic, cidr, iface); err != nil {
			// Subnet routes are additive best-effort; log and continue so one bad
			// CIDR doesn't sink the whole route.
			e.Logf("route %s: static route %s failed: %v", r.Name, cidr, err)
		}
	}
	if err := e.keenetic.Save(ctx); err != nil {
		e.Logf("route %s: RCI save warning: %v", r.Name, err)
	}
	e.markRouteApplied(id, created, true)
	e.Logf("route applied: %s -> %s (%d groups)", r.Name, iface, len(created))
	return nil
}

// unapplyRoute removes a route's object-groups + dns-proxy routes + subnet
// routes from the router (best-effort, resilient to a downed target).
func (e *Engine) unapplyRoute(r model.ServiceRoute) error {
	if e.runner.DryRun || e.keenetic == nil {
		e.markRouteApplied(r.ID, nil, false)
		return nil
	}
	ctx, cancel := context.WithTimeout(e.baseCtx(), 45*time.Second)
	defer cancel()
	iface, _ := e.resolveRouteIface(r)
	for _, group := range r.Groups {
		if iface != "" {
			_ = keenetic.DeleteDNSRoute(ctx, e.keenetic, group, iface)
		}
		_ = keenetic.DeleteObjectGroup(ctx, e.keenetic, group)
	}
	if iface != "" {
		for _, cidr := range r.Subnets {
			_ = keenetic.DeleteStaticRoute(ctx, e.keenetic, cidr, iface)
		}
	}
	_ = e.keenetic.Save(ctx)
	e.markRouteApplied(r.ID, nil, false)
	e.Logf("route removed: %s", r.Name)
	return nil
}

// rollbackRoute tears down the object-groups created so far when an apply fails
// partway, so a failed apply never leaves half a route on the router.
func (e *Engine) rollbackRoute(ctx context.Context, groups []string, iface string) {
	for _, group := range groups {
		if iface != "" {
			_ = keenetic.DeleteDNSRoute(ctx, e.keenetic, group, iface)
		}
		_ = keenetic.DeleteObjectGroup(ctx, e.keenetic, group)
	}
}

// reconcileRoutes re-applies enabled-but-unapplied routes whose target now has
// a native interface (e.g. after the target AWG connection was activated, or
// after a daemon restart). Runs off the hot path; every apply is best-effort.
func (e *Engine) reconcileRoutes() {
	if e.runner.DryRun || !e.dnsRoutingAvailable() {
		return
	}
	for _, r := range e.store.Get().Routes {
		if !r.Enabled || r.Applied {
			continue
		}
		if iface, ok := e.resolveRouteIface(r); ok && iface != "" {
			if err := e.applyRoute(r.ID); err != nil {
				e.Logf("reconcile route %s: %v", r.Name, err)
			}
		}
	}
}

func (e *Engine) markRouteApplied(id string, groups []string, applied bool) {
	_ = e.store.Mutate(func(s *model.State) error {
		for i := range s.Routes {
			if s.Routes[i].ID != id {
				continue
			}
			s.Routes[i].Applied = applied
			if applied {
				if groups != nil {
					s.Routes[i].Groups = groups
				}
			} else {
				s.Routes[i].Groups = nil
			}
		}
		return nil
	})
}

// ----- helpers -----

func findRoute(st model.State, id string) (model.ServiceRoute, bool) {
	for _, r := range st.Routes {
		if r.ID == id {
			return r, true
		}
	}
	return model.ServiceRoute{}, false
}

// routeSlug derives the object-group base name for a route (preset id when
// available, else the route id), sanitized by keenetic.SanitizeGroupName.
func routeSlug(r model.ServiceRoute) string {
	if r.PresetID != "" {
		return r.PresetID
	}
	return r.ID
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func dedupeLower(in []string) []string {
	for i := range in {
		in[i] = strings.ToLower(strings.TrimSpace(in[i]))
	}
	return dedupe(in)
}
