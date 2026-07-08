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

		// Password / auth handling.
		if pw, ok := getString(fields, "password"); ok && pw != "" {
			set.PasswordHash = hashPassword(pw)
			set.AuthEnabled = true
		}
		if v, ok := getBool(fields, "auth_enabled"); ok {
			if v && set.PasswordHash == "" {
				return fmt.Errorf("set a password before enabling authentication")
			}
			set.AuthEnabled = v
		}
		return nil
	})
	if err != nil {
		return err
	}
	e.Logf("settings saved")
	e.publishState()
	return nil
}

// ----- auth -----

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
