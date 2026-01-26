package statedetect

import (
	"hash/fnv"
	"sync"
	"time"
)

// DetectorConfig configures the state detector behavior.
type DetectorConfig struct {
	// StallThreshold is how long output must repeat before declaring stalled.
	StallThreshold time.Duration

	// StallRepetitions is how many identical outputs needed for stall detection.
	StallRepetitions int

	// IdleThreshold is how long without activity before declaring idle.
	IdleThreshold time.Duration

	// HistorySize is how many output snapshots to keep for repetition detection.
	HistorySize int

	// UseHooks enables reading state from hook-emitted files (Tier 0).
	// When true, hook state takes priority over pattern matching.
	UseHooks bool
}

// DefaultConfig returns sensible defaults for state detection.
func DefaultConfig() DetectorConfig {
	return DetectorConfig{
		StallThreshold:   60 * time.Second,
		StallRepetitions: 3,
		IdleThreshold:    30 * time.Second,
		HistorySize:      10,
		UseHooks:         true, // Prefer hook-based state when available
	}
}

// Detector performs four-tier agent state detection.
//
// Tier 0: Hook-based state (authoritative, from agent hooks)
// Tier 1: Fast pattern matching (handles ~90% of cases)
// Tier 2: Repetition detection for stall states
// Tier 3: Activity-based fallback
//
// Future: LLM classification for novel patterns.
type Detector struct {
	config     DetectorConfig
	matcher    *PatternMatcher
	hookReader *HookStateReader

	// Per-session output history for repetition detection
	mu      sync.RWMutex
	history map[string]*outputHistory
}

// outputHistory tracks recent outputs for a session.
type outputHistory struct {
	snapshots []OutputSnapshot
	mu        sync.Mutex
}

// NewDetector creates a state detector with default patterns.
func NewDetector() *Detector {
	return NewDetectorWithConfig(DefaultConfig())
}

// NewDetectorWithConfig creates a detector with custom configuration.
func NewDetectorWithConfig(cfg DetectorConfig) *Detector {
	d := &Detector{
		config:  cfg,
		matcher: NewPatternMatcher(DefaultPatterns()),
		history: make(map[string]*outputHistory),
	}
	if cfg.UseHooks {
		d.hookReader = NewHookStateReader()
	}
	return d
}

// Detect determines the current state of an agent session.
//
// Parameters:
//   - sessionName: tmux session identifier
//   - output: recent terminal output (typically 50 lines)
//   - agentType: "claude", "codex", "aider", etc.
//   - lastActivity: timestamp of last tmux activity
func (d *Detector) Detect(sessionName, output, agentType string, lastActivity time.Time) StateResult {
	now := time.Now()

	// Tier 0: Hook-based state (authoritative, highest confidence)
	if d.hookReader != nil {
		if event := d.hookReader.GetStateBySession(sessionName); event != nil {
			result := event.ToStateResult()
			// Hook state is authoritative but check if it's recent
			if time.Since(event.Timestamp) < 30*time.Second {
				return result
			}
			// Hook state is stale, fall through to pattern matching
		}
	}

	// Tier 1: Pattern matching (fast, high confidence)
	if result := d.matcher.Match(output, agentType); result != nil {
		result.DetectedAt = now
		return *result
	}

	// Tier 2: Repetition detection (medium confidence)
	if d.isRepeating(sessionName, output) {
		return StateResult{
			State:      StateStalled,
			Confidence: 0.80,
			Source:     SourceRepetition,
			DetectedAt: now,
		}
	}

	// Tier 3: Activity-based fallback
	if time.Since(lastActivity) > d.config.IdleThreshold {
		return StateResult{
			State:      StateUnknown,
			Confidence: 0.60,
			Source:     SourceActivity,
			DetectedAt: now,
		}
	}

	// Default: assume working if there's recent activity
	return StateResult{
		State:      StateWorking,
		Confidence: 0.50,
		Source:     SourceDefault,
		DetectedAt: now,
	}
}

// isRepeating checks if recent output is repeating (stall indicator).
func (d *Detector) isRepeating(sessionName, output string) bool {
	d.mu.Lock()
	hist, ok := d.history[sessionName]
	if !ok {
		hist = &outputHistory{
			snapshots: make([]OutputSnapshot, 0, d.config.HistorySize),
		}
		d.history[sessionName] = hist
	}
	d.mu.Unlock()

	hist.mu.Lock()
	defer hist.mu.Unlock()

	// Calculate hash of current output
	h := fnv.New64a()
	h.Write([]byte(normalizeOutput(output)))
	currentHash := h.Sum64()

	// Add to history
	snapshot := OutputSnapshot{
		Content:    output,
		CapturedAt: time.Now(),
		Hash:       currentHash,
	}

	hist.snapshots = append(hist.snapshots, snapshot)
	if len(hist.snapshots) > d.config.HistorySize {
		hist.snapshots = hist.snapshots[1:]
	}

	// Need at least N snapshots to detect repetition
	if len(hist.snapshots) < d.config.StallRepetitions {
		return false
	}

	// Check if last N snapshots have the same hash
	lastN := hist.snapshots[len(hist.snapshots)-d.config.StallRepetitions:]
	firstHash := lastN[0].Hash
	for _, snap := range lastN[1:] {
		if snap.Hash != firstHash {
			return false
		}
	}

	// Check if repetition spans the threshold duration
	oldest := lastN[0].CapturedAt
	newest := lastN[len(lastN)-1].CapturedAt
	return newest.Sub(oldest) >= d.config.StallThreshold
}

// ClearHistory removes output history for a session (e.g., on session restart).
func (d *Detector) ClearHistory(sessionName string) {
	d.mu.Lock()
	delete(d.history, sessionName)
	d.mu.Unlock()
}

// normalizeOutput removes volatile content (timestamps, PIDs, etc.) for stable hashing.
func normalizeOutput(output string) string {
	// For now, just trim whitespace. Could be enhanced to strip:
	// - Timestamps
	// - Progress percentages
	// - Cursor position sequences
	// - Session-specific identifiers
	return output
}

// GetConfig returns the current detector configuration.
func (d *Detector) GetConfig() DetectorConfig {
	return d.config
}

// SetConfig updates the detector configuration.
func (d *Detector) SetConfig(cfg DetectorConfig) {
	d.config = cfg
}
