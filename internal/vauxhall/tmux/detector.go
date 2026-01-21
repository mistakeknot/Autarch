package tmux

import (
	"path/filepath"
	"strings"
)

// AgentType represents the type of AI agent
type AgentType string

const (
	AgentClaude AgentType = "claude"
	AgentCodex  AgentType = "codex"
	AgentAider  AgentType = "aider"
	AgentCursor AgentType = "cursor"
)

// AgentInfo contains detected agent information
type AgentInfo struct {
	Type        AgentType `json:"type"`
	Name        string    `json:"name"`
	ProjectPath string    `json:"project_path,omitempty"`
}

// Detector identifies AI agent sessions
type Detector struct {
	projectPaths []string // known project paths for matching
}

// NewDetector creates a new agent detector
func NewDetector(projectPaths []string) *Detector {
	return &Detector{
		projectPaths: projectPaths,
	}
}

// DetectAgent attempts to identify if a session contains an AI agent
func (d *Detector) DetectAgent(session Session) *AgentInfo {
	// Check session name patterns
	if info := d.detectByName(session.Name); info != nil {
		info.ProjectPath = d.matchProject(session.CurrentPath)
		return info
	}

	// Check if session is in a known project directory
	if projectPath := d.matchProject(session.CurrentPath); projectPath != "" {
		// Session is in a project, but we don't know what agent type
		// Could be a manual session or an agent we didn't detect by name
		return nil
	}

	return nil
}

// detectByName checks session name for agent patterns
func (d *Detector) detectByName(name string) *AgentInfo {
	lower := strings.ToLower(name)

	// Claude Code patterns
	// Common: "claude", "claude-shadow-work", "cc-project"
	if strings.Contains(lower, "claude") || strings.HasPrefix(lower, "cc-") {
		return &AgentInfo{
			Type: AgentClaude,
			Name: formatAgentName("Claude", name),
		}
	}

	// Codex patterns
	// Common: "codex", "codex-project", "cx-project"
	if strings.Contains(lower, "codex") || strings.HasPrefix(lower, "cx-") {
		return &AgentInfo{
			Type: AgentCodex,
			Name: formatAgentName("Codex", name),
		}
	}

	// Aider patterns
	if strings.Contains(lower, "aider") {
		return &AgentInfo{
			Type: AgentAider,
			Name: formatAgentName("Aider", name),
		}
	}

	// Cursor patterns
	if strings.Contains(lower, "cursor") {
		return &AgentInfo{
			Type: AgentCursor,
			Name: formatAgentName("Cursor", name),
		}
	}

	return nil
}

// matchProject finds if a path is within a known project
func (d *Detector) matchProject(path string) string {
	if path == "" {
		return ""
	}

	// Normalize path
	path = filepath.Clean(path)

	for _, projectPath := range d.projectPaths {
		projectPath = filepath.Clean(projectPath)
		if path == projectPath || strings.HasPrefix(path, projectPath+string(filepath.Separator)) {
			return projectPath
		}
	}

	return ""
}

// formatAgentName creates a display name for the agent
func formatAgentName(agentType, sessionName string) string {
	// Extract project name from session if present
	// e.g., "claude-shadow-work" -> "Claude (shadow-work)"
	lower := strings.ToLower(sessionName)

	switch agentType {
	case "Claude":
		if strings.HasPrefix(lower, "claude-") {
			project := sessionName[7:]
			return "Claude (" + project + ")"
		}
		if strings.HasPrefix(lower, "cc-") {
			project := sessionName[3:]
			return "Claude (" + project + ")"
		}
	case "Codex":
		if strings.HasPrefix(lower, "codex-") {
			project := sessionName[6:]
			return "Codex (" + project + ")"
		}
		if strings.HasPrefix(lower, "cx-") {
			project := sessionName[3:]
			return "Codex (" + project + ")"
		}
	case "Aider":
		if strings.HasPrefix(lower, "aider-") {
			project := sessionName[6:]
			return "Aider (" + project + ")"
		}
	case "Cursor":
		if strings.HasPrefix(lower, "cursor-") {
			project := sessionName[7:]
			return "Cursor (" + project + ")"
		}
	}

	return agentType
}

// EnrichSessions adds agent detection to a list of sessions
func (d *Detector) EnrichSessions(sessions []Session) []EnrichedSession {
	result := make([]EnrichedSession, len(sessions))
	for i, s := range sessions {
		result[i] = EnrichedSession{
			Session: s,
			Agent:   d.DetectAgent(s),
		}
	}
	return result
}

// EnrichedSession combines session info with detected agent info
type EnrichedSession struct {
	Session
	Agent *AgentInfo `json:"agent,omitempty"`
}
