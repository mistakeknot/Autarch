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
)

// Server is the Vauxhall daemon HTTP server
type Server struct {
	addr       string
	mux        *http.ServeMux
	server     *http.Server
	sessions   *SessionManager
	projects   *ProjectManager
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
		addr:      cfg.Addr,
		mux:       http.NewServeMux(),
		sessions:  NewSessionManager(),
		projects:  NewProjectManager(cfg.ProjectDirs),
		startedAt: time.Now(),
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

// Session represents a managed tmux session
type Session struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectPath string    `json:"project_path"`
	AgentType   string    `json:"agent_type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
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
	Path           string     `json:"path"`
	Name           string     `json:"name"`
	HasPraude      bool       `json:"has_praude"`
	HasTandemonium bool       `json:"has_tandemonium"`
	HasPollard     bool       `json:"has_pollard"`
	TaskStats      *TaskStats `json:"task_stats,omitempty"`
}

type TaskStats struct {
	Todo       int `json:"todo"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
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

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement WebSocket terminal streaming
	writeError(w, http.StatusNotImplemented, "websocket not yet implemented")
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
