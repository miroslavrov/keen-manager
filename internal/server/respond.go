// Package server exposes the keen-manager engine over a JSON HTTP API and serves
// the embedded web UI same-origin. It is intentionally thin: every handler
// validates input, calls one engine method, and renders the result.
package server

import (
	"encoding/json"
	"io"
	"net/http"
)

// writeJSON renders v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeOK renders the generic {"ok":true} acknowledgement.
func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// writeErr renders {"error": msg} with the given status.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// noStore marks a response as never-cacheable. Auth-sensitive endpoints use it
// so a stale browser/proxy cache can never make the UI believe it is signed in
// (or signed out) when the daemon disagrees — the root of the "sometimes lets
// me in without a password" class of bugs.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
}

// readJSON decodes a JSON request body into dst (size-limited).
func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20)) // 4 MiB cap
	return dec.Decode(dst)
}
