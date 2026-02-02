package consistency

import "strings"

// Phase indices mirror arbiter.Phase to avoid import cycles.
const (
	PhaseVision        = 0
	PhaseProblem       = 1
	PhaseUsers         = 2
	PhaseFeaturesGoals = 3
)

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
type Engine struct {
	vision *VisionInfo
}

// NewEngine creates a new consistency Engine.
func NewEngine() *Engine {
	return &Engine{}
}

// SetVision configures the engine with vision context for vertical checks.
func (e *Engine) SetVision(vision *VisionInfo) {
	e.vision = vision
}

// Check analyzes sections for conflicts.
func (e *Engine) Check(sections map[int]*SectionInfo) []Conflict {
	var conflicts []Conflict

	problem := sections[int(PhaseProblem)]
	features := sections[int(PhaseFeaturesGoals)]

	if problem != nil && features != nil &&
		problem.Accepted && features.Accepted {
		conflicts = append(conflicts, e.checkUserFeatureAlignment(problem, features)...)
	}

	// Vertical checks against vision spec (if loaded)
	if e.vision != nil {
		conflicts = append(conflicts, CheckVisionAlignment(e.vision, sections)...)
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
			Sections: []int{int(PhaseProblem), int(PhaseFeaturesGoals)},
		}}
	}

	return nil
}
