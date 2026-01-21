package web

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the HTTP server for Vauxhall
type Server struct {
	cfg       config.ServerConfig
	agg       *aggregator.Aggregator
	templates map[string]*template.Template
	srv       *http.Server
}

// NewServer creates a new web server
func NewServer(cfg config.ServerConfig, agg *aggregator.Aggregator) *Server {
	s := &Server{
		cfg:       cfg,
		agg:       agg,
		templates: make(map[string]*template.Template),
	}

	// Template functions
	funcs := template.FuncMap{
		"basename": filepath.Base,
	}

	// Load templates - each page gets its own template with layout
	tmplFS, _ := fs.Sub(templateFS, "templates")
	layoutBytes, _ := fs.ReadFile(tmplFS, "layout.html")
	layoutStr := string(layoutBytes)

	// Pages to load
	pages := []string{"dashboard.html", "projects.html", "agents.html", "sessions.html", "tasks.html", "agent_detail.html"}

	for _, page := range pages {
		pageBytes, err := fs.ReadFile(tmplFS, page)
		if err != nil {
			continue
		}
		// Create a fresh template with layout + page
		tmpl := template.Must(template.New("").Funcs(funcs).Parse(layoutStr))
		template.Must(tmpl.Parse(string(pageBytes)))
		s.templates[page] = tmpl
	}

	return s
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe(addr string) error {
	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/projects", s.handleProjects)
	mux.HandleFunc("/projects/", s.handleProjectRoutes)
	mux.HandleFunc("/agents", s.handleAgents)
	mux.HandleFunc("/agents/", s.handleAgentDetail)
	mux.HandleFunc("/sessions", s.handleSessions)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/agents", s.handleAgentsAPI)

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	state := s.agg.GetState()
	s.render(w, "dashboard.html", map[string]any{
		"State": state,
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	state := s.agg.GetState()
	s.render(w, "projects.html", map[string]any{
		"Projects": state.Projects,
	})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	state := s.agg.GetState()
	s.render(w, "agents.html", map[string]any{
		"Agents": state.Agents,
	})
}

func (s *Server) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	// Extract agent name from /agents/{name}
	agentName := strings.TrimPrefix(r.URL.Path, "/agents/")
	if agentName == "" {
		http.Redirect(w, r, "/agents", http.StatusFound)
		return
	}

	// Get agent from aggregator state
	agent := s.agg.GetAgent(agentName)
	if agent == nil {
		slog.Warn("agent not found", "name", agentName)
		http.NotFound(w, r)
		return
	}

	// Get detailed agent info from agent mail
	mailAgent, err := s.agg.GetAgentMailAgent(agentName)
	if err != nil {
		slog.Error("failed to get agent mail agent", "name", agentName, "error", err)
	}

	// Get messages and reservations if we have agent mail data
	var messages []any
	var reservations []any
	if mailAgent != nil {
		msgs, err := s.agg.GetAgentMessages(mailAgent.ID, 20)
		if err != nil {
			slog.Error("failed to get agent messages", "error", err)
		} else {
			for _, m := range msgs {
				messages = append(messages, m)
			}
		}

		res, err := s.agg.GetAgentReservations(mailAgent.ID)
		if err != nil {
			slog.Error("failed to get agent reservations", "error", err)
		} else {
			for _, r := range res {
				reservations = append(reservations, r)
			}
		}
	}

	s.render(w, "agent_detail.html", map[string]any{
		"Agent":        agent,
		"Messages":     messages,
		"Reservations": reservations,
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	state := s.agg.GetState()
	s.render(w, "sessions.html", map[string]any{
		"Sessions": state.Sessions,
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if err := s.agg.Refresh(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back or return JSON
	if r.Header.Get("HX-Request") == "true" {
		// htmx request - return updated dashboard content
		s.handleDashboard(w, r)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Server) handleAgentsAPI(w http.ResponseWriter, r *http.Request) {
	state := s.agg.GetState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Agents)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, ok := s.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleProjectRoutes handles /projects/* routes
func (s *Server) handleProjectRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse path: /projects/{project_path}/tasks or /projects/{project_path}
	path := strings.TrimPrefix(r.URL.Path, "/projects")

	// Empty path means redirect to /projects
	if path == "" || path == "/" {
		http.Redirect(w, r, "/projects", http.StatusFound)
		return
	}

	// Remove leading slash if present (project paths are absolute like /root/projects/...)
	// Path format: /{absolute_project_path}/tasks or /{absolute_project_path}
	// Example: //root/projects/Tandemonium/tasks -> /root/projects/Tandemonium

	// Check if this is a tasks route
	if strings.HasSuffix(path, "/tasks") {
		projectPath := strings.TrimSuffix(path, "/tasks")
		projectPath = strings.TrimLeft(projectPath, "/") // Remove all leading /
		projectPath = "/" + projectPath                   // Add back single leading /
		s.handleProjectTasks(w, r, projectPath)
		return
	}

	// Otherwise it's a project detail route
	projectPath := strings.TrimLeft(path, "/")
	projectPath = "/" + projectPath
	s.handleProjectDetail(w, r, projectPath)
}

// handleProjectDetail shows project details
func (s *Server) handleProjectDetail(w http.ResponseWriter, r *http.Request, projectPath string) {
	project := s.agg.GetProject(projectPath)
	if project == nil {
		http.NotFound(w, r)
		return
	}

	// For now, redirect to tasks if project has tandemonium
	if project.HasTandemonium {
		http.Redirect(w, r, "/projects/"+projectPath+"/tasks", http.StatusFound)
		return
	}

	// TODO: Create a proper project detail template
	s.render(w, "projects.html", map[string]any{
		"Projects": []any{project},
	})
}

// handleProjectTasks shows the task board for a project
func (s *Server) handleProjectTasks(w http.ResponseWriter, r *http.Request, projectPath string) {
	project := s.agg.GetProject(projectPath)
	if project == nil {
		slog.Warn("project not found", "path", projectPath)
		http.NotFound(w, r)
		return
	}

	if !project.HasTandemonium {
		http.Error(w, "Project does not have Tandemonium", http.StatusNotFound)
		return
	}

	tasks, err := s.agg.GetProjectTasks(projectPath)
	if err != nil {
		slog.Error("failed to get tasks", "project", projectPath, "error", err)
		http.Error(w, "Failed to load tasks", http.StatusInternalServerError)
		return
	}

	// Extract tasks by status for template
	data := map[string]any{
		"Project":         project,
		"TodoTasks":       tasks["todo"],
		"InProgressTasks": tasks["in_progress"],
		"ReviewTasks":     tasks["review"],
		"DoneTasks":       tasks["done"],
		"BlockedTasks":    tasks["blocked"],
	}

	s.render(w, "tasks.html", data)
}
