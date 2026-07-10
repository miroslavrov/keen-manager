package engine

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// SessionCookieName is the auth cookie set on successful login.
const SessionCookieName = "km_session"

const (
	pbkdf2Iter    = 100_000
	pbkdf2KeyLen  = 32
	sessionMaxAge = 30 * 24 * time.Hour
)

// Settings returns the user-facing settings plus read-only platform facts.
func (e *Engine) Settings() SettingsView {
	s := e.store.Settings()
	return SettingsView{
		Port:                  s.Port,
		AuthEnabled:           s.AuthEnabled,
		Theme:                 firstNonEmpty(s.Theme, "dark"),
		BackupOnChange:        s.BackupOnChange,
		RollbackTimeoutS:      s.RollbackTimeoutS,
		KillSwitchDefault:     s.KillSwitchDefault,
		AutoSelectIntervalMin: s.AutoSelectIntervalMin,
		XrayIntegration:       displayXrayIntegration(s.XrayIntegration),
		XrayLogLevel:          normalizeXrayLogLevel(s.XrayLogLevel),
		XrayMSSClamp:          s.XrayMSSClamp,
		Platform: PlatformView{
			Arch:        e.Platform.Arch,
			OSVersion:   e.Platform.OSVersion,
			EntwarePath: e.Platform.EntwarePath,
		},
	}
}

// SaveSettings applies a partial settings update. Recognised keys: port, theme,
// backup_on_change, rollback_timeout_s, kill_switch_default,
// auto_select_interval_min, auth_enabled, password.
func (e *Engine) SaveSettings(fields map[string]any) error {
	// newHash / clearAuth carry the credential change out of the Mutate closure
	// so it can be mirrored to the 0600 vault (the persistent store for the
	// hash — see loadAuthFromVault) after the state write succeeds.
	var newHash string
	var clearAuth bool
	var clearProxyDown bool
	err := e.store.Mutate(func(s *model.State) error {
		set := &s.Settings
		if v, ok := getInt(fields, "port"); ok && v > 0 && v < 65536 {
			set.Port = v
		}
		if v, ok := getString(fields, "theme"); ok && (v == "dark" || v == "light") {
			set.Theme = v
		}
		if v, ok := getBool(fields, "backup_on_change"); ok {
			set.BackupOnChange = v
		}
		if v, ok := getInt(fields, "rollback_timeout_s"); ok && v >= 0 {
			set.RollbackTimeoutS = v
		}
		if v, ok := getBool(fields, "kill_switch_default"); ok {
			set.KillSwitchDefault = v
		}
		if v, ok := getInt(fields, "auto_select_interval_min"); ok && v >= 0 {
			set.AutoSelectIntervalMin = v
		}
		if v, ok := getString(fields, "xray_integration"); ok {
			norm, valid := normalizeXrayIntegration(v)
			if !valid {
				return fmt.Errorf("invalid xray_integration %q (want auto, proxy, or tproxy)", v)
			}
			if norm != set.XrayIntegration {
				// Changing the mode gives the proxy-connection path a fresh attempt
				// (clears any latched "Proxy client unavailable" fallback).
				clearProxyDown = true
			}
			set.XrayIntegration = norm
		}
		if v, ok := getString(fields, "xray_log_level"); ok {
			lvl := strings.ToLower(strings.TrimSpace(v))
			switch lvl {
			case "", "debug", "info", "warning", "error", "none":
				// "" resets to the default (stored empty → warning at build time).
				if lvl == "warning" {
					lvl = "" // canonical default is the empty (omitted) value
				}
				set.XrayLogLevel = lvl
			default:
				return fmt.Errorf("invalid xray_log_level %q (want debug, info, warning, error, or none)", v)
			}
		}
		if v, ok := getInt(fields, "xray_mss_clamp"); ok {
			// Clamp obviously-bogus positive values to a sane TCP MSS window; 0 and
			// negatives keep their special meaning (default / disabled).
			if v > 0 && (v < 400 || v > 1460) {
				return fmt.Errorf("invalid xray_mss_clamp %d (use 0 for auto, a negative to disable, or an MSS of 400–1460)", v)
			}
			set.XrayMSSClamp = v
		}

		// Password / auth handling.
		if pw, ok := getString(fields, "password"); ok && pw != "" {
			newHash = hashPassword(pw)
			set.PasswordHash = newHash
			set.AuthEnabled = true
		}
		if v, ok := getBool(fields, "auth_enabled"); ok {
			if v && set.PasswordHash == "" {
				return fmt.Errorf("set a password before enabling authentication")
			}
			set.AuthEnabled = v
			// Turning auth off clears the stored hash so a stale credential
			// can never silently re-arm a login prompt after a restart.
			if !v {
				set.PasswordHash = ""
				clearAuth = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Persist the credential change to the vault so it survives a restart.
	if newHash != "" {
		e.vault.setAuthHash(newHash)
	} else if clearAuth {
		e.vault.setAuthHash("")
	}
	if clearProxyDown {
		e.setProxyClientDown(false)
	}
	e.Logf("settings saved")
	e.publishState()
	return nil
}

// normalizeXrayIntegration validates and canonicalises the Xray integration
// mode. "" and "auto" both mean auto-detect and are stored as "" (so the JSON
// field is omitted); "proxy"/"tproxy" are stored verbatim. The bool is false
// for any other value.
func normalizeXrayIntegration(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto":
		return "", true
	case xrayModeProxy:
		return xrayModeProxy, true
	case xrayModeTProxy:
		return xrayModeTProxy, true
	default:
		return "", false
	}
}

// displayXrayIntegration renders the stored value for the UI, showing the
// empty (auto-detect) default as "auto".
func displayXrayIntegration(v string) string {
	if strings.TrimSpace(v) == "" {
		return "auto"
	}
	return v
}

// ----- auth -----

// loadAuthFromVault reinstates the persisted password hash into the in-memory
// settings at startup and self-heals the "phantom password" lockout: if auth
// was left enabled but no hash survived (older builds only held the hash in
// memory), Login() could never succeed, locking the user out of the web UI.
// In that case auth is turned off so the UI is reachable again; the user can
// set a fresh password via the Settings page or `keen-manager passwd`.
func (e *Engine) loadAuthFromVault() {
	if hash := e.vault.authHash(); hash != "" {
		e.store.SetAuthHashInMemory(hash)
	}
	s := e.store.Settings()
	if s.AuthEnabled && s.PasswordHash == "" {
		if err := e.store.Mutate(func(st *model.State) error {
			st.Settings.AuthEnabled = false
			return nil
		}); err != nil {
			return
		}
		e.Logf("auth: was enabled but no stored password survived a restart — auth disabled so the web UI is reachable; set a new password (Settings, or `keen-manager passwd <password>`)")
	}
}

// SetPassword sets (or replaces) the web UI password and enables auth,
// persisting the hash to the 0600 vault. A running daemon reads the hash at
// startup, so a change made via the CLI takes effect after a service restart.
func (e *Engine) SetPassword(pw string) error {
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return fmt.Errorf("password must not be empty")
	}
	hash := hashPassword(pw)
	if err := e.store.Mutate(func(s *model.State) error {
		s.Settings.PasswordHash = hash
		s.Settings.AuthEnabled = true
		return nil
	}); err != nil {
		return err
	}
	e.vault.setAuthHash(hash)
	e.Logf("web UI password set (auth enabled)")
	e.publishState()
	return nil
}

// DisableAuth turns off the web UI login gate and clears the stored hash. Use
// it to recover from a lockout without wiping the rest of the configuration.
func (e *Engine) DisableAuth() error {
	if err := e.store.Mutate(func(s *model.State) error {
		s.Settings.AuthEnabled = false
		s.Settings.PasswordHash = ""
		return nil
	}); err != nil {
		return err
	}
	e.vault.setAuthHash("")
	e.Logf("web UI auth disabled")
	e.publishState()
	return nil
}

// AuthState reports whether auth is enabled and whether the caller is signed in.
func (e *Engine) AuthState(authenticated bool) AuthStateView {
	enabled := e.store.Settings().AuthEnabled
	return AuthStateView{Enabled: enabled, Authenticated: !enabled || authenticated}
}

// AuthEnabled reports whether authentication is required.
func (e *Engine) AuthEnabled() bool { return e.store.Settings().AuthEnabled }

// Login verifies the password and, on success, returns a new session token.
func (e *Engine) Login(password string) (string, error) {
	s := e.store.Settings()
	if !s.AuthEnabled {
		return "", nil // auth disabled: nothing to do
	}
	if s.PasswordHash == "" || !verifyPassword(password, s.PasswordHash) {
		return "", fmt.Errorf("invalid password")
	}
	tok := randomToken()
	e.sessMu.Lock()
	e.sessions[tok] = time.Now().Add(sessionMaxAge)
	e.sessMu.Unlock()
	e.Logf("login successful")
	return tok, nil
}

// Logout invalidates a session token.
func (e *Engine) Logout(token string) {
	if token == "" {
		return
	}
	e.sessMu.Lock()
	delete(e.sessions, token)
	e.sessMu.Unlock()
}

// ValidateSession reports whether a token maps to a live session.
func (e *Engine) ValidateSession(token string) bool {
	if token == "" {
		return false
	}
	e.sessMu.Lock()
	defer e.sessMu.Unlock()
	exp, ok := e.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(e.sessions, token)
		return false
	}
	return true
}

// ----- password hashing (PBKDF2-HMAC-SHA256, no external deps) -----

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	dk := pbkdf2SHA256([]byte(pw), salt, pbkdf2Iter, pbkdf2KeyLen)
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk))
}

func verifyPassword(pw, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(pw), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// pbkdf2SHA256 is a dependency-free PBKDF2 (RFC 8018) with HMAC-SHA256.
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, numBlocks*hashLen)
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for x := range t {
				t[x] ^= u[x]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ----- field coercion helpers (JSON numbers arrive as float64) -----

func getInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func getBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func getString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
