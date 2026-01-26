package statedetect

import (
	"log/slog"
	"sync"
	"time"
)

// NudgeConfig configures nudge behavior for stalled agents.
type NudgeConfig struct {
	// Enabled controls whether nudging is active.
	Enabled bool

	// InitialDelay is how long to wait before first nudge after stall detected.
	InitialDelay time.Duration

	// RepeatInterval is how long between repeated nudges.
	RepeatInterval time.Duration

	// MaxNudges is the maximum number of nudges before giving up (0 = unlimited).
	MaxNudges int

	// Message is the text to send to the agent.
	// Supports placeholders: {session}, {duration}
	Message string
}

// DefaultNudgeConfig returns sensible defaults for nudging.
func DefaultNudgeConfig() NudgeConfig {
	return NudgeConfig{
		Enabled:        false, // Disabled by default - opt-in
		InitialDelay:   2 * time.Minute,
		RepeatInterval: 5 * time.Minute,
		MaxNudges:      3,
		Message:        "continue", // Simple nudge
	}
}

// TmuxSender can send keys to a tmux session.
type TmuxSender interface {
	SendKeys(sessionName string, keys string) error
}

// Nudger monitors stalled agents and sends nudges to wake them up.
type Nudger struct {
	config NudgeConfig
	sender TmuxSender

	mu       sync.Mutex
	sessions map[string]*sessionNudgeState
}

// sessionNudgeState tracks nudge state for a single session.
type sessionNudgeState struct {
	stalledAt   time.Time // When stall was first detected
	lastNudgeAt time.Time // When last nudge was sent
	nudgeCount  int       // Number of nudges sent
}

// NewNudger creates a new nudger with the given sender.
func NewNudger(sender TmuxSender, cfg NudgeConfig) *Nudger {
	return &Nudger{
		config:   cfg,
		sender:   sender,
		sessions: make(map[string]*sessionNudgeState),
	}
}

// Update processes a state detection result and nudges if appropriate.
//
// Returns true if a nudge was sent.
func (n *Nudger) Update(sessionName string, result StateResult) bool {
	if !n.config.Enabled {
		return false
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	state, ok := n.sessions[sessionName]

	// If not stalled, clear any existing state
	if result.State != StateStalled {
		if ok {
			delete(n.sessions, sessionName)
			slog.Debug("session recovered from stall", "session", sessionName)
		}
		return false
	}

	// Session is stalled
	now := time.Now()

	if !ok {
		// First time seeing this session stalled
		n.sessions[sessionName] = &sessionNudgeState{
			stalledAt: now,
		}
		slog.Info("session stalled, will nudge after initial delay",
			"session", sessionName,
			"delay", n.config.InitialDelay)
		return false
	}

	// Check if we've exceeded max nudges
	if n.config.MaxNudges > 0 && state.nudgeCount >= n.config.MaxNudges {
		slog.Warn("max nudges reached, giving up",
			"session", sessionName,
			"nudges", state.nudgeCount)
		return false
	}

	// Calculate when next nudge should happen
	var nextNudgeAt time.Time
	if state.nudgeCount == 0 {
		nextNudgeAt = state.stalledAt.Add(n.config.InitialDelay)
	} else {
		nextNudgeAt = state.lastNudgeAt.Add(n.config.RepeatInterval)
	}

	// Check if it's time to nudge
	if now.Before(nextNudgeAt) {
		return false
	}

	// Send the nudge
	if err := n.sender.SendKeys(sessionName, n.config.Message+"\n"); err != nil {
		slog.Error("failed to send nudge",
			"session", sessionName,
			"error", err)
		return false
	}

	state.lastNudgeAt = now
	state.nudgeCount++

	slog.Info("nudged stalled session",
		"session", sessionName,
		"nudge_count", state.nudgeCount,
		"stall_duration", now.Sub(state.stalledAt).Round(time.Second))

	return true
}

// Reset clears nudge state for a session (e.g., on manual intervention).
func (n *Nudger) Reset(sessionName string) {
	n.mu.Lock()
	delete(n.sessions, sessionName)
	n.mu.Unlock()
}

// ResetAll clears all nudge state.
func (n *Nudger) ResetAll() {
	n.mu.Lock()
	n.sessions = make(map[string]*sessionNudgeState)
	n.mu.Unlock()
}

// GetState returns the nudge state for a session (for debugging/monitoring).
func (n *Nudger) GetState(sessionName string) (stalledAt time.Time, nudgeCount int, exists bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	state, ok := n.sessions[sessionName]
	if !ok {
		return time.Time{}, 0, false
	}
	return state.stalledAt, state.nudgeCount, true
}

// StalledSessions returns a list of currently stalled session names.
func (n *Nudger) StalledSessions() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	names := make([]string, 0, len(n.sessions))
	for name := range n.sessions {
		names = append(names, name)
	}
	return names
}

// SetConfig updates the nudge configuration.
func (n *Nudger) SetConfig(cfg NudgeConfig) {
	n.mu.Lock()
	n.config = cfg
	n.mu.Unlock()
}

// GetConfig returns the current configuration.
func (n *Nudger) GetConfig() NudgeConfig {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.config
}
