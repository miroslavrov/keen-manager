package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/engine"
	"github.com/miroslavrov/keen-manager/internal/webui"
)

// Server wraps the engine with an HTTP mux (JSON API + embedded UI).
type Server struct {
	eng     *engine.Engine
	handler http.Handler
}

// New builds the HTTP server around an engine.
func New(eng *engine.Engine) *Server {
	s := &Server{eng: eng}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	// Everything not matched by an API route falls through to the SPA.
	mux.Handle("/", s.staticHandler())
	s.handler = logRequests(mux)
	return s
}

// Handler returns the composed HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// ListenAndServe runs the daemon until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// staticHandler serves the embedded front-end with SPA fallback: unknown paths
// (client-side routes) resolve to index.html.
func (s *Server) staticHandler() http.Handler {
	sub := webui.FS()
	fileServer := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" {
			clean = "index.html"
		}
		if f, err := sub.Open(clean); err == nil {
			_ = f.Close()
			// Long-cache fingerprinted assets; keep index.html fresh.
			if strings.HasPrefix(clean, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if index == nil {
			http.Error(w, "front-end not built", http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// logRequests is a minimal access log that mirrors into the engine log stream.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// notFoundJSON handles unmatched /api/* paths (so they don't fall into the SPA).
func notFoundJSON(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, fmt.Sprintf("no such endpoint: %s %s", r.Method, r.URL.Path))
}
