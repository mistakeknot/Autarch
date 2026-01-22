package aggregator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/agentcmd"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/agentmail"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/discovery"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/mcp"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tandemonium"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

// Agent represents a detected AI agent
type Agent struct {
	Name        string    `json:"name"`
	Program     string    `json:"program"`
	Model       string    `json:"model"`
	ProjectPath string    `json:"project_path"`
	TaskID      string    `json:"task_id,omitempty"`
	SessionName string    `json:"session_name,omitempty"`
	LastActive  time.Time `json:"last_active"`
	InboxCount  int       `json:"inbox_count"`
	UnreadCount int       `json:"unread_count"`
}

// TmuxSession represents an active tmux session
type TmuxSession struct {
	Name         string    `json:"name"`
	Created      time.Time `json:"created"`
	LastActivity time.Time `json:"last_activity"`
	WindowCount  int       `json:"window_count"`
	Attached     bool      `json:"attached"`
	AgentName    string    `json:"agent_name,omitempty"`
	AgentType    string    `json:"agent_type,omitempty"`
	ProjectPath  string    `json:"project_path,omitempty"`
}

// Activity represents a recent event
type Activity struct {
	Time        time.Time `json:"time"`
	Type        string    `json:"type"` // commit, message, reservation, task_update
	AgentName   string    `json:"agent_name,omitempty"`
	ProjectPath string    `json:"project_path"`
	Summary     string    `json:"summary"`
}

// State holds the aggregated view of all projects and agents
type State struct {
	Projects   []discovery.Project `json:"projects"`
	Agents     []Agent             `json:"agents"`
	Sessions   []TmuxSession       `json:"sessions"`
	MCP        map[string][]mcp.ComponentStatus `json:"mcp"`
	Activities []Activity          `json:"activities"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type tmuxAPI interface {
	IsAvailable() bool
	ListSessions() ([]tmux.Session, error)
	DetectStatus(name string) tmux.Status
	NewSession(name, path string, cmd []string) error
	RenameSession(oldName, newName string) error
	KillSession(name string) error
	AttachSession(name string) error
}

// Aggregator combines data from multiple sources
type Aggregator struct {
	scanner         *discovery.Scanner
	tmuxClient      tmuxAPI
	agentMailReader *agentmail.Reader
	mcpManager      *mcp.Manager
	resolver        *agentcmd.Resolver
	cfg             *config.Config
	mu              sync.RWMutex
	state           State
}

// New creates a new aggregator
func New(scanner *discovery.Scanner, cfg *config.Config) *Aggregator {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &Aggregator{
		scanner:         scanner,
		tmuxClient:      tmux.NewClient(),
		agentMailReader: agentmail.NewReader(),
		mcpManager:      mcp.NewManager(),
		resolver:        agentcmd.NewResolver(cfg),
		cfg:             cfg,
		state: State{
			Projects:   []discovery.Project{},
			Agents:     []Agent{},
			Sessions:   []TmuxSession{},
			MCP:        map[string][]mcp.ComponentStatus{},
			Activities: []Activity{},
		},
	}
}

// Refresh rescans all data sources
func (a *Aggregator) Refresh(ctx context.Context) error {
	slog.Debug("refreshing aggregator state")

	// Scan for projects
	projects, err := a.scanner.Scan()
	if err != nil {
		return err
	}

	// Enrich projects with Tandemonium task stats
	a.enrichWithTaskStats(projects)

	// Load agents from MCP Agent Mail
	agents := a.loadAgents()

	// Load tmux sessions
	sessions := a.loadTmuxSessions(projects)

	// Load MCP statuses
	mcpStatuses := a.loadMCPStatuses(projects)

	// TODO: Load recent activities
	activities := []Activity{}

	// Update state
	a.mu.Lock()
	a.state = State{
		Projects:   projects,
		Agents:     agents,
		Sessions:   sessions,
		MCP:        mcpStatuses,
		Activities: activities,
		UpdatedAt:  time.Now(),
	}
	a.mu.Unlock()

	slog.Info("refresh complete", "projects", len(projects), "agents", len(agents), "sessions", len(sessions))
	return nil
}

// enrichWithTaskStats loads Tandemonium task statistics for each project
func (a *Aggregator) enrichWithTaskStats(projects []discovery.Project) {
	for i := range projects {
		if !projects[i].HasTandemonium {
			continue
		}
		reader := tandemonium.NewReader(projects[i].Path)
		stats, err := reader.GetTaskStats()
		if err != nil {
			slog.Warn("failed to read task stats", "project", projects[i].Path, "error", err)
			continue
		}
		projects[i].TaskStats = &discovery.TaskStats{
			Total:      stats.Total,
			Todo:       stats.Todo,
			InProgress: stats.InProgress,
			Review:     stats.Review,
			Done:       stats.Done,
			Blocked:    stats.Blocked,
		}
	}
}

// loadAgents fetches registered agents from MCP Agent Mail
func (a *Aggregator) loadAgents() []Agent {
	if !a.agentMailReader.IsAvailable() {
		slog.Debug("agent mail database not available")
		return []Agent{}
	}

	mailAgents, err := a.agentMailReader.GetAllAgents()
	if err != nil {
		slog.Error("failed to load agents", "error", err)
		return []Agent{}
	}

	agents := make([]Agent, len(mailAgents))
	for i, ma := range mailAgents {
		slog.Debug("loading agent", "name", ma.Name, "lastActiveTS", ma.LastActiveTS)
		agents[i] = Agent{
			Name:        ma.Name,
			Program:     ma.Program,
			Model:       ma.Model,
			ProjectPath: ma.ProjectPath,
			LastActive:  ma.LastActiveTS,
			InboxCount:  ma.InboxCount,
			UnreadCount: ma.UnreadCount,
		}
	}

	return agents
}

// loadTmuxSessions fetches and enriches tmux sessions with agent detection
func (a *Aggregator) loadTmuxSessions(projects []discovery.Project) []TmuxSession {
	if !a.tmuxClient.IsAvailable() {
		slog.Debug("tmux not available")
		return []TmuxSession{}
	}

	rawSessions, err := a.tmuxClient.ListSessions()
	if err != nil {
		slog.Error("failed to list tmux sessions", "error", err)
		return []TmuxSession{}
	}

	// Extract project paths for detector
	projectPaths := make([]string, len(projects))
	for i, p := range projects {
		projectPaths[i] = p.Path
	}

	// Detect agents
	detector := tmux.NewDetector(projectPaths)
	enriched := detector.EnrichSessions(rawSessions)

	// Convert to aggregator type
	sessions := make([]TmuxSession, len(enriched))
	for i, e := range enriched {
		sessions[i] = TmuxSession{
			Name:         e.Name,
			Created:      e.Created,
			LastActivity: e.LastActivity,
			WindowCount:  e.WindowCount,
			Attached:     e.Attached,
			ProjectPath:  e.CurrentPath,
		}
		if e.Agent != nil {
			sessions[i].AgentName = e.Agent.Name
			sessions[i].AgentType = string(e.Agent.Type)
			if e.Agent.ProjectPath != "" {
				sessions[i].ProjectPath = e.Agent.ProjectPath
			}
		}
	}

	return sessions
}

func (a *Aggregator) loadMCPStatuses(projects []discovery.Project) map[string][]mcp.ComponentStatus {
	statuses := make(map[string][]mcp.ComponentStatus)
	for _, p := range projects {
		components := []string{}
		if pathIsDir(filepath.Join(p.Path, "mcp-server")) {
			components = append(components, "server")
		}
		if pathIsDir(filepath.Join(p.Path, "mcp-client")) {
			components = append(components, "client")
		}
		if len(components) == 0 {
			continue
		}

		list := make([]mcp.ComponentStatus, 0, len(components))
		for _, component := range components {
			status := a.mcpManager.Status(p.Path, component)
			if status == nil {
				status = &mcp.ComponentStatus{
					ProjectPath: p.Path,
					Component:   component,
					Status:      mcp.StatusStopped,
				}
			}
			list = append(list, *status)
		}
		statuses[p.Path] = list
	}
	return statuses
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetState returns the current aggregated state
func (a *Aggregator) GetState() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// GetProject returns a specific project by path
func (a *Aggregator) GetProject(path string) *discovery.Project {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, p := range a.state.Projects {
		if p.Path == path {
			return &p
		}
	}
	return nil
}

// GetAgent returns a specific agent by name
func (a *Aggregator) GetAgent(name string) *Agent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, ag := range a.state.Agents {
		if ag.Name == name {
			return &ag
		}
	}
	return nil
}

// GetProjectTasks returns tasks for a specific project, grouped by status
func (a *Aggregator) GetProjectTasks(projectPath string) (map[string][]tandemonium.Task, error) {
	reader := tandemonium.NewReader(projectPath)
	if !reader.Exists() {
		return nil, nil
	}
	return reader.GetTasksByStatus()
}

// GetProjectTaskList returns all tasks for a project
func (a *Aggregator) GetProjectTaskList(projectPath string) ([]tandemonium.Task, error) {
	reader := tandemonium.NewReader(projectPath)
	if !reader.Exists() {
		return nil, nil
	}
	return reader.ReadTasks()
}

// GetAgentMailAgent returns detailed agent info from MCP Agent Mail
func (a *Aggregator) GetAgentMailAgent(name string) (*agentmail.Agent, error) {
	return a.agentMailReader.GetAgent(name)
}

// GetAgentMessages returns recent messages for an agent
func (a *Aggregator) GetAgentMessages(agentID int, limit int) ([]agentmail.Message, error) {
	return a.agentMailReader.GetAgentMessages(agentID, limit)
}

// GetAgentReservations returns file reservations for an agent
func (a *Aggregator) GetAgentReservations(agentID int) ([]agentmail.FileReservation, error) {
	return a.agentMailReader.GetAgentReservations(agentID)
}

// GetActiveReservations returns all active file reservations
func (a *Aggregator) GetActiveReservations() ([]agentmail.FileReservation, error) {
	return a.agentMailReader.GetActiveReservations()
}

// NewSession creates a new tmux session for an agent.
func (a *Aggregator) NewSession(name, projectPath, agentType string) error {
	cmd, args := a.resolver.Resolve(agentType, projectPath)
	if cmd == "" {
		return fmt.Errorf("unknown agent type: %s", agentType)
	}
	full := append([]string{cmd}, args...)
	return a.tmuxClient.NewSession(name, projectPath, full)
}

// RestartSession kills and recreates a tmux session for an agent.
func (a *Aggregator) RestartSession(name, projectPath, agentType string) error {
	cmd, args := a.resolver.Resolve(agentType, projectPath)
	if cmd == "" {
		return fmt.Errorf("unknown agent type: %s", agentType)
	}
	full := append([]string{cmd}, args...)
	if err := a.tmuxClient.KillSession(name); err != nil {
		return err
	}
	return a.tmuxClient.NewSession(name, projectPath, full)
}

// ForkSession creates a new session in the same project.
func (a *Aggregator) ForkSession(name, projectPath, agentType string) error {
	return a.NewSession(name, projectPath, agentType)
}

// RenameSession renames an existing tmux session.
func (a *Aggregator) RenameSession(oldName, newName string) error {
	return a.tmuxClient.RenameSession(oldName, newName)
}

// AttachSession attaches to a tmux session (TUI use).
func (a *Aggregator) AttachSession(name string) error {
	return a.tmuxClient.AttachSession(name)
}

// StartMCP starts a repo MCP component.
func (a *Aggregator) StartMCP(ctx context.Context, projectPath, component string) error {
	cmd, workdir, err := a.resolveMCPCommand(projectPath, component)
	if err != nil {
		return err
	}
	return a.mcpManager.Start(ctx, projectPath, component, cmd, workdir)
}

// StopMCP stops a repo MCP component.
func (a *Aggregator) StopMCP(projectPath, component string) error {
	return a.mcpManager.Stop(projectPath, component)
}

func (a *Aggregator) resolveMCPCommand(projectPath, component string) ([]string, string, error) {
	if a.cfg != nil {
		var cfg config.MCPComponentConfig
		switch component {
		case "server":
			cfg = a.cfg.MCP.Server
		case "client":
			cfg = a.cfg.MCP.Client
		default:
			return nil, "", fmt.Errorf("unknown component: %s", component)
		}
		if cfg.Command != "" {
			cmd := append([]string{cfg.Command}, cfg.Args...)
			workdir := cfg.Workdir
			if workdir == "" {
				workdir = projectPath
			}
			return cmd, workdir, nil
		}
	}

	var dir string
	switch component {
	case "server":
		dir = filepath.Join(projectPath, "mcp-server")
	case "client":
		dir = filepath.Join(projectPath, "mcp-client")
	default:
		return nil, "", fmt.Errorf("unknown component: %s", component)
	}
	if !pathIsDir(dir) {
		return nil, "", fmt.Errorf("mcp %s directory not found", component)
	}
	return []string{"npm", "run", "dev"}, dir, nil
}
