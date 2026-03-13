// Package spawn handles OS-level agent session creation (tmux sessions).
package spawn

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// ClaudeCodeSpawner creates tmux sessions for Claude Code agents.
type ClaudeCodeSpawner struct {
	terminal string // terminal app identifier (e.g., "iterm")
	project  string // project name for session naming
}

// NewClaudeCodeSpawner creates a spawner.
func NewClaudeCodeSpawner(terminal, project string) *ClaudeCodeSpawner {
	if terminal == "" {
		terminal = "iterm"
	}
	if project == "" {
		project = "Demarch"
	}
	return &ClaudeCodeSpawner{terminal: terminal, project: project}
}

// SessionName generates the tmux session name for an agent.
// Convention: {terminal}-{project}-{agent}-{instance}
func (s *ClaudeCodeSpawner) SessionName(agentName string, instance int) string {
	name := fmt.Sprintf("%s-%s-%s", s.terminal, s.project, agentName)
	if instance > 0 {
		name = fmt.Sprintf("%s-%02d", name, instance)
	}
	return name
}

// Spawn creates a new tmux session for the given agent and bead.
func (s *ClaudeCodeSpawner) Spawn(agentName string, bead mycroft.BeadView, contextFile string) (string, error) {
	sessionName := s.SessionName(agentName, 1)

	// Check if session already exists.
	if sessionExists(sessionName) {
		return "", fmt.Errorf("session %q already exists", sessionName)
	}

	// Create tmux session.
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux new-session %q: %w", sessionName, err)
	}

	return sessionName, nil
}

// Kill terminates a tmux session.
func (s *ClaudeCodeSpawner) Kill(sessionID string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-session %q: %w", sessionID, err)
	}
	return nil
}

func sessionExists(name string) bool {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}
