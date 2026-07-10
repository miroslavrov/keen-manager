package engine

import (
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// addSubWithConn adds an enabled subscription plus one member Xray connection.
func addSubWithConn(t *testing.T, e *Engine, subID, connID string) {
	t.Helper()
	addXrayConn(t, e, connID, "node-"+connID)
	if err := e.store.Mutate(func(s *model.State) error {
		for i := range s.Connections {
			if s.Connections[i].ID == connID {
				s.Connections[i].SubscriptionID = subID
			}
		}
		s.Subscriptions = append(s.Subscriptions, model.Subscription{
			ID: subID, Name: "sub-" + subID, URL: "https://h/" + subID,
			Enabled: true, AutoSelectBest: true, ServerIDs: []string{connID},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestConnEligibleRespectsSubscription is the core predicate: a member is
// eligible only when both it AND its subscription stream are enabled. A
// connection with no subscription is governed solely by its own flag.
func TestConnEligibleRespectsSubscription(t *testing.T) {
	e := newTestEngine(t)
	addSubWithConn(t, e, "sub1", "c1")
	addXrayConn(t, e, "standalone", "Manual") // no subscription

	get := func(id string) model.Connection {
		c, ok := findConn(e.store.Get(), id)
		if !ok {
			t.Fatalf("conn %s missing", id)
		}
		return c
	}

	if !connEligible(e.store.Get(), get("c1")) {
		t.Error("member of an enabled subscription should be eligible")
	}

	// Disable the subscription stream.
	if _, err := e.UpdateSubscription("sub1", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("disable sub: %v", err)
	}
	if connEligible(e.store.Get(), get("c1")) {
		t.Error("member of a DISABLED subscription must be ineligible even though its own Enabled is true")
	}
	if !connEligible(e.store.Get(), get("standalone")) {
		t.Error("a subscription-less connection must be unaffected by a disabled subscription")
	}

	// Its list view should read disabled while its own switch stays on.
	var cv ConnView
	for _, v := range e.Connections() {
		if v.ID == "c1" {
			cv = v
		}
	}
	if cv.Status != string(model.StatusDisabled) {
		t.Errorf("c1 status = %q, want disabled (subscription off)", cv.Status)
	}
	if !cv.Enabled {
		t.Error("c1 per-connection Enabled should remain true (only the subscription is off)")
	}

	// Re-enabling restores eligibility.
	if _, err := e.UpdateSubscription("sub1", map[string]any{"enabled": true}); err != nil {
		t.Fatalf("re-enable sub: %v", err)
	}
	if !connEligible(e.store.Get(), get("c1")) {
		t.Error("member should be eligible again after re-enabling the subscription")
	}
}

// TestDisableSubscriptionTearsDownActive verifies that turning a stream off while
// one of its servers is active clears the active connection (LAN → direct).
func TestDisableSubscriptionTearsDownActive(t *testing.T) {
	e := newTestEngine(t)
	addSubWithConn(t, e, "sub1", "c1")
	if err := e.store.Mutate(func(s *model.State) error { s.ActiveConnID = "c1"; return nil }); err != nil {
		t.Fatal(err)
	}

	if _, err := e.UpdateSubscription("sub1", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if got := e.store.Get().ActiveConnID; got != "" {
		t.Errorf("ActiveConnID = %q, want empty after disabling the active server's subscription", got)
	}
}

// TestSelectBestRejectsDisabledSubscription: select-best on a disabled stream is
// a clear error, not a silent no-op.
func TestSelectBestRejectsDisabledSubscription(t *testing.T) {
	e := newTestEngine(t)
	addSubWithConn(t, e, "sub1", "c1")
	if _, err := e.UpdateSubscription("sub1", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	_, err := e.SelectBest("sub1")
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("SelectBest on a disabled subscription should error mentioning 'disabled', got %v", err)
	}
}

// TestActivateRejectsDisabledSubscriptionMember: manual activation of a server
// under a disabled stream is refused with an actionable message.
func TestActivateRejectsDisabledSubscriptionMember(t *testing.T) {
	e := newTestEngine(t)
	addSubWithConn(t, e, "sub1", "c1")
	// Disable the sub directly (avoid UpdateSubscription's active-teardown path).
	if err := e.store.Mutate(func(s *model.State) error {
		for i := range s.Subscriptions {
			if s.Subscriptions[i].ID == "sub1" {
				s.Subscriptions[i].Enabled = false
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := e.Activate("c1")
	if err == nil || !strings.Contains(err.Error(), "disabled subscription") {
		t.Errorf("Activate of a disabled-subscription member should be refused, got %v", err)
	}
}

// TestSubViewCarriesEnabled: the API view surfaces the stream flag both ways.
func TestSubViewCarriesEnabled(t *testing.T) {
	e := newTestEngine(t)
	addSubWithConn(t, e, "sub1", "c1")
	find := func() SubView {
		for _, v := range e.Subscriptions() {
			if v.ID == "sub1" {
				return v
			}
		}
		t.Fatal("sub1 not in view")
		return SubView{}
	}
	if !find().Enabled {
		t.Error("SubView.Enabled should be true for a fresh subscription")
	}
	if _, err := e.UpdateSubscription("sub1", map[string]any{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	if find().Enabled {
		t.Error("SubView.Enabled should be false after disabling")
	}
}
