package aggregator

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/agentcmd"
	"github.com/mistakeknot/autarch/internal/bigend/coldwine"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	"github.com/mistakeknot/autarch/internal/bigend/mcp"
	"github.com/mistakeknot/autarch/internal/bigend/statedetect"
	"github.com/mistakeknot/autarch/internal/bigend/tmux"
	gurgSpecs "github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/intermute"
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

	// State detection fields (NudgeNik-style)
	State           string    `json:"state"`            // working, waiting, blocked, stalled, done, error
	StateConfidence float64   `json:"state_confidence"` // 0.0-1.0 detection certainty
	StateSource     string    `json:"state_source"`     // pattern, repetition, activity, llm
	StateAt         time.Time `json:"state_at"`         // when state was last detected
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
	CapturePane(sessionName string, lines int) (string, error)
	NewSession(name, path string, cmd []string) error
	RenameSession(oldName, newName string) error
	KillSession(name string) error
	AttachSession(name string) error
}

// EventHandler processes aggregator events (spec changes, agent updates, etc.)
type EventHandler func(Event)

// Event represents a domain event from Intermute or local detection
type Event struct {
	Type      string      `json:"type"`       // spec.created, task.updated, message.sent, etc.
	Project   string      `json:"project"`    // Project context
	EntityID  string      `json:"entity_id"`  // ID of affected entity
	Data      interface{} `json:"data"`       // Event-specific data
	Timestamp time.Time   `json:"timestamp"`
}

// Aggregator combines data from multiple sources
type Aggregator struct {
	scanner         *discovery.Scanner
	tmuxClient      tmuxAPI
	stateDetector   *statedetect.Detector
	intermuteClient *intermute.Client
	mcpManager      *mcp.Manager
	resolver        *agentcmd.Resolver
	cfg             *config.Config
	mu              sync.RWMutex
	state           State

	// WebSocket event handling
	handlers   map[string][]EventHandler
	handlersMu sync.RWMutex
	wsCtx      context.Context
	wsCancel   context.CancelFunc
	wsConnected bool
}

// New creates a new aggregator
func New(scanner *discovery.Scanner, cfg *config.Config) *Aggregator {
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Initialize Intermute client (optional - may not be available)
	var ic *intermute.Client
	icClient, err := intermute.NewClient(nil) // Uses environment variables
	if err != nil {
		slog.Debug("intermute client unavailable", "error", err)
	} else {
		ic = icClient
	}

	return &Aggregator{
		scanner:         scanner,
		tmuxClient:      tmux.NewClient(),
		stateDetector:   statedetect.NewDetector(),
		intermuteClient: ic,
		mcpManager:      mcp.NewManager(),
		resolver:        agentcmd.NewResolver(cfg),
		cfg:             cfg,
		handlers:        make(map[string][]EventHandler),
		state: State{
			Projects:   []discovery.Project{},
			Agents:     []Agent{},
			Sessions:   []TmuxSession{},
			MCP:        map[string][]mcp.ComponentStatus{},
			Activities: []Activity{},
		},
	}
}

// ConnectWebSocket establishes a WebSocket connection to Intermute for real-time events.
// This enables reactive updates instead of polling.
func (a *Aggregator) ConnectWebSocket(ctx context.Context) error {
	if a.intermuteClient == nil {
		return fmt.Errorf("intermute client not available")
	}

	// Create cancellable context for the WebSocket connection
	a.wsCtx, a.wsCancel = context.WithCancel(ctx)

	// Connect to Intermute WebSocket
	if err := a.intermuteClient.Connect(a.wsCtx); err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}

	// Register event handler
	a.intermuteClient.On("*", func(evt intermute.Event) {
		a.handleIntermuteEvent(evt)
	})

	// Subscribe to all relevant event types
	eventTypes := []string{
		"spec.created", "spec.updated", "spec.deleted",
		"epic.created", "epic.updated", "epic.deleted",
		"story.created", "story.updated", "story.deleted",
		"task.created", "task.updated", "task.deleted", "task.assigned",
		"insight.created", "insight.updated",
		"cuj.created", "cuj.updated", "cuj.deleted",
		"agent.registered", "agent.updated",
		"message.sent", "message.read",
		"reservation.created", "reservation.released",
	}

	if err := a.intermuteClient.Subscribe(a.wsCtx, eventTypes...); err != nil {
		return fmt.Errorf("websocket subscribe: %w", err)
	}

	a.wsConnected = true
	slog.Info("connected to Intermute WebSocket", "events", len(eventTypes))
	return nil
}

// DisconnectWebSocket closes the WebSocket connection
func (a *Aggregator) DisconnectWebSocket() error {
	if a.wsCancel != nil {
		a.wsCancel()
	}
	a.wsConnected = false
	if a.intermuteClient != nil {
		return a.intermuteClient.Close()
	}
	return nil
}

// IsWebSocketConnected returns whether the WebSocket connection is active
func (a *Aggregator) IsWebSocketConnected() bool {
	return a.wsConnected
}

// On registers an event handler for specific event types.
// Pass "*" to receive all events.
func (a *Aggregator) On(eventType string, handler EventHandler) {
	a.handlersMu.Lock()
	defer a.handlersMu.Unlock()
	a.handlers[eventType] = append(a.handlers[eventType], handler)
}

// handleIntermuteEvent processes events from Intermute and triggers appropriate updates
func (a *Aggregator) handleIntermuteEvent(evt intermute.Event) {
	// Convert to aggregator event
	aggEvt := Event{
		Type:      evt.Type,
		Project:   evt.Project,
		EntityID:  evt.EntityID,
		Data:      evt.Data,
		Timestamp: evt.Timestamp,
	}

	// Add to activities feed
	a.addActivity(aggEvt)

	// Trigger targeted refresh based on event type
	a.refreshForEvent(evt.Type)

	// Dispatch to registered handlers
	a.dispatchEvent(aggEvt)
}

// addActivity adds an event to the activities feed
func (a *Aggregator) addActivity(evt Event) {
	activity := Activity{
		Time:        evt.Timestamp,
		Type:        evt.Type,
		ProjectPath: evt.Project,
		Summary:     summarizeEvent(evt),
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Prepend new activity (most recent first)
	a.state.Activities = append([]Activity{activity}, a.state.Activities...)

	// Keep only last 100 activities
	if len(a.state.Activities) > 100 {
		a.state.Activities = a.state.Activities[:100]
	}

	a.state.UpdatedAt = time.Now()
}

// refreshForEvent triggers targeted refresh based on event type
func (a *Aggregator) refreshForEvent(eventType string) {
	ctx := context.Background()

	switch {
	case strings.HasPrefix(eventType, "spec.") ||
		strings.HasPrefix(eventType, "epic.") ||
		strings.HasPrefix(eventType, "story.") ||
		strings.HasPrefix(eventType, "task."):
		// Spec/task events - refresh Gurgeh stats
		go func() {
			a.mu.Lock()
			a.enrichWithGurgStats(a.state.Projects)
			a.state.UpdatedAt = time.Now()
			a.mu.Unlock()
		}()

	case strings.HasPrefix(eventType, "agent.") ||
		strings.HasPrefix(eventType, "message."):
		// Agent events - refresh agent list
		go func() {
			agents := a.loadAgents()
			a.mu.Lock()
			a.state.Agents = agents
			a.state.UpdatedAt = time.Now()
			a.mu.Unlock()
		}()

	case strings.HasPrefix(eventType, "insight."):
		// Insight events - refresh Pollard stats
		go func() {
			a.mu.Lock()
			a.enrichWithPollardStats(a.state.Projects)
			a.state.UpdatedAt = time.Now()
			a.mu.Unlock()
		}()

	case strings.HasPrefix(eventType, "reservation."):
		// Reservation events - no specific refresh needed, just activity logged
		slog.Debug("reservation event", "type", eventType)
	}

	// Full refresh can be requested externally
	_ = ctx // silence unused variable if needed
}

// dispatchEvent dispatches an event to all registered handlers
func (a *Aggregator) dispatchEvent(evt Event) {
	a.handlersMu.RLock()
	// Get handlers for this specific event type
	handlers := make([]EventHandler, 0)
	handlers = append(handlers, a.handlers[evt.Type]...)
	// Get handlers for wildcard
	handlers = append(handlers, a.handlers["*"]...)
	a.handlersMu.RUnlock()

	for _, h := range handlers {
		h(evt)
	}
}

// summarizeEvent creates a human-readable summary of an event
func summarizeEvent(evt Event) string {
	parts := strings.Split(evt.Type, ".")
	if len(parts) != 2 {
		return evt.Type
	}

	entity := parts[0]
	action := parts[1]

	switch action {
	case "created":
		return fmt.Sprintf("New %s created: %s", entity, evt.EntityID)
	case "updated":
		return fmt.Sprintf("%s updated: %s", strings.Title(entity), evt.EntityID)
	case "deleted":
		return fmt.Sprintf("%s deleted: %s", strings.Title(entity), evt.EntityID)
	case "assigned":
		return fmt.Sprintf("%s assigned: %s", strings.Title(entity), evt.EntityID)
	case "sent":
		return fmt.Sprintf("Message sent: %s", evt.EntityID)
	case "read":
		return fmt.Sprintf("Message read: %s", evt.EntityID)
	case "registered":
		return fmt.Sprintf("Agent registered: %s", evt.EntityID)
	case "released":
		return fmt.Sprintf("Reservation released: %s", evt.EntityID)
	default:
		return fmt.Sprintf("%s %s: %s", strings.Title(entity), action, evt.EntityID)
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

	// Enrich projects with Praude stats
	a.enrichWithGurgStats(projects)

	// Enrich projects with Pollard stats
	a.enrichWithPollardStats(projects)

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

// enrichWithTaskStats loads Coldwine task statistics for each project
func (a *Aggregator) enrichWithTaskStats(projects []discovery.Project) {
	for i := range projects {
		if !projects[i].HasColdwine {
			continue
		}
		reader := coldwine.NewReader(projects[i].Path)
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

// enrichWithGurgStats loads Gurgeh PRD statistics for each project
func (a *Aggregator) enrichWithGurgStats(projects []discovery.Project) {
	for i := range projects {
		if !projects[i].HasGurgeh {
			continue
		}
		// Check .gurgeh first, then .praude for legacy
		gurgDir := filepath.Join(projects[i].Path, ".gurgeh", "specs")
		if _, err := os.Stat(gurgDir); os.IsNotExist(err) {
			gurgDir = filepath.Join(projects[i].Path, ".praude", "specs")
		}
		summaries, _ := gurgSpecs.LoadSummaries(gurgDir)

		stats := &discovery.GurgStats{}
		for _, s := range summaries {
			stats.Total++
			switch strings.ToLower(s.Status) {
			case "draft":
				stats.Draft++
			case "active", "in_progress", "approved":
				stats.Active++
			case "done", "complete":
				stats.Done++
			default:
				stats.Draft++ // Default unknown status to draft
			}
		}
		projects[i].GurgStats = stats
	}
}

// enrichWithPollardStats loads Pollard research statistics for each project
func (a *Aggregator) enrichWithPollardStats(projects []discovery.Project) {
	for i := range projects {
		if !projects[i].HasPollard {
			continue
		}
		pollardPath := filepath.Join(projects[i].Path, ".pollard")

		// Count sources
		sourcesDir := filepath.Join(pollardPath, "sources")
		sourceCount := countYAMLFiles(sourcesDir)

		// Count insights
		insightsDir := filepath.Join(pollardPath, "insights")
		insightCount := countYAMLFiles(insightsDir)

		// Count reports and find latest
		reportsDir := filepath.Join(pollardPath, "reports")
		reportCount, lastReport := countReportsAndFindLatest(reportsDir)

		projects[i].PollardStats = &discovery.PollardStats{
			Sources:    sourceCount,
			Insights:   insightCount,
			Reports:    reportCount,
			LastReport: lastReport,
		}
	}
}

// countYAMLFiles counts YAML files in a directory
func countYAMLFiles(dir string) int {
	count := 0
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml")) {
			count++
		}
		return nil
	})
	return count
}

// countReportsAndFindLatest counts report files and finds the most recent
func countReportsAndFindLatest(dir string) (int, string) {
	count := 0
	var latestPath string
	var latestTime time.Time

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			count++
			info, err := d.Info()
			if err == nil && info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestPath = path
			}
		}
		return nil
	})

	// Return just the filename for the latest report
	if latestPath != "" {
		return count, filepath.Base(latestPath)
	}
	return count, ""
}

// loadAgents fetches registered agents from Intermute
func (a *Aggregator) loadAgents() []Agent {
	if a.intermuteClient == nil {
		slog.Debug("intermute client not available")
		return []Agent{}
	}

	ctx := context.Background()
	intermuteAgents, err := a.intermuteClient.ListAgentsEnriched(ctx)
	if err != nil {
		slog.Error("failed to load agents from intermute", "error", err)
		return []Agent{}
	}

	agents := make([]Agent, len(intermuteAgents))
	for i, ia := range intermuteAgents {
		slog.Debug("loading agent", "name", ia.Name, "lastSeen", ia.LastSeen)
		// Extract program and model from metadata if available
		program := ia.Metadata["program"]
		model := ia.Metadata["model"]
		projectPath := ia.Metadata["project_path"]

		agents[i] = Agent{
			Name:        ia.Name,
			Program:     program,
			Model:       model,
			ProjectPath: projectPath,
			LastActive:  ia.LastSeen,
			InboxCount:  ia.InboxCount,
			UnreadCount: ia.UnreadCount,
		}
	}

	return agents
}

// loadTmuxSessions fetches and enriches tmux sessions with agent detection and state
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

	// Convert to aggregator type with state detection
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

		// Detect agent state (NudgeNik-style)
		a.detectSessionState(&sessions[i])
	}

	return sessions
}

// detectSessionState uses the statedetect package to determine agent state.
func (a *Aggregator) detectSessionState(session *TmuxSession) {
	// Only detect state for agent sessions
	if session.AgentType == "" {
		session.State = string(statedetect.StateUnknown)
		session.StateConfidence = 0.0
		session.StateSource = string(statedetect.SourceDefault)
		return
	}

	// Capture recent pane output
	output, err := a.tmuxClient.CapturePane(session.Name, 50)
	if err != nil {
		slog.Debug("failed to capture pane for state detection",
			"session", session.Name, "error", err)
		session.State = string(statedetect.StateUnknown)
		session.StateConfidence = 0.0
		session.StateSource = string(statedetect.SourceDefault)
		return
	}

	// Run detection
	result := a.stateDetector.Detect(
		session.Name,
		output,
		session.AgentType,
		session.LastActivity,
	)

	session.State = string(result.State)
	session.StateConfidence = result.Confidence
	session.StateSource = string(result.Source)
	session.StateAt = result.DetectedAt
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
func (a *Aggregator) GetProjectTasks(projectPath string) (map[string][]coldwine.Task, error) {
	reader := coldwine.NewReader(projectPath)
	if !reader.Exists() {
		return nil, nil
	}
	return reader.GetTasksByStatus()
}

// GetProjectTaskList returns all tasks for a project
func (a *Aggregator) GetProjectTaskList(projectPath string) ([]coldwine.Task, error) {
	reader := coldwine.NewReader(projectPath)
	if !reader.Exists() {
		return nil, nil
	}
	return reader.ReadTasks()
}

// GetIntermuteAgent returns detailed agent info from Intermute
func (a *Aggregator) GetIntermuteAgent(name string) (*intermute.Agent, error) {
	if a.intermuteClient == nil {
		return nil, fmt.Errorf("intermute client not available")
	}
	return a.intermuteClient.GetAgent(context.Background(), name)
}

// GetAgentMessages returns recent messages for an agent
func (a *Aggregator) GetAgentMessages(agentID string, limit int) ([]intermute.Message, error) {
	if a.intermuteClient == nil {
		return nil, fmt.Errorf("intermute client not available")
	}
	return a.intermuteClient.AgentMessages(context.Background(), agentID, limit)
}

// GetAgentReservations returns file reservations for an agent
func (a *Aggregator) GetAgentReservations(agentID string) ([]intermute.Reservation, error) {
	if a.intermuteClient == nil {
		return nil, fmt.Errorf("intermute client not available")
	}
	return a.intermuteClient.AgentReservations(context.Background(), agentID)
}

// GetActiveReservations returns all active file reservations
func (a *Aggregator) GetActiveReservations() ([]intermute.Reservation, error) {
	if a.intermuteClient == nil {
		return nil, fmt.Errorf("intermute client not available")
	}
	return a.intermuteClient.ActiveReservations(context.Background())
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
