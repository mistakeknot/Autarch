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
	"sort"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/agentmail"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/discovery"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tandemonium"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server is the HTTP server for Vauxhall
type Server struct {
	cfg          config.ServerConfig
	agg          aggregatorAPI
	statusClient statusClient
	templates    map[string]*template.Template
	srv          *http.Server
}

type aggregatorAPI interface {
	GetState() aggregator.State
	Refresh(ctx context.Context) error
	GetProject(path string) *discovery.Project
	GetProjectTasks(projectPath string) (map[string][]tandemonium.Task, error)
	GetAgent(name string) *aggregator.Agent
	GetAgentMailAgent(name string) (*agentmail.Agent, error)
	GetAgentMessages(agentID int, limit int) ([]agentmail.Message, error)
	GetAgentReservations(agentID int) ([]agentmail.FileReservation, error)
	GetActiveReservations() ([]agentmail.FileReservation, error)
	NewSession(name, projectPath, agentType string) error
	RestartSession(name, projectPath, agentType string) error
	RenameSession(oldName, newName string) error
	ForkSession(name, projectPath, agentType string) error
	AttachSession(name string) error
	StartMCP(ctx context.Context, projectPath, component string) error
	StopMCP(projectPath, component string) error
}

type statusClient interface {
	DetectStatus(name string) tmux.Status
}

type webGroup[T any] struct {
	Name  string
	Path  string
	Items []T
}

// NewServer creates a new web server
func NewServer(cfg config.ServerConfig, agg aggregatorAPI) *Server {
	s := &Server{
		cfg:          cfg,
		agg:          agg,
		statusClient: tmux.NewClient(),
		templates:    make(map[string]*template.Template),
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
	mux.HandleFunc("/api/sessions/new", s.handleSessionNew)
	mux.HandleFunc("/api/sessions/", s.handleSessionAction)
	mux.HandleFunc("/api/projects/", s.handleProjectMCPAction)
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
		"MCP":      state.MCP,
	})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	state := s.agg.GetState()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	filterState := parseFilter(query)
	agents := state.Agents
	var statusBySession map[string]tmux.Status
	if filterState.Raw != "" {
		if len(filterState.Statuses) > 0 && s.statusClient != nil {
			statusBySession = make(map[string]tmux.Status, len(state.Sessions))
			for _, session := range state.Sessions {
				statusBySession[session.Name] = s.statusClient.DetectStatus(session.Name)
			}
		}
		agents = filterAgents(agents, filterState, statusBySession)
	}
	groups := groupAgentsForWeb(agents)
	s.render(w, "agents.html", map[string]any{
		"Agents": agents,
		"Groups": groups,
		"Query":  filterState.Raw,
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
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	filterState := parseFilter(query)
	sessions := state.Sessions
	var statusBySession map[string]tmux.Status
	if filterState.Raw != "" {
		if len(filterState.Statuses) > 0 && s.statusClient != nil {
			statusBySession = make(map[string]tmux.Status, len(state.Sessions))
			for _, session := range state.Sessions {
				statusBySession[session.Name] = s.statusClient.DetectStatus(session.Name)
			}
		}
		sessions = filterSessions(sessions, filterState, statusBySession)
	}
	groups := groupSessionsForWeb(sessions)
	s.render(w, "sessions.html", map[string]any{
		"Sessions": sessions,
		"Groups":   groups,
		"Query":    filterState.Raw,
	})
}

type sessionActionPayload struct {
	Name        string `json:"name"`
	ProjectPath string `json:"project_path"`
	AgentType   string `json:"agent_type"`
}

type renamePayload struct {
	Name string `json:"name"`
}

func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload sessionActionPayload
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		payload.Name = r.FormValue("name")
		payload.ProjectPath = r.FormValue("project_path")
		payload.AgentType = r.FormValue("agent_type")
	}
	if payload.Name == "" || payload.ProjectPath == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	if payload.AgentType == "" {
		payload.AgentType = "claude"
	}
	if err := s.agg.NewSession(payload.Name, payload.ProjectPath, payload.AgentType); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	action := parts[1]

	switch action {
	case "restart":
		session, ok := findSession(s.agg.GetState(), name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if session.AgentType == "" {
			http.Error(w, "unknown agent type", http.StatusBadRequest)
			return
		}
		if err := s.agg.RestartSession(name, session.ProjectPath, session.AgentType); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	case "rename":
		var payload renamePayload
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			payload.Name = r.FormValue("name")
		}
		if payload.Name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		if err := s.agg.RenameSession(name, payload.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	case "fork":
		var payload renamePayload
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		} else {
			_ = r.ParseForm()
			payload.Name = r.FormValue("name")
		}
		newName := payload.Name
		if newName == "" {
			newName = name + "-fork"
		}
		session, ok := findSession(s.agg.GetState(), name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if session.AgentType == "" {
			http.Error(w, "unknown agent type", http.StatusBadRequest)
			return
		}
		if err := s.agg.ForkSession(newName, session.ProjectPath, session.AgentType); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	default:
		http.NotFound(w, r)
		return
	}
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

func (s *Server) handleProjectMCPAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/projects")
	if !strings.Contains(path, "/mcp/") {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(path, "/mcp/", 2)
	projectPath := strings.TrimLeft(parts[0], "/")
	projectPath = "/" + projectPath
	rest := parts[1]
	segments := strings.Split(rest, "/")
	if len(segments) < 2 {
		http.NotFound(w, r)
		return
	}
	component := segments[0]
	action := segments[1]

	switch action {
	case "start":
		if err := s.agg.StartMCP(r.Context(), projectPath, component); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	case "stop":
		if err := s.agg.StopMCP(projectPath, component); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func findSession(state aggregator.State, name string) (aggregator.TmuxSession, bool) {
	for _, s := range state.Sessions {
		if s.Name == name {
			return s, true
		}
	}
	return aggregator.TmuxSession{}, false
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
		projectPath = "/" + projectPath                  // Add back single leading /
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

func groupSessionsForWeb(sessions []aggregator.TmuxSession) []webGroup[aggregator.TmuxSession] {
	grouped := map[string][]aggregator.TmuxSession{}
	for _, session := range sessions {
		grouped[session.ProjectPath] = append(grouped[session.ProjectPath], session)
	}
	return finalizeWebGroups(grouped, func(path string, items []aggregator.TmuxSession) webGroup[aggregator.TmuxSession] {
		return webGroup[aggregator.TmuxSession]{Name: groupName(path), Path: path, Items: items}
	})
}

func groupAgentsForWeb(agents []aggregator.Agent) []webGroup[aggregator.Agent] {
	grouped := map[string][]aggregator.Agent{}
	for _, agent := range agents {
		grouped[agent.ProjectPath] = append(grouped[agent.ProjectPath], agent)
	}
	return finalizeWebGroups(grouped, func(path string, items []aggregator.Agent) webGroup[aggregator.Agent] {
		return webGroup[aggregator.Agent]{Name: groupName(path), Path: path, Items: items}
	})
}

func finalizeWebGroups[T any](grouped map[string][]T, build func(path string, items []T) webGroup[T]) []webGroup[T] {
	keys := make([]string, 0, len(grouped))
	for key, items := range grouped {
		if len(items) == 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai := groupName(keys[i])
		aj := groupName(keys[j])
		if ai == "Unassigned" && aj != "Unassigned" {
			return false
		}
		if aj == "Unassigned" && ai != "Unassigned" {
			return true
		}
		return ai < aj
	})
	out := make([]webGroup[T], 0, len(keys))
	for _, key := range keys {
		out = append(out, build(key, grouped[key]))
	}
	return out
}

func groupName(path string) string {
	if path == "" {
		return "Unassigned"
	}
	return filepath.Base(path)
}
