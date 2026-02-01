package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// InterviewQuestion represents a single interview question with research integration.
// This type is defined here to avoid import cycles between tui and tui/views packages.
type InterviewQuestion struct {
	ID          string   // Stable identifier
	TopicKey    string   // Maps to research topics (e.g., "platform", "storage")
	Title       string   // Short title for navigation
	Prompt      string   // Full question text
	Placeholder string   // Input placeholder
	MultiLine   bool     // Allow multi-line input
	Options     []string // For choice questions (empty = text input)
}

// SpecSummary represents a completed spec ready for review.
// This type is defined here to avoid import cycles between tui and tui/views packages.
type SpecSummary struct {
	ProjectID    string
	Name         string
	Vision       string
	Users        string
	Problem      string
	Platform     string
	Language     string
	Requirements []string
	Decisions    []SpecDecision
}

// SpecDecision represents a decision made during the interview.
type SpecDecision struct {
	Key       string // e.g., "platform", "language"
	Value     string
	Source    string // "user" or InsightID
	InsightID string // If from research
}

// DefaultInterviewQuestions returns the standard onboarding questions.
func DefaultInterviewQuestions() []InterviewQuestion {
	return []InterviewQuestion{
		{
			ID:          "vision",
			TopicKey:    "vision",
			Title:       "Vision",
			Prompt:      "What is your project vision? Describe what you want to build.",
			Placeholder: "A tool that...",
			MultiLine:   true,
		},
		{
			ID:          "users",
			TopicKey:    "users",
			Title:       "Users",
			Prompt:      "Who are the primary users of this project?",
			Placeholder: "Developers who...",
			MultiLine:   false,
		},
		{
			ID:          "problem",
			TopicKey:    "problem",
			Title:       "Problem",
			Prompt:      "What problem are you solving?",
			Placeholder: "Currently, users have to...",
			MultiLine:   true,
		},
		{
			ID:          "platform",
			TopicKey:    "platform",
			Title:       "Platform",
			Prompt:      "What platform(s) will this run on?",
			Placeholder: "",
			Options:     []string{"Web", "CLI", "Desktop", "Mobile", "API/Backend"},
		},
		{
			ID:          "language",
			TopicKey:    "language",
			Title:       "Language",
			Prompt:      "What programming language(s) will you use?",
			Placeholder: "",
			Options:     []string{"Go", "TypeScript", "Python", "Rust", "Other"},
		},
		{
			ID:          "requirements",
			TopicKey:    "requirements",
			Title:       "Requirements",
			Prompt:      "List the key requirements (one per line or comma-separated).",
			Placeholder: "User authentication, Real-time updates, ...",
			MultiLine:   true,
		},
	}
}

// CreateSpecSummaryFromAnswers creates a SpecSummary from interview answers.
func CreateSpecSummaryFromAnswers(projectID string, answers map[string]string, decisions []SpecDecision) *SpecSummary {
	spec := &SpecSummary{
		ProjectID: projectID,
		Name:      answers["vision"],
		Vision:    answers["vision"],
		Users:     answers["users"],
		Problem:   answers["problem"],
		Platform:  answers["platform"],
		Language:  answers["language"],
		Decisions: decisions,
	}

	// Parse requirements
	if reqs := answers["requirements"]; reqs != "" {
		lines := splitLines(reqs)
		for _, line := range lines {
			line = trimSpace(line)
			if line != "" {
				spec.Requirements = append(spec.Requirements, line)
			}
		}
		// Also handle comma-separated
		if len(spec.Requirements) == 1 && containsComma(spec.Requirements[0]) {
			spec.Requirements = nil
			for _, req := range splitComma(answers["requirements"]) {
				req = trimSpace(req)
				if req != "" {
					spec.Requirements = append(spec.Requirements, req)
				}
			}
		}
	}

	return spec
}

// Helper functions to avoid importing strings package in views
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func containsComma(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return true
		}
	}
	return false
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// InterviewViewSetter is an interface for views that can receive AI suggestions.
type InterviewViewSetter interface {
	SetSuggestions(suggestions map[string]string)
	SetCompleteCallback(cb func(answers map[string]string) tea.Cmd)
}

// SpecSummaryViewSetter is an interface for spec summary views.
type SpecSummaryViewSetter interface {
	SetCallbacks(
		onGenerateEpics func(*SpecSummary) tea.Cmd,
		onEditSpec func(*SpecSummary) tea.Cmd,
		onWaitResearch func() tea.Cmd,
	)
}

// SprintStarter is an interface for views that can start a sprint.
type SprintStarter interface {
	StartSprint(userInput string) tea.Cmd
}

// ChatSettingsSetter allows views to receive persisted chat settings.
type ChatSettingsSetter interface {
	SetChatSettings(settings pkgtui.ChatSettings)
}

// ChatStreamSetter allows views to append streaming agent output.
type ChatStreamSetter interface {
	AppendChatLine(line string)
}

// DocumentSnapshotter allows views to provide a plain document snapshot.
type DocumentSnapshotter interface {
	DocumentSnapshot() (label, content string)
}
