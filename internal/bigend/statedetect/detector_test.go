package statedetect

import (
	"testing"
	"time"
)

func TestPatternMatcherClaudeWorking(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	tests := []struct {
		name      string
		output    string
		agentType string
		wantState AgentState
	}{
		{
			name:      "spinner character",
			output:    "  ⠋ Thinking...",
			agentType: "claude",
			wantState: StateWorking,
		},
		{
			name:      "thinking text",
			output:    "Thinking about the problem...\n",
			agentType: "claude",
			wantState: StateWorking,
		},
		{
			name:      "tool call - Read",
			output:    "Read ━━━━━━━━━━━━━━━━━",
			agentType: "claude",
			wantState: StateWorking,
		},
		{
			name:      "tool call - Edit",
			output:    "Edit ━━━━━━━━━━━━━━━━━",
			agentType: "claude",
			wantState: StateWorking,
		},
		{
			name:      "reading file",
			output:    "Reading package.json...",
			agentType: "claude",
			wantState: StateWorking,
		},
		{
			name:      "searching",
			output:    "Searching for files...",
			agentType: "claude",
			wantState: StateWorking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(tt.output, tt.agentType)
			if result == nil {
				t.Fatalf("expected match, got nil")
			}
			if result.State != tt.wantState {
				t.Errorf("got state %q, want %q", result.State, tt.wantState)
			}
		})
	}
}

func TestPatternMatcherClaudeWaiting(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	tests := []struct {
		name      string
		output    string
		agentType string
		wantState AgentState
	}{
		{
			name:      "claude prompt in context",
			output:    "Done reading files.\n\n? What should I do next?\n",
			agentType: "claude",
			wantState: StateWaiting,
		},
		{
			name:      "what would you like",
			output:    "What would you like me to do?",
			agentType: "claude",
			wantState: StateWaiting,
		},
		{
			name:      "enter a command",
			output:    "Enter a command:",
			agentType: "claude",
			wantState: StateWaiting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(tt.output, tt.agentType)
			if result == nil {
				t.Fatalf("expected match, got nil")
			}
			if result.State != tt.wantState {
				t.Errorf("got state %q, want %q", result.State, tt.wantState)
			}
		})
	}
}

func TestPatternMatcherBlocked(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	tests := []struct {
		name   string
		output string
	}{
		{"approve question", "Approve?"},
		{"allow question", "Allow?"},
		{"yes no brackets", "[y/n]"},
		{"yes no parens", "(y/n)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(tt.output, "claude")
			if result == nil {
				t.Fatalf("expected match, got nil")
			}
			if result.State != StateBlocked {
				t.Errorf("got state %q, want %q", result.State, StateBlocked)
			}
		})
	}
}

func TestPatternMatcherError(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	tests := []struct {
		name   string
		output string
	}{
		{"error keyword", "Error: something went wrong"},
		{"exception", "Exception: null pointer"},
		{"panic", "panic: runtime error"},
		{"rate limit", "Rate limit exceeded, try again"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.Match(tt.output, "claude")
			if result == nil {
				t.Fatalf("expected match, got nil")
			}
			if result.State != StateError {
				t.Errorf("got state %q, want %q", result.State, StateError)
			}
		})
	}
}

func TestPatternMatcherCodexPrompt(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	output := "> "
	result := matcher.Match(output, "codex")
	if result == nil {
		t.Fatalf("expected match for codex prompt")
	}
	if result.State != StateWaiting {
		t.Errorf("got state %q, want %q", result.State, StateWaiting)
	}
}

func TestDetectorRepetitionDetection(t *testing.T) {
	cfg := DetectorConfig{
		StallThreshold:   100 * time.Millisecond, // Short for testing
		StallRepetitions: 3,
		IdleThreshold:    30 * time.Second,
		HistorySize:      10,
	}
	detector := NewDetectorWithConfig(cfg)

	sessionName := "test-session"
	output := "Some output that keeps repeating"
	lastActivity := time.Now()

	// First call - just adds to history
	result := detector.Detect(sessionName, output, "claude", lastActivity)
	if result.State == StateStalled {
		t.Error("should not be stalled after first call")
	}

	// Second call - still not stalled (need 3 repetitions)
	time.Sleep(50 * time.Millisecond)
	result = detector.Detect(sessionName, output, "claude", lastActivity)
	if result.State == StateStalled {
		t.Error("should not be stalled after second call")
	}

	// Third call - still not stalled (threshold not met)
	time.Sleep(50 * time.Millisecond)
	result = detector.Detect(sessionName, output, "claude", lastActivity)
	// May or may not be stalled depending on timing

	// Fourth call after threshold - should be stalled
	time.Sleep(100 * time.Millisecond)
	result = detector.Detect(sessionName, output, "claude", lastActivity)
	if result.State != StateStalled {
		t.Errorf("expected stalled state after repeated output, got %q", result.State)
	}
	if result.Source != SourceRepetition {
		t.Errorf("expected source %q, got %q", SourceRepetition, result.Source)
	}
}

func TestDetectorClearHistory(t *testing.T) {
	detector := NewDetector()
	sessionName := "test-session"
	output := "test output"

	// Add some history
	detector.Detect(sessionName, output, "claude", time.Now())
	detector.Detect(sessionName, output, "claude", time.Now())

	// Clear history
	detector.ClearHistory(sessionName)

	// Verify history is cleared by checking internal state
	detector.mu.RLock()
	_, exists := detector.history[sessionName]
	detector.mu.RUnlock()

	if exists {
		t.Error("expected history to be cleared")
	}
}

func TestDetectorAgentTypeFiltering(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	// Claude-specific prompt should not match for codex
	output := "? "
	result := matcher.Match(output, "codex")

	// Should not match the claude-specific pattern
	if result != nil && result.MatchedPattern == "waiting-claude-prompt" {
		t.Error("claude prompt pattern should not match for codex")
	}
}

func TestStateResultHelpers(t *testing.T) {
	tests := []struct {
		state          AgentState
		wantActive     bool
		wantAttention  bool
	}{
		{StateWorking, true, false},
		{StateWaiting, true, true},
		{StateBlocked, true, true},
		{StateStalled, false, true},
		{StateDone, false, false},
		{StateError, false, true},
		{StateUnknown, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsActive(); got != tt.wantActive {
				t.Errorf("IsActive() = %v, want %v", got, tt.wantActive)
			}
			if got := tt.state.NeedsAttention(); got != tt.wantAttention {
				t.Errorf("NeedsAttention() = %v, want %v", got, tt.wantAttention)
			}
		})
	}
}

func TestConfidenceLevels(t *testing.T) {
	matcher := NewPatternMatcher(DefaultPatterns())

	// Spinner should have high confidence
	result := matcher.Match("⠋ Loading...", "claude")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Confidence < 0.9 {
		t.Errorf("spinner pattern should have high confidence, got %f", result.Confidence)
	}

	// Error patterns should have high confidence
	result = matcher.Match("Error: connection refused", "claude")
	if result == nil {
		t.Fatal("expected match")
	}
	if result.Confidence < 0.9 {
		t.Errorf("error pattern should have high confidence, got %f", result.Confidence)
	}
}
