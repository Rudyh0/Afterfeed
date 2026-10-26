// Package server is Afterfeed's HTTP server: the JSON API and the web app.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rudyh0/afterfeed/internal/auth"
	"github.com/rudyh0/afterfeed/internal/config"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
	"github.com/rudyh0/afterfeed/internal/store"
	"github.com/rudyh0/afterfeed/internal/version"
)

// Server serves the API and the web app.
type Server struct {
	cfg   *config.Config
	store *store.Store
	mux   *http.ServeMux
	deps  Deps
}

// Deps are the services the API uses.
type Deps struct {
	Auth    *auth.Service
	Imports *importer.Manager
	Library *library.Library
	Media   *media.Service
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, deps Deps) *Server {
	s := &Server{cfg: cfg, store: st, mux: http.NewServeMux(), deps: deps}
	s.routes()
	return s
}

// Handler returns the root HTTP handler with all middleware applied.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.csrf(h)
	h = securityHeaders(h)
	h = recoverer(h)
	h = requestLogger(h)
	return h
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Addr(),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "version", version.Version)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", s.handle(s.health))
	m.HandleFunc("GET /api/state", s.handle(s.stateWithAuth))
	s.authRoutes()
	s.settingsRoutes()
	s.importRoutes()
	s.mediaRoutes()

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, errNotFound())
	})
	m.Handle("/", spaHandler())
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	if err := s.store.R.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
	return nil
}

// --- middleware ---

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			return
		}
		level := slog.LevelDebug
		if rec.status >= 500 {
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration", time.Since(start).Round(time.Millisecond))
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "value", v, "stack", string(debug.Stack()))
				writeError(w, r, errors.New("panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(self)")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy())
		next.ServeHTTP(w, r)
	})
}

// csrf rejects state-changing API requests that do not carry the custom
// header the web app sends. Browsers do not let other sites add custom headers
// to cross-site requests without a CORS preflight, which Afterfeed never allows.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if r.Header.Get("X-Afterfeed") != "1" {
					writeError(w, r, errForbidden("csrf"))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// tileHosts are the hosts the map may load tiles and styles from. They are set
// from the map settings when they change.
var tileHosts atomic.Pointer[[]string]

// SetMapHosts updates the hosts allowed by the content security policy for the map.
func SetMapHosts(hosts []string) { tileHosts.Store(&hosts) }

func contentSecurityPolicy() string {
	extra := ""
	if hosts := tileHosts.Load(); hosts != nil {
		for _, h := range *hosts {
			extra += " " + h
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:" + extra,
		"media-src 'self' blob:",
		"font-src 'self'",
		"connect-src 'self'" + extra,
		"worker-src 'self' blob:",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
}
