package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/miroslavrov/keen-manager/internal/engine"
)

// handleEvents is GET /api/events — a Server-Sent Events stream that pushes
// "state" (refetch) and "log" (live log line) frames to the browser.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	id, ch := s.eng.Subscribe()
	defer s.eng.Unsubscribe(id)

	// Prime the client so it renders immediately.
	fmt.Fprint(w, ": connected\n\n")
	writeEvent(w, engine.Event{Type: engine.EventState})
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			writeEvent(w, ev)
			flusher.Flush()
		}
	}
}

// writeEvent renders a single SSE frame.
func writeEvent(w http.ResponseWriter, ev engine.Event) {
	name := string(ev.Type)
	if name == "" {
		name = "message"
	}
	data := "{}"
	if ev.Data != nil {
		if b, err := json.Marshal(ev.Data); err == nil {
			data = string(b)
		}
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
}
