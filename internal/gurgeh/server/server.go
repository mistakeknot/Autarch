package server

import (
	"context"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gsignals "github.com/mistakeknot/autarch/internal/gurgeh/signals"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/httpapi"
	"github.com/mistakeknot/autarch/pkg/netguard"
	psignals "github.com/mistakeknot/autarch/pkg/signals"
)

type Server struct {
	root string
	mux  *http.ServeMux
	srv  *http.Server
}

func New(root string) *Server {
	return &Server{root: root, mux: http.NewServeMux()}
}

func (s *Server) ListenAndServe(addr string) error {
	if err := netguard.EnsureLocalOnly(addr); err != nil {
		return err
	}
	s.routes()
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return s.srv.ListenAndServe()
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/specs", s.handleSpecs)
	s.mux.HandleFunc("/api/specs/", s.handleSpec)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	httpapi.WriteOK(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (s *Server) handleSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	list, _ := specs.LoadSummaries(specsDir(s.root))
	// Stable ordering by ID for pagination.
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	cursor, limit := parsePagination(r, 50)
	paged, next := paginate(list, cursor, limit)
	meta := &httpapi.Meta{Cursor: next, Limit: limit}
	httpapi.WriteOK(w, http.StatusOK, paged, meta)
}

func (s *Server) handleSpec(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/specs/")
	path = strings.Trim(path, "/")
	if path == "" {
		httpapi.WriteError(w, http.StatusNotFound, httpapi.ErrNotFound, "spec not found", nil, false)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	specPath, ok := specPathForID(s.root, id)
	if !ok {
		httpapi.WriteError(w, http.StatusNotFound, httpapi.ErrNotFound, "spec not found", nil, false)
		return
	}
	spec, err := specs.LoadSpec(specPath)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, httpapi.ErrInternal, "failed to load spec", nil, false)
		return
	}
	s.refreshSignals(r.Context(), spec)
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		httpapi.WriteOK(w, http.StatusOK, spec, nil)
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		switch parts[1] {
		case "requirements":
			httpapi.WriteOK(w, http.StatusOK, spec.Requirements, nil)
			return
		case "cujs":
			httpapi.WriteOK(w, http.StatusOK, spec.CriticalUserJourneys, nil)
			return
		case "hypotheses":
			httpapi.WriteOK(w, http.StatusOK, spec.Hypotheses, nil)
			return
		case "history":
			revisions, err := specs.LoadHistory(s.root, id)
			if err != nil {
				httpapi.WriteError(w, http.StatusInternalServerError, httpapi.ErrInternal, "failed to load history", nil, false)
				return
			}
			httpapi.WriteOK(w, http.StatusOK, revisions, nil)
			return
		}
	}
	httpapi.WriteError(w, http.StatusNotFound, httpapi.ErrNotFound, "spec not found", nil, false)
}

func (s *Server) refreshSignals(ctx context.Context, spec specs.Spec) {
	store, err := gsignals.NewStore(s.root)
	if err != nil {
		return
	}
	defer store.Close()

	emitter := gsignals.NewEmitter()
	sigs := emitter.CheckSpec(&spec)
	if len(sigs) == 0 {
		return
	}
	_ = store.EmitAll(sigs)

	client := psignals.NewClient(psignals.DefaultServerURL())
	publishCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, sig := range sigs {
		_ = client.Publish(publishCtx, sig)
	}
}

func specsDir(root string) string {
	return filepath.Join(root, ".gurgeh", "specs")
}

func specPathForID(root, id string) (string, bool) {
	list, _ := specs.LoadSummaries(specsDir(root))
	for _, s := range list {
		if s.ID == id {
			return s.Path, true
		}
	}
	return "", false
}

func parsePagination(r *http.Request, defaultLimit int) (int, int) {
	cursor := 0
	if v := r.URL.Query().Get("cursor"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			cursor = parsed
		}
	}
	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	return cursor, limit
}

func paginate[T any](items []T, cursor int, limit int) ([]T, string) {
	if cursor >= len(items) {
		return []T{}, ""
	}
	end := cursor + limit
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return items[cursor:end], next
}

func methodNotAllowed(w http.ResponseWriter) {
	httpapi.WriteError(w, http.StatusMethodNotAllowed, httpapi.ErrInvalidRequest, "method not allowed", nil, false)
}
