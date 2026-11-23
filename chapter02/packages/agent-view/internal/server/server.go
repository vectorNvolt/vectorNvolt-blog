// Package server is the read-only HTTP surface of agent_view: exec listing, exec file download,
// a change feed (SSE) and the embedded UI. Only GET and HEAD are served.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"agent-view/internal/wsread"
)

// CSP is sent on every response.
const CSP = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; worker-src 'self' blob:"

const uiMissing = "agent_view: UI bundle not built (run npm run build in packages/agent-view/web)\n"

// Config wires the server. Static and WS are injected so tests can use fstest.MapFS and t.TempDir().
type Config struct {
	WS           *wsread.Reader
	Static       fs.FS
	PollInterval time.Duration // SSE fingerprint poll; default 1s
	PingInterval time.Duration // SSE keep-alive comment; default 15s
}

type srv struct {
	cfg    Config
	static http.Handler
}

// New returns the handler with all guards applied.
func New(cfg Config) http.Handler {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 15 * time.Second
	}
	s := &srv{cfg: cfg}
	if cfg.Static != nil {
		s.static = http.FileServerFS(cfg.Static)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/execs", s.list)
	mux.HandleFunc("/api/execs/{id}/file", s.file)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/", s.ui)
	return guard(mux)
}

// guard sets the security headers, rejects non-loopback Host headers (DNS rebinding) and
// every method but GET and HEAD.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", CSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if !loopbackHost(r.Host) {
			writeErr(w, http.StatusMisdirectedRequest, "misdirected request")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost accepts localhost or a loopback IP literal, with or without a port.
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	} else {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *srv) list(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"execs": s.cfg.WS.ListExecs()})
}

func isText(rel string) bool {
	if strings.HasPrefix(rel, "in/") {
		return true
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".log", ".json", ".cir", ".txt":
		return true
	}
	return false
}

func (s *srv) file(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	f, st, err := s.cfg.WS.OpenFile(r.PathValue("id"), rel)
	if err != nil {
		switch {
		case errors.Is(err, wsread.ErrBadRequest):
			writeErr(w, http.StatusBadRequest, "bad request")
		case errors.Is(err, wsread.ErrTooBig):
			writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
		default:
			writeErr(w, http.StatusNotFound, "not found")
		}
		return
	}
	defer f.Close()
	ct := "application/octet-stream"
	if clean, cerr := wsread.CleanRel(rel); cerr == nil && isText(clean) {
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (s *srv) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	// No write deadline for this long-lived stream (the http.Server has a WriteTimeout for the rest).
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	last := s.cfg.WS.Fingerprint()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	send := func(s string) bool {
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(": connected\n\n") {
		return
	}
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()
	ping := time.NewTicker(s.cfg.PingInterval)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			if cur := s.cfg.WS.Fingerprint(); cur != last {
				last = cur
				if !send("event: execs\ndata: {}\n\n") {
					return
				}
			}
		case <-ping.C:
			if !send(": ping\n\n") {
				return
			}
		}
	}
}

func (s *srv) ui(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || s.static == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if r.URL.Path == "/" {
		if _, err := fs.Stat(s.cfg.Static, "index.html"); err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(uiMissing))
			return
		}
	}
	for _, seg := range strings.Split(r.URL.Path, "/") {
		if strings.HasPrefix(seg, ".") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	s.static.ServeHTTP(w, r)
}

// NewHTTPServer wraps a handler with the timeouts used in production.
func NewHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // cleared per request on /api/events
		IdleTimeout:       60 * time.Second,
	}
}
