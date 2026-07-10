// Package config persists the keen-manager State document with atomic writes and
// automatic backups.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// SchemaVersion is bumped when the persisted format changes.
//   v2: model.Subscription.Enabled introduced (default true for existing subs).
const SchemaVersion = 2

// Store is a thread-safe wrapper around the persisted State.
type Store struct {
	mu        sync.RWMutex
	path      string
	backupDir string
	state     model.State
}

// Open loads the state from path, creating defaults if it does not exist.
func Open(path, backupDir string) (*Store, error) {
	s := &Store{path: path, backupDir: backupDir}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		s.state = DefaultState()
		if err := s.save(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	migrate(&s.state)
	return s, nil
}

// Get returns a deep-ish copy of the current state (safe to read).
func (s *Store) Get() model.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Settings returns the current settings.
func (s *Store) Settings() model.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Settings
}

// SetAuthHashInMemory reinstates the web UI password hash on the in-memory
// settings WITHOUT persisting state.json. The hash is json:"-" in the model
// (so it never lands in state.json or its backups) and is instead sourced from
// the 0600 vault at startup; this setter lets the engine load it back into the
// live settings after a restart without triggering a state write. It is a
// no-op for the persisted document, so it does not churn backups on every boot.
func (s *Store) SetAuthHashInMemory(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Settings.PasswordHash = hash
}

// Mutate applies fn under lock and persists the result. If fn returns an error,
// the state is not saved.
func (s *Store) Mutate(fn func(*model.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Work on a copy so a failed mutation cannot leave partial changes.
	work := s.state
	if err := fn(&work); err != nil {
		return err
	}
	s.state = work
	return s.save()
}

// save writes atomically and keeps a rolling backup of the previous file.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	// Backup previous.
	if old, rerr := os.ReadFile(s.path); rerr == nil {
		bak := filepath.Join(s.backupDir, fmt.Sprintf("state-%d.json", time.Now().Unix()))
		_ = os.WriteFile(bak, old, 0o600)
		pruneBackups(s.backupDir, 20)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// FindConnection returns a pointer to a connection by ID (from a fresh copy).
func (s *Store) FindConnection(id string) (model.Connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.state.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return model.Connection{}, false
}

func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) <= keep {
		return
	}
	// entries are sorted by name; state-<unix>.json sorts chronologically.
	for _, e := range entries[:len(entries)-keep] {
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
