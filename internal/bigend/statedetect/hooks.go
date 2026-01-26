package statedetect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HookEvent represents a state event emitted by agent hooks.
type HookEvent struct {
	State       string    `json:"state"`
	AgentType   string    `json:"agent_type"`
	SessionID   string    `json:"session_id"`
	ProjectDir  string    `json:"project_dir"`
	ProjectName string    `json:"project_name"`
	Timestamp   time.Time `json:"timestamp"`
}

// HookStateReader reads agent state from hook-emitted files.
//
// Agent hooks (Claude Code, Codex CLI) write state events to:
//   ~/.autarch/agent-states/{agent}-{project}.json
//
// This provides authoritative state information directly from agents,
// which is more reliable than terminal pattern matching.
type HookStateReader struct {
	statesDir string
	mu        sync.RWMutex
	cache     map[string]*cachedHookState
	ttl       time.Duration
}

type cachedHookState struct {
	event    *HookEvent
	readAt   time.Time
	fileTime time.Time
}

// NewHookStateReader creates a reader for hook-emitted state files.
func NewHookStateReader() *HookStateReader {
	homeDir, _ := os.UserHomeDir()
	return &HookStateReader{
		statesDir: filepath.Join(homeDir, ".autarch", "agent-states"),
		cache:     make(map[string]*cachedHookState),
		ttl:       2 * time.Second,
	}
}

// NewHookStateReaderWithDir creates a reader with a custom states directory.
func NewHookStateReaderWithDir(statesDir string) *HookStateReader {
	return &HookStateReader{
		statesDir: statesDir,
		cache:     make(map[string]*cachedHookState),
		ttl:       2 * time.Second,
	}
}

// GetState reads the latest state for an agent/project combination.
//
// Returns nil if no state file exists or the file is stale (>30s old).
func (r *HookStateReader) GetState(agentType, projectName string) *HookEvent {
	key := agentType + "-" + projectName
	filePath := filepath.Join(r.statesDir, key+".json")

	// Check cache first
	r.mu.RLock()
	cached, ok := r.cache[key]
	r.mu.RUnlock()

	if ok && time.Since(cached.readAt) < r.ttl {
		return cached.event
	}

	// Read file
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}

	// Check if file is stale (agent might have crashed)
	if time.Since(info.ModTime()) > 30*time.Second {
		return nil
	}

	// Check if file changed since last read
	if ok && cached.fileTime.Equal(info.ModTime()) {
		// File unchanged, just update read time
		r.mu.Lock()
		cached.readAt = time.Now()
		r.mu.Unlock()
		return cached.event
	}

	// Read and parse file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var event HookEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil
	}

	// Update cache
	r.mu.Lock()
	r.cache[key] = &cachedHookState{
		event:    &event,
		readAt:   time.Now(),
		fileTime: info.ModTime(),
	}
	r.mu.Unlock()

	return &event
}

// GetStateBySession looks up state by session name.
//
// Session names are expected to be in format: {agent}-{project}
// e.g., "claude-Autarch", "codex-shadow-work"
func (r *HookStateReader) GetStateBySession(sessionName string) *HookEvent {
	// Parse session name to extract agent type and project
	agentType, projectName := parseSessionName(sessionName)
	if agentType == "" {
		return nil
	}

	return r.GetState(agentType, projectName)
}

// parseSessionName extracts agent type and project from session name.
func parseSessionName(name string) (agentType, projectName string) {
	lower := strings.ToLower(name)

	// Check for known agent prefixes
	prefixes := []struct {
		prefix    string
		agentType string
	}{
		{"claude-", "claude"},
		{"cc-", "claude"},
		{"codex-", "codex"},
		{"cx-", "codex"},
		{"aider-", "aider"},
	}

	for _, p := range prefixes {
		if strings.HasPrefix(lower, p.prefix) {
			return p.agentType, name[len(p.prefix):]
		}
	}

	// Check for agent name anywhere in session name
	if strings.Contains(lower, "claude") {
		return "claude", extractProjectName(name, "claude")
	}
	if strings.Contains(lower, "codex") {
		return "codex", extractProjectName(name, "codex")
	}

	return "", ""
}

// extractProjectName removes the agent name from session name to get project.
func extractProjectName(sessionName, agentName string) string {
	lower := strings.ToLower(sessionName)
	idx := strings.Index(lower, agentName)
	if idx == -1 {
		return sessionName
	}

	// Remove agent name and surrounding separators
	result := sessionName[:idx] + sessionName[idx+len(agentName):]
	result = strings.Trim(result, "-_")
	if result == "" {
		return sessionName
	}
	return result
}

// ToAgentState converts a hook state string to AgentState enum.
func (e *HookEvent) ToAgentState() AgentState {
	switch strings.ToLower(e.State) {
	case "working", "executing_tool":
		return StateWorking
	case "waiting":
		return StateWaiting
	case "blocked", "blocked_permission":
		return StateBlocked
	case "stalled":
		return StateStalled
	case "done", "stopped":
		return StateDone
	case "error":
		return StateError
	default:
		return StateUnknown
	}
}

// ToStateResult converts a HookEvent to a StateResult.
func (e *HookEvent) ToStateResult() StateResult {
	return StateResult{
		State:      e.ToAgentState(),
		Confidence: 1.0, // Hooks provide authoritative state
		Source:     SourceHook,
		DetectedAt: e.Timestamp,
	}
}

// SourceHook indicates state was determined from agent hooks.
const SourceHook DetectionSource = "hook"

// ListAllStates returns all current agent states from state files.
func (r *HookStateReader) ListAllStates() []*HookEvent {
	entries, err := os.ReadDir(r.statesDir)
	if err != nil {
		return nil
	}

	var events []*HookEvent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Skip events.log
		if entry.Name() == "events.log" {
			continue
		}

		filePath := filepath.Join(r.statesDir, entry.Name())
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) > 30*time.Second {
			continue // Skip stale files
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var event HookEvent
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		events = append(events, &event)
	}

	return events
}
