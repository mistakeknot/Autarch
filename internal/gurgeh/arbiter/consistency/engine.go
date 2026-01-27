package consistency

import "strings"

// SprintState mirrors the fields needed for consistency checking.
// This avoids import cycles with the parent arbiter package.
type SprintState struct {
	Sections map[int]*SectionInfo
}

// SectionInfo holds the minimum section data needed for checking.
type SectionInfo struct {
	Content  string
	Accepted bool
}

// Conflict represents a consistency issue.
type Conflict struct {
	TypeCode int
	Severity int // 0 = blocker, 1 = warning
	Message  string
	Sections []int
}

// Engine checks for consistency conflicts between PRD sections.
type Engine struct{}

// NewEngine creates a new consistency Engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Check analyzes sections for conflicts.
func (e *Engine) Check(sections map[int]*SectionInfo) []Conflict {
	var conflicts []Conflict

	problem := sections[0]    // PhaseProblem
	features := sections[2]   // PhaseFeaturesGoals

	if problem != nil && features != nil &&
		problem.Accepted && features.Accepted {
		conflicts = append(conflicts, e.checkUserFeatureAlignment(problem, features)...)
	}

	return conflicts
}

func (e *Engine) checkUserFeatureAlignment(problem, features *SectionInfo) []Conflict {
	problemLower := strings.ToLower(problem.Content)
	featuresLower := strings.ToLower(features.Content)

	if (strings.Contains(problemLower, "solo") || strings.Contains(problemLower, "individual")) &&
		(strings.Contains(featuresLower, "enterprise") || strings.Contains(featuresLower, "100+")) {
		return []Conflict{{
			TypeCode: 0, // ConflictUserFeature
			Severity: 0, // SeverityBlocker
			Message:  "Feature targets enterprise users but problem describes solo/individual users",
			Sections: []int{0, 2},
		}}
	}

	return nil
}
