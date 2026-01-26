// Package daemon provides the HTTP API server for Vauxhall (schmux-inspired).
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/tmux"
	"nhooyr.io/websocket"
)

// Server is the Vauxhall daemon HTTP server
type Server struct {
	addr       string
	mux        *http.ServeMux
	server     *http.Server
	sessions   *SessionManager
	projects   *ProjectManager
	tmuxClient *tmux.Client
	mu         sync.RWMutex
	startedAt  time.Time
}

// Config holds server configuration
type Config struct {
	Addr        string
	ProjectDirs []string
}

// NewServer creates a new daemon server
func NewServer(cfg Config) *Server {
	s := &Server{
		addr:       cfg.Addr,
		mux:        http.NewServeMux(),
		sessions:   NewSessionManager(),
		projects:   NewProjectManager(cfg.ProjectDirs),
		tmuxClient: tmux.NewClient(),
		startedAt:  time.Now(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Health check
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)

	// Sessions API (schmux-inspired)
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	s.mux.HandleFunc("POST /api/spawn", s.handleSpawn)
	s.mux.HandleFunc("DELETE /api/dispose/{id}", s.handleDispose)
	s.mux.HandleFunc("POST /api/sessions/{id}/restart", s.handleRestart)
	s.mux.HandleFunc("POST /api/sessions/{id}/attach", s.handleAttach)

	// Projects API
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("GET /api/projects/{path}/tasks", s.handleProjectTasks)

	// Agents API
	s.mux.HandleFunc("GET /api/agents", s.handleListAgents)
	s.mux.HandleFunc("GET /api/agents/{name}", s.handleGetAgent)

	// WebSocket for terminal streaming
	s.mux.HandleFunc("GET /ws/terminal/{id}", s.handleWebSocket)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:    s.addr,
		Handler: s.mux,
	}
	log.Printf("Vauxhall daemon starting on %s", s.addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Health response
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	StartedAt string `json:"started_at"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:    "ok",
		Version:   "0.1.0",
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
		StartedAt: s.startedAt.Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

// Status response with counts
type StatusResponse struct {
	Health      HealthResponse `json:"health"`
	SessionCount int           `json:"session_count"`
	ProjectCount int           `json:"project_count"`
	AgentCount   int           `json:"agent_count"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		Health: HealthResponse{
			Status:    "ok",
			Version:   "0.1.0",
			Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
			StartedAt: s.startedAt.Format(time.RFC3339),
		},
		SessionCount: s.sessions.Count(),
		ProjectCount: s.projects.Count(),
		AgentCount:   0, // TODO: integrate with agent registry
	}
	writeJSON(w, http.StatusOK, resp)
}

// SessionStatus represents the lifecycle state of a session
type SessionStatus string

const (
	SessionSpawning SessionStatus = "spawning" // Session being created
	SessionRunning  SessionStatus = "running"  // Session actively running
	SessionDone     SessionStatus = "done"     // Agent completed work
	SessionDisposed SessionStatus = "disposed" // Session destroyed
)

// Session represents a managed tmux session
type Session struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	ProjectPath string        `json:"project_path"`
	AgentType   string        `json:"agent_type"`
	Status      SessionStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`

	// Lifecycle tracking
	LastOutputAt time.Time `json:"last_output_at,omitempty"` // Last terminal output
	LastViewedAt time.Time `json:"last_viewed_at,omitempty"` // Last time user viewed

	// Git status (updated periodically)
	GitBranch     string `json:"git_branch,omitempty"`
	GitDirty      bool   `json:"git_dirty,omitempty"`
	CommitsAhead  int    `json:"commits_ahead,omitempty"`
	CommitsBehind int    `json:"commits_behind,omitempty"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.sessions.List()
	writeJSON(w, http.StatusOK, sessions)
}

// SpawnRequest to create a new session
type SpawnRequest struct {
	Name        string `json:"name"`
	ProjectPath string `json:"project_path"`
	AgentType   string `json:"agent_type"`
}

func (s *Server) handleSpawn(w http.ResponseWriter, r *http.Request) {
	var req SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.ProjectPath == "" {
		writeError(w, http.StatusBadRequest, "name and project_path required")
		return
	}

	session, err := s.sessions.Spawn(req.Name, req.ProjectPath, req.AgentType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleDispose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	if err := s.sessions.Dispose(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disposed"})
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	session, err := s.sessions.Restart(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	if err := s.sessions.Attach(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "attached"})
}

// Project represents a discovered project
type Project struct {
	Path           string        `json:"path"`
	Name           string        `json:"name"`
	HasGurgeh      bool          `json:"has_gurgeh"`
	HasTandemonium bool          `json:"has_tandemonium"`
	HasPollard     bool          `json:"has_pollard"`
	TaskStats      *TaskStats    `json:"task_stats,omitempty"`
	GurgStats    *GurgStats  `json:"gurg_stats,omitempty"`
	PollardStats   *PollardStats `json:"pollard_stats,omitempty"`
}

type TaskStats struct {
	Todo       int `json:"todo"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
}

type GurgStats struct {
	Total  int `json:"total"`
	Draft  int `json:"draft"`
	Active int `json:"active"`
	Done   int `json:"done"`
}

type PollardStats struct {
	Sources    int    `json:"sources"`
	Insights   int    `json:"insights"`
	Reports    int    `json:"reports"`
	LastReport string `json:"last_report,omitempty"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects := s.projects.List()
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleProjectTasks(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "project path required")
		return
	}

	tasks, err := s.projects.GetTasks(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

// Agent represents a registered AI agent
type Agent struct {
	Name        string `json:"name"`
	Program     string `json:"program"`
	Model       string `json:"model"`
	ProjectPath string `json:"project_path"`
	Status      string `json:"status"`
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	// TODO: Integrate with agent registry
	writeJSON(w, http.StatusOK, []Agent{})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "agent name required")
		return
	}

	// TODO: Integrate with agent registry
	writeError(w, http.StatusNotFound, "agent not found")
}

// handleWebSocket streams terminal output for a session in real-time.
//
// Protocol:
// - Server sends terminal output as text messages at ~10 FPS
// - Server sends only changed content (diff-based updates)
// - Client can send "ping" to keep connection alive
// - Connection closes when session ends or client disconnects
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	// Find the session
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// Accept the WebSocket connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"}, // Allow all origins for local development
	})
	if err != nil {
		log.Printf("websocket accept failed: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "closing")

	ctx := r.Context()

	// Stream terminal output at ~10 FPS
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastOutput string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Capture pane output
			output, err := s.tmuxClient.CapturePane(session.Name, 50)
			if err != nil {
				// Session may have ended
				errMsg := struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "error",
					Message: "session ended or capture failed",
				}
				data, _ := json.Marshal(errMsg)
				conn.Write(ctx, websocket.MessageText, data)
				return
			}

			// Only send if output changed (reduces bandwidth)
			if output != lastOutput {
				msg := struct {
					Type    string `json:"type"`
					Content string `json:"content"`
				}{
					Type:    "output",
					Content: output,
				}
				data, _ := json.Marshal(msg)

				err = conn.Write(ctx, websocket.MessageText, data)
				if err != nil {
					return // Client disconnected
				}
				lastOutput = output
			}
		}
	}
}

// Helper functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// WritePIDFile writes the daemon PID to a file
func WritePIDFile(path string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// RemovePIDFile removes the daemon PID file
func RemovePIDFile(path string) error {
	return os.Remove(path)
}
