package server

import (
	"net/http"
	"time"

	"github.com/miroslavrov/keen-manager/internal/engine"
)

// authed reports whether the request is authenticated (always true when auth is
// disabled).
func (s *Server) authed(r *http.Request) bool {
	if !s.eng.AuthEnabled() {
		return true
	}
	c, err := r.Cookie(engine.SessionCookieName)
	if err != nil {
		return false
	}
	return s.eng.ValidateSession(c.Value)
}

// requireAuth wraps a handler so it returns 401 for unauthenticated callers.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// handleAuthState is GET /api/auth.
func (s *Server) handleAuthState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.AuthState(s.authed(r)))
}

// handleLogin is POST /api/login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tok, err := s.eng.Login(body.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid password")
		return
	}
	if tok != "" {
		setSessionCookie(w, tok)
	}
	writeOK(w)
}

// handleLogout is POST /api/logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(engine.SessionCookieName); err == nil {
		s.eng.Logout(c.Value)
	}
	clearSessionCookie(w)
	writeOK(w)
}

// setSessionCookie issues the session cookie. It is HttpOnly and SameSite=Lax;
// it is NOT marked Secure because the UI is served over plain HTTP on the LAN.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     engine.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     engine.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
