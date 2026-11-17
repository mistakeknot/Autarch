package serve

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/mistakeknot/autarch/pkg/callerlog"
)

type guardOutcomeKey struct{}

func recordGuard(r *http.Request, outcome string) {
	if p, ok := r.Context().Value(guardOutcomeKey{}).(*string); ok {
		*p = outcome
	}
}

type auditWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *auditWriter) WriteHeader(code int) {
	if (code >= 200 || code == http.StatusSwitchingProtocols) && w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *auditWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
func (w *auditWriter) Flush() {
	if w.status == 0 {
		w.status = 200
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *auditWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	return c, rw, err
}
func callerLogging(log *callerlog.Log, mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outcome := "ok"
		r = r.WithContext(context.WithValue(r.Context(), guardOutcomeKey{}, &outcome))
		// Resolve before the guard so rejected requests also get sanitized patterns.
		_, pattern := mux.Handler(r)
		// ServeMux may return a concrete redirect path instead of a registered pattern.
		switch pattern {
		case "GET /health", "GET /api/projects", "/bigend/", "/signals/", "/gurgeh/{project}/", "GET /gurgeh/{project}/":
		default:
			pattern = ""
		}
		aw := &auditWriter{ResponseWriter: w}
		next.ServeHTTP(preserveWriter(aw), r)
		status := aw.status
		if status == 0 {
			status = 200
		}
		file := r.Method == "GET" && pattern == "GET /api/projects"
		read := r.Method == "GET" && (file || strings.HasPrefix(pattern, "/signals/") || strings.HasPrefix(pattern, "/gurgeh/") || strings.HasPrefix(pattern, "GET /gurgeh/"))
		thread := strings.ToValidUTF8(r.Header.Get("X-Autarch-Thread"), "")
		if len(thread) > 128 {
			thread = strings.ToValidUTF8(thread[:128], "")
		}
		denied := outcome != "ok"
		log.Record(map[string]any{"method": r.Method, "route": pattern, "status": status, "guard": outcome, "claimed_thread": thread, "peer": "unknown", "would_deny_cards:file": denied || !file, "would_deny_cards:read": denied || !read, "would_deny_admin": denied}, r.RemoteAddr)
	})
}

func (w *auditWriter) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type baseWriter interface {
	http.ResponseWriter
	Unwrap() http.ResponseWriter
}

// Preserve optional streaming interfaces so handlers choose the same response path.
func preserveWriter(w *auditWriter) http.ResponseWriter {
	mask := 0
	if _, ok := w.ResponseWriter.(http.Flusher); ok {
		mask |= 1
	}
	if _, ok := w.ResponseWriter.(http.Hijacker); ok {
		mask |= 2
	}
	if _, ok := w.ResponseWriter.(http.Pusher); ok {
		mask |= 4
	}
	switch mask {
	case 1:
		return struct {
			baseWriter
			http.Flusher
		}{w, w}
	case 2:
		return struct {
			baseWriter
			http.Hijacker
		}{w, w}
	case 3:
		return struct {
			baseWriter
			http.Flusher
			http.Hijacker
		}{w, w, w}
	case 4:
		return struct {
			baseWriter
			http.Pusher
		}{w, w}
	case 5:
		return struct {
			baseWriter
			http.Flusher
			http.Pusher
		}{w, w, w}
	case 6:
		return struct {
			baseWriter
			http.Hijacker
			http.Pusher
		}{w, w, w}
	case 7:
		return struct {
			baseWriter
			http.Flusher
			http.Hijacker
			http.Pusher
		}{w, w, w, w}
	default:
		return struct{ baseWriter }{w}
	}
}
