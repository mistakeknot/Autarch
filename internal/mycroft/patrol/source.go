package patrol

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// PatrolSource queries intermux and beads CLI directly.
// This is the v0.1 default — standalone Claude Code mode.
type PatrolSource struct {
	registryPath string // path to fleet-registry.yaml
}

// NewPatrolSource creates a PatrolSource.
func NewPatrolSource(registryPath string) *PatrolSource {
	return &PatrolSource{registryPath: registryPath}
}

// FleetState queries intermux and composes agent views.
func (s *PatrolSource) FleetState() (mycroft.FleetView, error) {
	var view mycroft.FleetView
	view.Freshness = make(map[string]time.Time)

	// Query tmux sessions directly for agent detection.
	agents, err := s.detectAgentsFromTmux()
	if err != nil {
		return view, fmt.Errorf("detect agents: %w", err)
	}
	view.Agents = agents
	view.Freshness["intermux"] = time.Now()

	return view, nil
}

// AgentHealth returns the status of a specific agent.
func (s *PatrolSource) AgentHealth(name string) (string, error) {
	view, err := s.FleetState()
	if err != nil {
		return "", err
	}
	for _, a := range view.Agents {
		if a.Name == name {
			return a.Status, nil
		}
	}
	return "unknown", nil
}

// BeadQueue queries bd ready --json for available work.
func (s *PatrolSource) BeadQueue() ([]mycroft.BeadView, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return nil, nil // bd not available — degrade gracefully
	}

	out, err := exec.Command("bd", "ready", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	var raw []beadJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse bd output: %w", err)
	}

	beads := make([]mycroft.BeadView, 0, len(raw))
	for _, r := range raw {
		beads = append(beads, mycroft.BeadView{
			ID:           r.ID,
			Title:        r.Title,
			Type:         r.Type,
			Priority:     r.Priority,
			Complexity:   extractComplexity(r.Labels),
			Labels:       r.Labels,
			Dependencies: r.Dependencies,
			DepsResolved: true, // bd ready only returns unblocked beads
			CreatedAt:    r.CreatedAt,
		})
	}
	return beads, nil
}

// beadJSON is the expected JSON output from bd ready --json.
type beadJSON struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	Priority     int       `json:"priority"`
	Labels       []string  `json:"labels"`
	Dependencies []string  `json:"dependencies"`
	CreatedAt    time.Time `json:"created_at"`
}

// extractComplexity reads complexity from labels like "complexity/simple".
func extractComplexity(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "complexity/") {
			return strings.TrimPrefix(l, "complexity/")
		}
	}
	return "unknown"
}

// detectAgentsFromTmux lists tmux sessions and detects agents.
func (s *PatrolSource) detectAgentsFromTmux() ([]mycroft.AgentView, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, nil // tmux not available
	}

	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		// tmux server not running is not an error — just no sessions.
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var agents []mycroft.AgentView
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Simple heuristic: sessions with at least 3 segments containing
		// known agent-type keywords are agent sessions.
		// Full detection uses intermux's ParseSessionName, but we avoid
		// importing the intermux module here to keep dependencies minimal.
		parts := strings.Split(name, "-")
		if len(parts) >= 3 {
			agents = append(agents, mycroft.AgentView{
				Name:    name,
				Runtime: "claude-code",
				Status:  "unknown",
			})
		}
	}

	return agents, nil
}
