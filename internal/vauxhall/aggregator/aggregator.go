package aggregator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/agentmail"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/discovery"
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
	Activities []Activity          `json:"activities"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

// Aggregator combines data from multiple sources
type Aggregator struct {
	scanner         *discovery.Scanner
	tmuxClient      *tmux.Client
	agentMailReader *agentmail.Reader
	mu              sync.RWMutex
	state           State
}

// New creates a new aggregator
func New(scanner *discovery.Scanner) *Aggregator {
	return &Aggregator{
		scanner:         scanner,
		tmuxClient:      tmux.NewClient(),
		agentMailReader: agentmail.NewReader(),
		state: State{
			Projects:   []discovery.Project{},
			Agents:     []Agent{},
			Sessions:   []TmuxSession{},
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

	// TODO: Load recent activities
	activities := []Activity{}

	// Update state
	a.mu.Lock()
	a.state = State{
		Projects:   projects,
		Agents:     agents,
		Sessions:   sessions,
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
