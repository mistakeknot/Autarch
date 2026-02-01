// Package scan provides adapter types for carrying codebase scan artifacts
// into the arbiter sprint engine without creating import cycles.
//
// The tui layer converts tui.PhaseArtifacts → scan.Artifacts at the boundary.
// The arbiter consumes scan.Artifacts for evidence injection, quality score
// mapping, and resolved question seeding.
package scan

// Artifacts holds the lossless output from the kickoff codebase scan
// for the first three sprint phases (Vision, Problem, Users).
type Artifacts struct {
	Vision  *PhaseData
	Problem *PhaseData
	Users   *PhaseData
}

// PhaseData holds a single phase's scan results in a format the arbiter
// can consume directly.
type PhaseData struct {
	Summary           string
	Evidence          []EvidenceItem
	ResolvedQuestions []ResolvedQuestion
	Quality           QualityScores
}

// EvidenceItem is a codebase-grounded piece of evidence.
type EvidenceItem struct {
	Type       string  // e.g. "readme", "package", "code"
	FilePath   string
	Quote      string
	Confidence float64
}

// ResolvedQuestion is a question answered during the scan interview.
type ResolvedQuestion struct {
	Question string
	Answer   string
}

// QualityScores holds per-phase quality metrics from the scan.
type QualityScores struct {
	Clarity      float64
	Completeness float64
	Grounding    float64
	Consistency  float64
}

// HasEvidence returns true if any phase has at least one evidence item.
func (a *Artifacts) HasEvidence() bool {
	if a == nil {
		return false
	}
	for _, pd := range []*PhaseData{a.Vision, a.Problem, a.Users} {
		if pd != nil && len(pd.Evidence) > 0 {
			return true
		}
	}
	return false
}

// PhaseFor returns the PhaseData for the given phase name, or nil.
func (a *Artifacts) PhaseFor(phase string) *PhaseData {
	if a == nil {
		return nil
	}
	switch phase {
	case "Vision":
		return a.Vision
	case "Problem":
		return a.Problem
	case "Users":
		return a.Users
	default:
		return nil
	}
}
