package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// TestMigrateEnablesLegacySubscriptions guards the schema-v2 upgrade: a state
// written before Subscription.Enabled existed has no such field, so it
// unmarshals to false — but those subscriptions were implicitly active and must
// come back enabled, or the upgrade would silently kill the user's fleet.
func TestMigrateEnablesLegacySubscriptions(t *testing.T) {
	// A v1 document as it would exist on disk before this change: a subscription
	// with no "enabled" key at all.
	legacy := `{
	  "schema_version": 1,
	  "subscriptions": [
	    {"id": "sub-1", "name": "OceanLink", "url": "https://x/y", "auto_select_best": true},
	    {"id": "sub-2", "name": "SkyRoute", "url": "https://a/b", "auto_select_best": false}
	  ]
	}`
	var st model.State
	if err := json.Unmarshal([]byte(legacy), &st); err != nil {
		t.Fatal(err)
	}
	// Sanity: before migration the field is false (no key in the JSON).
	if st.Subscriptions[0].Enabled || st.Subscriptions[1].Enabled {
		t.Fatal("precondition: legacy subs should unmarshal to Enabled=false")
	}

	migrate(&st)

	if st.Version != SchemaVersion {
		t.Errorf("Version = %d after migrate, want %d", st.Version, SchemaVersion)
	}
	for _, s := range st.Subscriptions {
		if !s.Enabled {
			t.Errorf("subscription %q should be enabled after migration from v1", s.Name)
		}
	}
}

// TestMigrateFromZeroVersion covers a state with no schema_version at all (the
// oldest installs): treated as pre-v2, so its subscriptions are enabled too.
func TestMigrateFromZeroVersion(t *testing.T) {
	var st model.State
	st.Subscriptions = []model.Subscription{{ID: "s", Name: "n"}}
	migrate(&st)
	if st.Version != SchemaVersion {
		t.Errorf("Version = %d, want %d", st.Version, SchemaVersion)
	}
	if !st.Subscriptions[0].Enabled {
		t.Error("a version-0 subscription should be enabled after migration")
	}
}

// TestOpenMigratesOnDisk exercises the real Open() path end-to-end: writing a
// legacy state file and re-opening it must yield enabled subscriptions.
func TestOpenMigratesOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	legacy := `{"schema_version":1,"subscriptions":[{"id":"s1","name":"Old","url":"https://h/t"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	subs := store.Get().Subscriptions
	if len(subs) != 1 || !subs[0].Enabled {
		t.Errorf("expected the on-disk legacy subscription to be enabled after Open, got %+v", subs)
	}
}

// TestDefaultStateIsCurrentSchema ensures a fresh install is stamped with the
// current schema version (so migrate's version-gated steps never re-run on it).
func TestDefaultStateIsCurrentSchema(t *testing.T) {
	if v := DefaultState().Version; v != SchemaVersion {
		t.Errorf("DefaultState().Version = %d, want %d", v, SchemaVersion)
	}
}
