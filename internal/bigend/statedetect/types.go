// Package statedetect provides LLM-aware agent state detection.
//
// This implements a NudgeNik-style three-tier detection approach:
// 1. Fast pattern matching (handles 90%+ of cases)
// 2. Repetition detection for stall states
// 3. LLM fallback for novel output patterns (future)
package statedetect

import "time"

// AgentState represents the current operational state of an AI agent.
type AgentState string

const (
	// StateUnknown indicates the state could not be determined.
	StateUnknown AgentState = "unknown"

	// StateWorking means the agent is actively processing (thinking, reading, writing).
	StateWorking AgentState = "working"

	// StateWaiting means the agent is waiting for user input at a prompt.
	StateWaiting AgentState = "waiting"

	// StateBlocked means the agent needs permission approval (Allow? Approve?).
	StateBlocked AgentState = "blocked"

	// StateStalled means no meaningful progress is being made (repeating output).
	StateStalled AgentState = "stalled"

	// StateDone means the agent has completed its task.
	StateDone AgentState = "done"

	// StateError means the agent encountered an error or crashed.
	StateError AgentState = "error"
)

// String returns the string representation of the state.
func (s AgentState) String() string {
	return string(s)
}

// IsActive returns true if the agent is doing something (working, waiting, or blocked).
func (s AgentState) IsActive() bool {
	return s == StateWorking || s == StateWaiting || s == StateBlocked
}

// NeedsAttention returns true if the agent needs human intervention.
func (s AgentState) NeedsAttention() bool {
	return s == StateWaiting || s == StateBlocked || s == StateStalled || s == StateError
}

// StateResult contains the detection result with confidence and source info.
type StateResult struct {
	// State is the detected agent state.
	State AgentState `json:"state"`

	// Confidence is a value from 0.0 to 1.0 indicating detection certainty.
	// Pattern matches typically have 0.9+, repetition detection ~0.8, LLM varies.
	Confidence float64 `json:"confidence"`

	// Source indicates how the state was detected.
	Source DetectionSource `json:"source"`

	// MatchedPattern is the pattern name that matched (if Source is SourcePattern).
	MatchedPattern string `json:"matched_pattern,omitempty"`

	// DetectedAt is when this state was detected.
	DetectedAt time.Time `json:"detected_at"`
}

// DetectionSource indicates how the state was determined.
type DetectionSource string

const (
	// SourcePattern means a regex pattern matched.
	SourcePattern DetectionSource = "pattern"

	// SourceRepetition means stall was detected via repeated output.
	SourceRepetition DetectionSource = "repetition"

	// SourceActivity means status was inferred from activity timestamps.
	SourceActivity DetectionSource = "activity"

	// SourceLLM means an LLM classified the output (future).
	SourceLLM DetectionSource = "llm"

	// SourceDefault means no detection method matched.
	SourceDefault DetectionSource = "default"
)

// OutputSnapshot represents a captured terminal output at a point in time.
type OutputSnapshot struct {
	// Content is the raw terminal output.
	Content string

	// CapturedAt is when the output was captured.
	CapturedAt time.Time

	// Hash is a quick fingerprint for repetition detection.
	Hash uint64
}
