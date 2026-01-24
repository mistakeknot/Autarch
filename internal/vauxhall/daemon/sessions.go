package daemon

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionManager manages tmux sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// List returns all sessions
func (m *SessionManager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// Get returns a session by ID
func (m *SessionManager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Count returns the number of sessions
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Spawn creates and starts a new session
func (m *SessionManager) Spawn(name, projectPath, agentType string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate name
	for _, s := range m.sessions {
		if s.Name == name {
			return nil, fmt.Errorf("session with name %q already exists", name)
		}
	}

	id := uuid.New().String()[:8]

	// Create tmux session
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", projectPath)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create tmux session: %w", err)
	}

	// If agent type specified, start the agent
	if agentType != "" {
		agentCmd := agentCommand(agentType)
		if agentCmd != "" {
			sendCmd := exec.Command("tmux", "send-keys", "-t", name, agentCmd, "Enter")
			_ = sendCmd.Run()
		}
	}

	session := &Session{
		ID:          id,
		Name:        name,
		ProjectPath: projectPath,
		AgentType:   agentType,
		Status:      "running",
		CreatedAt:   time.Now(),
	}
	m.sessions[id] = session

	return session, nil
}

// Dispose stops and removes a session
func (m *SessionManager) Dispose(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}

	// Kill tmux session
	cmd := exec.Command("tmux", "kill-session", "-t", session.Name)
	_ = cmd.Run() // Ignore errors if session already gone

	delete(m.sessions, id)
	return nil
}

// Restart restarts a session
func (m *SessionManager) Restart(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}

	// Kill and recreate
	killCmd := exec.Command("tmux", "kill-session", "-t", session.Name)
	_ = killCmd.Run()

	createCmd := exec.Command("tmux", "new-session", "-d", "-s", session.Name, "-c", session.ProjectPath)
	if err := createCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to restart session: %w", err)
	}

	// Restart agent if configured
	if session.AgentType != "" {
		agentCmd := agentCommand(session.AgentType)
		if agentCmd != "" {
			sendCmd := exec.Command("tmux", "send-keys", "-t", session.Name, agentCmd, "Enter")
			_ = sendCmd.Run()
		}
	}

	session.Status = "running"
	session.CreatedAt = time.Now()
	return session, nil
}

// Attach attaches to a session (foreground)
func (m *SessionManager) Attach(id string) error {
	m.mu.RLock()
	session, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %q not found", id)
	}

	// Note: This would need to be handled specially for daemon mode
	// since we can't attach from HTTP context
	cmd := exec.Command("tmux", "attach-session", "-t", session.Name)
	return cmd.Start()
}

// DiscoverExisting finds existing tmux sessions and adds them to the manager
func (m *SessionManager) DiscoverExisting() error {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_path}")
	output, err := cmd.Output()
	if err != nil {
		return nil // No sessions or tmux not running
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Parse output and add sessions
	// Format: name:path
	_ = output // TODO: Parse and add existing sessions

	return nil
}

// agentCommand returns the command to start an agent type
func agentCommand(agentType string) string {
	switch agentType {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "aider":
		return "aider"
	case "cursor":
		return "cursor ."
	default:
		return ""
	}
}
