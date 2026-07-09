package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// vault stores full Xray server records — including secrets (UUID, passwords,
// reality keys) — separately from the main state document.
//
// Why a separate store: model.Server marks its secret fields json:"-" so that
// the domain state (and anything derived from it) can never accidentally leak
// credentials to the UI. That safety property means secrets are also absent
// from state.json, so the engine keeps them here, in a 0600 file the API layer
// never serialises. Records are keyed by connection ID.
type vault struct {
	mu      sync.Mutex
	path    string
	Servers map[string]storedServer `json:"servers"`
	// Auth persists the web UI credential. model.Settings.PasswordHash is
	// json:"-" (so a hash never leaks to the UI and never lands in state.json
	// or its backups), which historically meant the hash was lost on restart
	// while auth_enabled stayed true — locking the user out (HANDOFF §0 [P1]).
	// Keeping the hash here, in the same 0600 file as server secrets, persists
	// it across restarts without weakening that no-secrets-in-state property.
	Auth vaultAuth `json:"auth"`
}

// vaultAuth is the persisted web UI credential block.
type vaultAuth struct {
	PasswordHash string `json:"password_hash,omitempty"`
}

// storedServer mirrors model.Server with explicit JSON tags so every field —
// secrets included — round-trips to disk.
type storedServer struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Location      string         `json:"location,omitempty"`
	Protocol      model.Protocol `json:"protocol"`
	Address       string         `json:"address"`
	Port          int            `json:"port"`
	UUID          string         `json:"uuid,omitempty"`
	Password      string         `json:"password,omitempty"`
	AlterID       int            `json:"alter_id,omitempty"`
	Cipher        string         `json:"cipher,omitempty"`
	Flow          string         `json:"flow,omitempty"`
	Security      string         `json:"security,omitempty"`
	Network       string         `json:"network,omitempty"`
	SNI           string         `json:"sni,omitempty"`
	Fingerprint   string         `json:"fingerprint,omitempty"`
	PublicKey     string         `json:"public_key,omitempty"`
	ShortID       string         `json:"short_id,omitempty"`
	SpiderX       string         `json:"spider_x,omitempty"`
	Path          string         `json:"path,omitempty"`
	Host          string         `json:"host,omitempty"`
	ALPN          []string       `json:"alpn,omitempty"`
	AllowInsecure bool           `json:"allow_insecure,omitempty"`
	Raw           string         `json:"raw,omitempty"`
}

func fromModel(s model.Server) storedServer {
	return storedServer{
		ID: s.ID, Name: s.Name, Location: s.Location, Protocol: s.Protocol,
		Address: s.Address, Port: s.Port, UUID: s.UUID, Password: s.Password,
		AlterID: s.AlterID, Cipher: s.Cipher, Flow: s.Flow, Security: s.Security,
		Network: s.Network, SNI: s.SNI, Fingerprint: s.Fingerprint,
		PublicKey: s.PublicKey, ShortID: s.ShortID, SpiderX: s.SpiderX,
		Path: s.Path, Host: s.Host, ALPN: s.ALPN, AllowInsecure: s.AllowInsecure,
		Raw: s.Raw,
	}
}

func (r storedServer) toModel() model.Server {
	return model.Server{
		ID: r.ID, Name: r.Name, Location: r.Location, Protocol: r.Protocol,
		Address: r.Address, Port: r.Port, UUID: r.UUID, Password: r.Password,
		AlterID: r.AlterID, Cipher: r.Cipher, Flow: r.Flow, Security: r.Security,
		Network: r.Network, SNI: r.SNI, Fingerprint: r.Fingerprint,
		PublicKey: r.PublicKey, ShortID: r.ShortID, SpiderX: r.SpiderX,
		Path: r.Path, Host: r.Host, ALPN: r.ALPN, AllowInsecure: r.AllowInsecure,
		Raw: r.Raw,
	}
}

func openVault(path string) (*vault, error) {
	v := &vault{path: path, Servers: map[string]storedServer{}}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(data, v) // tolerate an empty/partial file
	if v.Servers == nil {
		v.Servers = map[string]storedServer{}
	}
	return v, nil
}

func (v *vault) put(connID string, s model.Server) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Servers[connID] = fromModel(s)
	_ = v.save()
}

func (v *vault) get(connID string) (model.Server, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	r, ok := v.Servers[connID]
	if !ok {
		return model.Server{}, false
	}
	return r.toModel(), true
}

func (v *vault) delete(connID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.Servers[connID]; ok {
		delete(v.Servers, connID)
		_ = v.save()
	}
}

// authHash returns the persisted web UI password hash ("" when unset).
func (v *vault) authHash() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.Auth.PasswordHash
}

// setAuthHash persists (or clears, when hash is "") the web UI password hash.
func (v *vault) setAuthHash(hash string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Auth.PasswordHash = hash
	_ = v.save()
}

// save writes the vault atomically with owner-only permissions.
func (v *vault) save() error {
	data, err := json.MarshalIndent(struct {
		Servers map[string]storedServer `json:"servers"`
		Auth    vaultAuth               `json:"auth"`
	}{v.Servers, v.Auth}, "", "  ")
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}
