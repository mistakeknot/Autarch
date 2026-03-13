package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AggregatorSource composes fleet state from multiple external systems:
// tmux sessions (agent detection), bd CLI (work queue), and the fleet
// registry (agent capabilities/cost profiles). Results are cached with TTL.
type AggregatorSource struct {
	registryPath string
	registry     []AgentSpec
	registryOnce sync.Once

	mu        sync.RWMutex
	cache     FleetView
	cacheTime time.Time
	cacheTTL  time.Duration
}

// AggregatorOption configures an AggregatorSource.
type AggregatorOption func(*AggregatorSource)

// WithCacheTTL sets the cache time-to-live. Default is 10 seconds.
func WithCacheTTL(d time.Duration) AggregatorOption {
	return func(a *AggregatorSource) { a.cacheTTL = d }
}

// NewAggregatorSource creates a source that aggregates fleet state from
// tmux, bd, and the fleet registry.
func NewAggregatorSource(registryPath string, opts ...AggregatorOption) *AggregatorSource {
	a := &AggregatorSource{
		registryPath: registryPath,
		cacheTTL:     10 * time.Second,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// FleetState returns the composed fleet view, using cache if fresh.
func (a *AggregatorSource) FleetState() (FleetView, error) {
	a.mu.RLock()
	if !a.cacheTime.IsZero() && time.Since(a.cacheTime) < a.cacheTTL {
		v := a.cache
		a.mu.RUnlock()
		return v, nil
	}
	a.mu.RUnlock()

	view, err := a.refresh()
	if err != nil {
		return FleetView{}, err
	}

	a.mu.Lock()
	a.cache = view
	a.cacheTime = time.Now()
	a.mu.Unlock()

	return view, nil
}

// AgentHealth returns the status of a specific agent.
func (a *AggregatorSource) AgentHealth(name string) (string, error) {
	view, err := a.FleetState()
	if err != nil {
		return "", err
	}
	for _, ag := range view.Agents {
		if ag.Name == name {
			return ag.Status, nil
		}
	}
	return "unknown", nil
}

// BeadQueue returns the current work queue.
func (a *AggregatorSource) BeadQueue() ([]BeadView, error) {
	return queryBeadQueue()
}

// refresh composes a FleetView from all sources.
func (a *AggregatorSource) refresh() (FleetView, error) {
	now := time.Now()
	view := FleetView{
		Freshness: make(map[string]time.Time),
	}

	// Load registry once.
	a.registryOnce.Do(func() {
		a.registry = LoadRegistryOrEmpty(a.registryPath)
	})

	// Query agents from tmux sessions.
	agents, err := detectAgentsFromTmux()
	if err != nil {
		return view, fmt.Errorf("detect agents: %w", err)
	}

	// Enrich agents with registry data.
	for i := range agents {
		a.enrichAgent(&agents[i])
	}

	// Also add registry agents not seen in tmux (they're offline).
	seen := make(map[string]bool, len(agents))
	for _, ag := range agents {
		seen[ag.Name] = true
	}
	for _, spec := range a.registry {
		if !seen[spec.Name] && spec.Runtime.Mode == "cli" {
			agents = append(agents, AgentView{
				Name:         spec.Name,
				Runtime:      runtimeLabel(spec.Runtime),
				Capabilities: spec.Capabilities,
				Status:       "offline",
				CostProfile: CostProfile{
					Model: spec.Models.Preferred,
				},
			})
		}
	}

	view.Agents = agents
	view.Freshness["tmux"] = now

	// Query work queue.
	beads, err := queryBeadQueue()
	if err != nil {
		// Non-fatal — degrade gracefully.
		view.Freshness["beads"] = time.Time{}
	} else {
		view.Work = beads
		view.Freshness["beads"] = now
	}

	// Query interlock for conflicts (optional — degrade gracefully).
	conflicts, err := queryInterlockConflicts()
	if err == nil {
		view.Conflicts = conflicts
		view.Freshness["interlock"] = now
	}

	return view, nil
}

// enrichAgent merges registry data into a detected agent view.
func (a *AggregatorSource) enrichAgent(agent *AgentView) {
	spec, ok := FindByName(a.registry, agent.Name)
	if !ok {
		return
	}
	if len(agent.Capabilities) == 0 {
		agent.Capabilities = spec.Capabilities
	}
	if agent.CostProfile.Model == "" {
		agent.CostProfile.Model = spec.Models.Preferred
	}
	if agent.Runtime == "" {
		agent.Runtime = runtimeLabel(spec.Runtime)
	}
}

func runtimeLabel(r Runtime) string {
	switch r.Mode {
	case "cli":
		if r.Binary != "" {
			return r.Binary
		}
		return "cli"
	case "subagent":
		return "subagent"
	default:
		return r.Mode
	}
}

// detectAgentsFromTmux lists tmux sessions and extracts agent sessions.
func detectAgentsFromTmux() ([]AgentView, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, nil
	}

	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_activity}").Output()
	if err != nil {
		// tmux server not running — not an error.
		return nil, nil
	}

	var agents []AgentView
	now := time.Now()

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		name := parts[0]

		// Heuristic: sessions with 3+ segments are likely agent sessions.
		segments := strings.Split(name, "-")
		if len(segments) < 3 {
			continue
		}

		agent := AgentView{
			Name:    name,
			Runtime: "claude-code",
			Status:  "unknown",
		}

		// Parse activity timestamp if available.
		if len(parts) > 1 {
			if ts, err := time.Parse("2006-01-02T15:04:05", parts[1]); err == nil {
				agent.Health.LastSeen = ts
				agent.Health.IsHealthy = time.Since(ts) < 5*time.Minute
				if agent.Health.IsHealthy {
					agent.Status = "active"
				} else {
					agent.Status = "idle"
				}
			} else {
				// Activity is epoch seconds from tmux.
				agent.Health.LastSeen = now
				agent.Health.IsHealthy = true
				agent.Status = "active"
			}
		}

		agents = append(agents, agent)
	}

	return agents, nil
}

// beadJSON matches the JSON output from bd ready --json.
type beadJSON struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	Priority     int       `json:"priority"`
	Labels       []string  `json:"labels"`
	Dependencies []string  `json:"dependencies"`
	CreatedAt    time.Time `json:"created_at"`
}

// queryBeadQueue runs bd ready --json and parses the results.
func queryBeadQueue() ([]BeadView, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return nil, nil
	}

	out, err := exec.Command("bd", "ready", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	var raw []beadJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse bd output: %w", err)
	}

	beads := make([]BeadView, 0, len(raw))
	for _, r := range raw {
		beads = append(beads, BeadView{
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

func extractComplexity(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "complexity/") {
			return strings.TrimPrefix(l, "complexity/")
		}
	}
	return "unknown"
}

// interlockConflictJSON matches interlock conflict output.
type interlockConflictJSON struct {
	File    string   `json:"file"`
	Holders []string `json:"holders"`
}

// queryInterlockConflicts checks for file reservation conflicts.
func queryInterlockConflicts() ([]ConflictView, error) {
	if _, err := exec.LookPath("ic"); err != nil {
		return nil, nil
	}

	out, err := exec.Command("ic", "conflicts", "--json").Output()
	if err != nil {
		return nil, nil // interlock not running — degrade gracefully
	}

	var raw []interlockConflictJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, nil
	}

	conflicts := make([]ConflictView, 0, len(raw))
	for _, r := range raw {
		if len(r.Holders) > 1 {
			conflicts = append(conflicts, ConflictView{
				File:    r.File,
				Holders: r.Holders,
			})
		}
	}
	return conflicts, nil
}

// DiscoverRegistryPath finds fleet-registry.yaml by searching common locations.
// Returns empty string if not found.
func DiscoverRegistryPath() string {
	// Check environment variable first.
	if p := os.Getenv("FLEET_REGISTRY_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Walk up from cwd looking for os/Clavain/config/fleet-registry.yaml.
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "os", "Clavain", "config", "fleet-registry.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Check home directory convention.
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".clavain", "fleet-registry.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}
