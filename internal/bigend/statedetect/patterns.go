package statedetect

import (
	"regexp"
	"strings"
)

// PatternRule defines a regex pattern that maps to an agent state.
type PatternRule struct {
	// Name is a human-readable identifier for the pattern.
	Name string

	// Pattern is the compiled regex.
	Pattern *regexp.Regexp

	// State is the agent state to assign when this pattern matches.
	State AgentState

	// Confidence is the confidence level for this pattern (0.0-1.0).
	Confidence float64

	// AgentTypes limits this pattern to specific agents (empty = all agents).
	AgentTypes []string

	// MatchMode controls how the pattern is applied.
	MatchMode MatchMode

	// Priority determines order when multiple patterns match (higher = first).
	Priority int
}

// MatchMode controls where in the output to look for patterns.
type MatchMode int

const (
	// MatchLastLines checks only the last N non-empty lines.
	MatchLastLines MatchMode = iota

	// MatchAnywhere checks the entire captured output.
	MatchAnywhere

	// MatchLineStart requires the pattern to match at line start.
	MatchLineStart
)

// DefaultPatterns returns the standard pattern rules for common agent types.
// These are ordered by priority and cover Claude, Codex, Aider, and generic patterns.
func DefaultPatterns() []PatternRule {
	return []PatternRule{
		// ─────────────────────────────────────────────────────────────────────
		// ERROR patterns (highest priority - detect problems immediately)
		// ─────────────────────────────────────────────────────────────────────
		{
			Name:       "error-explicit",
			Pattern:    regexp.MustCompile(`(?i)\b(error|exception|panic|fatal|failed|failure):`),
			State:      StateError,
			Confidence: 0.95,
			MatchMode:  MatchLastLines,
			Priority:   100,
		},
		{
			Name:       "error-traceback",
			Pattern:    regexp.MustCompile(`(?i)(traceback|stack trace|at .+:\d+)`),
			State:      StateError,
			Confidence: 0.90,
			MatchMode:  MatchLastLines,
			Priority:   99,
		},
		{
			Name:       "error-api-limit",
			Pattern:    regexp.MustCompile(`(?i)(rate limit|quota exceeded|too many requests|429)`),
			State:      StateError,
			Confidence: 0.95,
			MatchMode:  MatchAnywhere,
			Priority:   98,
		},

		// ─────────────────────────────────────────────────────────────────────
		// BLOCKED patterns (needs permission)
		// ─────────────────────────────────────────────────────────────────────
		{
			Name:       "blocked-approve",
			Pattern:    regexp.MustCompile(`(?i)\b(approve|allow|permit|accept)\?`),
			State:      StateBlocked,
			Confidence: 0.95,
			MatchMode:  MatchLastLines,
			Priority:   90,
		},
		{
			Name:       "blocked-yesno",
			Pattern:    regexp.MustCompile(`(?i)\[y/n\]|\(y/n\)|\[yes/no\]`),
			State:      StateBlocked,
			Confidence: 0.90,
			MatchMode:  MatchLastLines,
			Priority:   89,
		},
		{
			Name:       "blocked-continue",
			Pattern:    regexp.MustCompile(`(?i)press enter to continue|hit enter|type 'yes'`),
			State:      StateBlocked,
			Confidence: 0.85,
			MatchMode:  MatchLastLines,
			Priority:   88,
		},

		// ─────────────────────────────────────────────────────────────────────
		// DONE patterns
		// ─────────────────────────────────────────────────────────────────────
		{
			Name:       "done-completed",
			Pattern:    regexp.MustCompile(`(?i)\b(completed|finished|done|success)(!|\.|\s*$)`),
			State:      StateDone,
			Confidence: 0.85,
			MatchMode:  MatchLastLines,
			Priority:   80,
		},
		{
			Name:       "done-exit",
			Pattern:    regexp.MustCompile(`(?i)(exiting|goodbye|session ended|bye)`),
			State:      StateDone,
			Confidence: 0.80,
			MatchMode:  MatchLastLines,
			Priority:   79,
		},

		// ─────────────────────────────────────────────────────────────────────
		// WAITING patterns (awaiting user input)
		// ─────────────────────────────────────────────────────────────────────
		{
			Name:       "waiting-claude-prompt",
			Pattern:    regexp.MustCompile(`^\s*\?\s+`),
			State:      StateWaiting,
			Confidence: 0.95,
			AgentTypes: []string{"claude"},
			MatchMode:  MatchLineStart,
			Priority:   70,
		},
		{
			Name:       "waiting-codex-prompt",
			Pattern:    regexp.MustCompile(`^\s*>\s*$`),
			State:      StateWaiting,
			Confidence: 0.95,
			AgentTypes: []string{"codex"},
			MatchMode:  MatchLineStart,
			Priority:   70,
		},
		{
			Name:       "waiting-aider-prompt",
			Pattern:    regexp.MustCompile(`^\s*(aider|>>>)\s*>`),
			State:      StateWaiting,
			Confidence: 0.95,
			AgentTypes: []string{"aider"},
			MatchMode:  MatchLineStart,
			Priority:   70,
		},
		{
			Name:       "waiting-generic-prompt",
			Pattern:    regexp.MustCompile(`^\s*[\$%#>❯]\s*$`),
			State:      StateWaiting,
			Confidence: 0.85,
			MatchMode:  MatchLineStart,
			Priority:   65,
		},
		{
			Name:       "waiting-input-request",
			Pattern:    regexp.MustCompile(`(?i)(enter|type|input|provide|what would you like)`),
			State:      StateWaiting,
			Confidence: 0.75,
			MatchMode:  MatchLastLines,
			Priority:   60,
		},

		// ─────────────────────────────────────────────────────────────────────
		// WORKING patterns (actively processing)
		// ─────────────────────────────────────────────────────────────────────
		{
			Name:       "working-spinner",
			Pattern:    regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏◐◑◒◓⣾⣽⣻⢿⡿⣟⣯⣷]`),
			State:      StateWorking,
			Confidence: 0.95,
			MatchMode:  MatchLastLines,
			Priority:   50,
		},
		{
			Name:       "working-thinking",
			Pattern:    regexp.MustCompile(`(?i)(thinking|processing|analyzing|generating|loading)`),
			State:      StateWorking,
			Confidence: 0.90,
			MatchMode:  MatchLastLines,
			Priority:   49,
		},
		{
			Name:       "working-tool-call",
			Pattern:    regexp.MustCompile(`(?i)(reading|writing|searching|running|executing|calling|fetching)`),
			State:      StateWorking,
			Confidence: 0.90,
			MatchMode:  MatchLastLines,
			Priority:   48,
		},
		{
			Name:       "working-progress",
			Pattern:    regexp.MustCompile(`\[\d+/\d+\]|\d+%|\.{3,}`),
			State:      StateWorking,
			Confidence: 0.85,
			MatchMode:  MatchLastLines,
			Priority:   47,
		},
		{
			Name:       "working-claude-edit",
			Pattern:    regexp.MustCompile(`(Edit|Write|Read|Bash|Grep|Glob)\s*━`),
			State:      StateWorking,
			Confidence: 0.95,
			AgentTypes: []string{"claude"},
			MatchMode:  MatchLastLines,
			Priority:   52,
		},
	}
}

// PatternMatcher applies pattern rules to terminal output.
type PatternMatcher struct {
	rules []PatternRule
}

// NewPatternMatcher creates a matcher with the given rules.
func NewPatternMatcher(rules []PatternRule) *PatternMatcher {
	// Sort by priority (highest first)
	sorted := make([]PatternRule, len(rules))
	copy(sorted, rules)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Priority > sorted[i].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return &PatternMatcher{rules: sorted}
}

// Match tries to find a matching pattern in the output.
// Returns nil if no pattern matches.
func (m *PatternMatcher) Match(output string, agentType string) *StateResult {
	// Get last N non-empty lines for MatchLastLines mode
	lines := strings.Split(output, "\n")
	lastLines := extractLastLines(lines, 10)
	lastLinesText := strings.Join(lastLines, "\n")

	for _, rule := range m.rules {
		// Check agent type filter
		if len(rule.AgentTypes) > 0 && !containsIgnoreCase(rule.AgentTypes, agentType) {
			continue
		}

		var text string
		switch rule.MatchMode {
		case MatchLastLines:
			text = lastLinesText
		case MatchAnywhere:
			text = output
		case MatchLineStart:
			// Check each of the last lines
			for _, line := range lastLines {
				if rule.Pattern.MatchString(line) {
					return &StateResult{
						State:          rule.State,
						Confidence:     rule.Confidence,
						Source:         SourcePattern,
						MatchedPattern: rule.Name,
					}
				}
			}
			continue
		}

		if rule.Pattern.MatchString(text) {
			return &StateResult{
				State:          rule.State,
				Confidence:     rule.Confidence,
				Source:         SourcePattern,
				MatchedPattern: rule.Name,
			}
		}
	}

	return nil
}

// extractLastLines returns the last N non-empty lines.
func extractLastLines(lines []string, n int) []string {
	result := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(result) < n; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			result = append([]string{line}, result...)
		}
	}
	return result
}

// containsIgnoreCase checks if slice contains string (case insensitive).
func containsIgnoreCase(slice []string, s string) bool {
	s = strings.ToLower(s)
	for _, item := range slice {
		if strings.ToLower(item) == s {
			return true
		}
	}
	return false
}
