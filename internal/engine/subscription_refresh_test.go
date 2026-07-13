package engine

import (
	"testing"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// These tests lock the two guarantees the user asked us to "проконтролировать":
//
//  1. auto-select-best / select-best only ever consider ENABLED servers, so a
//     server the user switched off (e.g. a home-country node) is never picked —
//     covered by TestSubMembersExcludesDisabled.
//  2. a subscription refresh must NOT reset the per-server on/off choice: an
//     unchanged server keeps its Enabled flag (a disabled one is never silently
//     re-enabled), while a changed endpoint is treated as new (reset is
//     acceptable) — covered by the reconcile tests below, which exercise the pure
//     core of RefreshSubscription with no network.

func member(id, subID, addr string, port int, enabled bool) model.Connection {
	return model.Connection{
		ID: id, Type: model.ConnXray, Name: id, Enabled: enabled, SubscriptionID: subID,
		Xray: &model.Server{ID: id, Address: addr, Port: port, Protocol: model.ProtoVLESS},
	}
}

func fetched(addr string, port int) model.Server {
	return model.Server{Address: addr, Port: port, Protocol: model.ProtoVLESS, UUID: "uuid-" + addr}
}

func connByID(st model.State, id string) (model.Connection, bool) { return findConn(st, id) }

func connByAddr(st model.State, subID, addr string) (model.Connection, bool) {
	for _, c := range st.Connections {
		if c.SubscriptionID == subID && c.Xray != nil && c.Xray.Address == addr {
			return c, true
		}
	}
	return model.Connection{}, false
}

// TestReconcilePreservesDisabledServer: when the provider returns the SAME
// servers, every member keeps its id and — crucially — its Enabled flag, so a
// server the user turned off stays off. Nothing is removed or re-enabled.
func TestReconcilePreservesDisabledServer(t *testing.T) {
	st := model.State{
		Subscriptions: []model.Subscription{{ID: "s1", Enabled: true, AutoSelectBest: true}},
		Connections: []model.Connection{
			member("A", "s1", "a.example", 443, true),
			member("B", "s1", "b.example", 443, true),
			member("C", "s1", "c.example", 443, false), // user switched this off
		},
		ActiveConnID: "A",
	}
	in := []model.Server{fetched("a.example", 443), fetched("b.example", 443), fetched("c.example", 443)}

	_, removed, ids := reconcileSubscriptionMembers(&st, "s1", in, time.Now())

	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none (all servers still present)", removed)
	}
	if len(st.Connections) != 3 {
		t.Fatalf("connections = %d, want 3", len(st.Connections))
	}
	if len(ids) != 3 {
		t.Fatalf("serverIDs = %d, want 3", len(ids))
	}
	c, ok := connByID(st, "C")
	if !ok {
		t.Fatal("disabled server C vanished after refresh")
	}
	if c.Enabled {
		t.Error("server C was re-ENABLED by a refresh that changed nothing — must stay off")
	}
	if a, _ := connByID(st, "A"); !a.Enabled {
		t.Error("server A should remain enabled")
	}
	if b, _ := connByID(st, "B"); !b.Enabled {
		t.Error("server B should remain enabled")
	}
}

// TestReconcileAddsNewServerEnabled: a genuinely new server (new endpoint) joins
// as an enabled connection, while the existing disabled one stays disabled.
func TestReconcileAddsNewServerEnabled(t *testing.T) {
	st := model.State{
		Subscriptions: []model.Subscription{{ID: "s1", Enabled: true}},
		Connections: []model.Connection{
			member("A", "s1", "a.example", 443, true),
			member("C", "s1", "c.example", 443, false),
		},
	}
	in := []model.Server{
		fetched("a.example", 443),
		fetched("c.example", 443),
		fetched("d.example", 443), // brand new
	}

	_, removed, ids := reconcileSubscriptionMembers(&st, "s1", in, time.Now())

	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", removed)
	}
	if len(ids) != 3 || len(st.Connections) != 3 {
		t.Fatalf("want 3 members, got ids=%d conns=%d", len(ids), len(st.Connections))
	}
	d, ok := connByAddr(st, "s1", "d.example")
	if !ok {
		t.Fatal("new server d.example was not added")
	}
	if !d.Enabled {
		t.Error("a brand-new server should default to enabled")
	}
	if c, _ := connByID(st, "C"); c.Enabled {
		t.Error("existing disabled server C must stay disabled when a new one is added")
	}
}

// TestReconcileChangedEndpointResets: a server whose endpoint changed is a new
// connection (enabled), and the stale one is removed — the user accepted that a
// changed server may lose its off state ("пусть слетает").
func TestReconcileChangedEndpointResets(t *testing.T) {
	st := model.State{
		Subscriptions: []model.Subscription{{ID: "s1", Enabled: true}},
		Connections: []model.Connection{
			member("A", "s1", "a.example", 443, true),
			member("C", "s1", "c.example", 443, false), // disabled, endpoint about to rotate
		},
		ActiveConnID: "A",
	}
	in := []model.Server{
		fetched("a.example", 443),
		fetched("c2.example", 443), // C's endpoint rotated
	}

	_, removed, _ := reconcileSubscriptionMembers(&st, "s1", in, time.Now())

	if len(removed) != 1 || removed[0] != "C" {
		t.Fatalf("removed = %v, want [C] (stale endpoint dropped)", removed)
	}
	if _, ok := connByID(st, "C"); ok {
		t.Error("stale server C should be gone after its endpoint changed")
	}
	nc, ok := connByAddr(st, "s1", "c2.example")
	if !ok {
		t.Fatal("rotated server c2.example was not added")
	}
	if !nc.Enabled {
		t.Error("a changed-endpoint server is treated as new and defaults to enabled")
	}
}

// TestReconcileKeepsStaleActive: if the active server vanishes from the provider
// it is kept (stale) so a refresh never tears down the live tunnel.
func TestReconcileKeepsStaleActive(t *testing.T) {
	st := model.State{
		Subscriptions: []model.Subscription{{ID: "s1", Enabled: true}},
		Connections: []model.Connection{
			member("A", "s1", "a.example", 443, true),
			member("C", "s1", "c.example", 443, true),
		},
		ActiveConnID: "C",
	}
	in := []model.Server{fetched("a.example", 443)} // C disappeared

	_, removed, ids := reconcileSubscriptionMembers(&st, "s1", in, time.Now())

	for _, r := range removed {
		if r == "C" {
			t.Fatal("active server C must not be removed even though it vanished upstream")
		}
	}
	if _, ok := connByID(st, "C"); !ok {
		t.Error("active-but-vanished server C should be kept")
	}
	foundC := false
	for _, id := range ids {
		if id == "C" {
			foundC = true
		}
	}
	if !foundC {
		t.Error("stale-but-active C should remain in the subscription's ServerIDs")
	}
}

// TestSubMembersExcludesDisabled: the shared pool builder returns only enabled
// members of an enabled subscription — the guarantee behind "auto-best never
// picks a server you switched off".
func TestSubMembersExcludesDisabled(t *testing.T) {
	st := model.State{
		Subscriptions: []model.Subscription{{ID: "s1", Enabled: true}},
		Connections: []model.Connection{
			member("A", "s1", "a.example", 443, true),
			member("C", "s1", "c.example", 443, false), // switched off
			{ID: "X", Type: model.ConnXray, Enabled: true}, // not a member of s1
		},
	}

	m := subMembers(st, "s1")
	if len(m) != 1 || m[0].ID != "A" {
		t.Fatalf("subMembers = %v, want just [A] (C disabled, X not a member)", ids(m))
	}

	// Turning the whole subscription stream off drops every member from the pool,
	// even ones whose own switch is still on.
	st.Subscriptions[0].Enabled = false
	if m := subMembers(st, "s1"); len(m) != 0 {
		t.Errorf("subMembers on a disabled stream = %v, want empty", ids(m))
	}
}

func ids(cs []model.Connection) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
