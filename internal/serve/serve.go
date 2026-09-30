// Package serve is the consolidated read-only loopback service: one mux that
// mounts Bigend, Signals and per-project Gurgeh behind a bearer token.
//
// It holds no decisions and imports no decisions code [B-16].
package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/daemon"
	gserver "github.com/mistakeknot/autarch/internal/gurgeh/server"
	"github.com/mistakeknot/autarch/pkg/netguard"
	"github.com/mistakeknot/autarch/pkg/signals"
)

// DefaultAddr does not collide with a running Bigend daemon on 8100.
const DefaultAddr = "127.0.0.1:8110"

// Config configures the service.
type Config struct {
	Addr         string
	ProjectDirs  []string
	TokenPath    string
	AllowOrigins []string
}

// Server is the consolidated service.
type Server struct {
	cfg      Config
	token    string
	resolver *Resolver
	broker   *signals.Broker
	gurgeh   *gurgehMounts
	handler  http.Handler
}

// New builds the service: it loads or creates the token and wires the mux.
func New(cfg Config) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}
	token, err := LoadOrCreateToken(cfg.TokenPath)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, token: token, resolver: NewResolver(cfg.ProjectDirs), broker: signals.NewBroker()}
	s.gurgeh = newGurgehMounts(s.resolver, func(root string) *gserver.Server {
		return gserver.New(root, gserver.WithPublisher(s.broker.Publish))
	})

	bigend := daemon.NewServer(daemon.Config{Addr: cfg.Addr, ProjectDirs: cfg.ProjectDirs})
	sig := signals.NewServer(s.broker)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		_ = writeJSON(w, s.resolver.Projects())
	})
	mux.Handle("/bigend/", http.StripPrefix("/bigend", bigend.Handler()))
	mux.Handle("/signals/", http.StripPrefix("/signals", sig.Handler()))
	mux.Handle("/gurgeh/{project}/", s.gurgeh)
	s.handler = guard(token, cfg.AllowOrigins, cfg.Addr, mux)
	return s, nil
}

// Handler returns the guarded mux.
func (s *Server) Handler() http.Handler { return s.handler }

// Listen checks the bind address is loopback and opens the listener.
func (s *Server) Listen() (net.Listener, error) {
	if err := netguard.EnsureLocalOnly(s.cfg.Addr); err != nil {
		return nil, err
	}
	return net.Listen("tcp", s.cfg.Addr)
}

// Serve serves on ln until ctx is done, then shuts down with a 5 s timeout.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		<-done
		return nil
	}
	return err
}

// Run is EnsureLocalOnly, Listen, Serve, and shutdown when ctx ends.
func (s *Server) Run(ctx context.Context) error {
	ln, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Build describes the running binary.
type Build struct {
	Revision  string `json:"revision"`
	ExeSHA256 string `json:"exe_sha256"`
}

var (
	buildOnce sync.Once
	buildInfo Build
)

// BuildInfo returns the VCS revision and the SHA-256 of the running executable [G-7].
func BuildInfo() Build {
	buildOnce.Do(func() {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, kv := range bi.Settings {
				if kv.Key == "vcs.revision" {
					buildInfo.Revision = kv.Value
				}
			}
		}
		if buildInfo.Revision == "" {
			buildInfo.Revision = "unknown"
		}
		if exe, err := os.Executable(); err == nil {
			if f, err := os.Open(exe); err == nil {
				h := sha256.New()
				if _, err := io.Copy(h, f); err == nil {
					buildInfo.ExeSHA256 = hex.EncodeToString(h.Sum(nil))
				}
				f.Close()
			}
		}
	})
	return buildInfo
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	projects := s.resolver.Projects()
	resp := map[string]any{
		"status":   "ok",
		"bigend":   "ok",
		"gurgeh":   map[string]any{"mounted": true, "projects": s.gurgeh.count()},
		"signals":  "ok",
		"projects": len(projects),
		"build":    BuildInfo(),
	}
	if err := writeJSON(w, resp); err != nil {
		fmt.Fprintln(os.Stderr, "serve: health:", err)
	}
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}
