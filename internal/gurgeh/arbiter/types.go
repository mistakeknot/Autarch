package arbiter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/pkg/thinking"
)

// Phase represents a section of the PRD sprint
type Phase int

const (
	PhaseVision Phase = iota
	PhaseProblem
	PhaseUsers
	PhaseFeaturesGoals
	PhaseRequirements
	PhaseScopeAssumptions
	PhaseCUJs
	PhaseAcceptanceCriteria
)

// PhaseCount is the total number of sprint phases.
const PhaseCount = 8

// AllPhases returns phases in order
func AllPhases() []Phase {
	return []Phase{
		PhaseVision,
		PhaseProblem,
		PhaseUsers,
		PhaseFeaturesGoals,
		PhaseRequirements,
		PhaseScopeAssumptions,
		PhaseCUJs,
		PhaseAcceptanceCriteria,
	}
}

// String returns the display name for a phase
func (p Phase) String() string {
	names := []string{
		"Vision",
		"Problem",
		"Users",
		"Features + Goals",
		"Requirements",
		"Scope + Assumptions",
		"Critical User Journeys",
		"Acceptance Criteria",
	}
	if p >= 0 && int(p) < len(names) {
		return names[p]
	}
	return "Unknown"
}

// DraftStatus tracks the state of a section draft
type DraftStatus int

const (
	DraftPending DraftStatus = iota
	DraftProposed
	DraftAccepted
	DraftNeedsRevision
)

// SectionDraft holds Arbiter's proposal for a section
type SectionDraft struct {
	Content       string      // Arbiter's current proposal
	Options       []string    // Alternative phrasings (2-3 options)
	Status        DraftStatus
	AutoAccept    bool        // true = no signals/decay, skip in review
	ActiveSignals []string    // signal IDs relevant to this section
	UserEdits     []Edit      // History of user changes
	UpdatedAt     time.Time
}

// Edit records a user modification
type Edit struct {
	Before    string
	After     string
	Reason    string    // Optional: why the user changed it
	Timestamp time.Time
}

// ConfidenceScore tracks PRD quality metrics
type ConfidenceScore struct {
	Completeness float64 // 0-1, weight: 20%
	Consistency  float64 // 0-1, weight: 25%
	Specificity  float64 // 0-1, weight: 20%
	Research     float64 // 0-1, weight: 20%
	Assumptions  float64 // 0-1, weight: 15%
}

// Total returns the weighted confidence score
func (c ConfidenceScore) Total() float64 {
	return c.Completeness*0.20 +
		c.Consistency*0.25 +
		c.Specificity*0.20 +
		c.Research*0.20 +
		c.Assumptions*0.15
}

// VisionContext holds a loaded vision spec for vertical consistency checks.
type VisionContext struct {
	SpecID      string
	Goals       []string // vision principles
	Assumptions []string // strategic bets
	CUJs        []string // key workflows
	Hypotheses  []string // predictions
}

// SprintState holds the full state of a PRD sprint session
type SprintState struct {
	ID              string
	SpecID          string // Intermute Spec ID (empty if no research provider)
	ProjectPath     string
	Phase           Phase
	Sections        map[Phase]*SectionDraft
	Conflicts       []Conflict
	Confidence      ConfidenceScore
	ResearchCtx     *QuickScanResult
	Findings        []ResearchFinding // Intermute research findings
	DeepScan        DeepScanState     // Async deep scan tracking
	VisionContext   *VisionContext    // loaded vision spec for vertical checks (nil if none)
	SpecType        string            // "" for PRD, "vision" for vision specs
	IsReview        bool                        // true when reviewing an existing spec
	ReviewingSpecID string                      // ID of spec being reviewed
	ShapeOverrides  map[Phase]thinking.Shape    // per-sprint user overrides for thinking shapes
	ScanArtifacts   *scan.Artifacts             // lossless kickoff scan results (nil if no scan)
	StartedAt       time.Time
	UpdatedAt       time.Time
}

// NewSprintState creates a new sprint with all sections initialized.
// It generates a unique 32-character hex ID using crypto/rand.
func NewSprintState(projectPath string) *SprintState {
	sections := make(map[Phase]*SectionDraft)
	for _, p := range AllPhases() {
		sections[p] = &SectionDraft{
			Status: DraftPending,
		}
	}

	return &SprintState{
		ID:          generateID(),
		ProjectPath: projectPath,
		Phase:       PhaseVision,
		Sections:    sections,
		Conflicts:   []Conflict{},
		StartedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// generateID returns a 32-character hex string from 16 random bytes.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// QuickScanResult holds Ranger's research findings
type QuickScanResult struct {
	Topic      string
	GitHubHits []GitHubFinding
	HNHits     []HNFinding
	Summary    string
	ScannedAt  time.Time
}

// GitHubFinding represents a relevant repository
type GitHubFinding struct {
	Name        string
	Description string
	Stars       int
	URL         string
}

// HNFinding represents a relevant HN discussion
type HNFinding struct {
	Title    string
	Points   int
	Comments int
	URL      string
	Theme    string // Extracted theme from discussion
}

// ResearchFinding represents a research insight from Intermute.
type ResearchFinding struct {
	ID         string
	Title      string
	Summary    string
	Source     string   // URL
	SourceType string   // "github", "hackernews", "arxiv", etc.
	Relevance  float64  // 0.0-1.0
	Tags       []string
}

// DeepScanStatus tracks the state of an async deep scan.
type DeepScanStatus int

const (
	DeepScanNone       DeepScanStatus = iota // No deep scan requested
	DeepScanRunning                          // Scan in progress
	DeepScanComplete                         // Results ready to import
	DeepScanFailed                           // Scan encountered an error
)

// DeepScanState holds the tracking info for an async deep scan.
type DeepScanState struct {
	Status    DeepScanStatus
	ScanID    string // Intermute scan job ID
	StartedAt time.Time
	Error     string // Non-empty if DeepScanFailed
}

// QuickScanner performs a fast research scan and returns findings.
// The default stub returns placeholder text; the real implementation
// in internal/pollard/quick runs GitHub Scout + HackerNews hunters.
type QuickScanner interface {
	Scan(ctx context.Context, topic string, projectPath string) (*QuickScanResult, error)
}

// PriorArtResult aggregates deep research findings for a spec phase.
type PriorArtResult struct {
	SimilarProjects []SimilarProject
	AcademicPapers  []AcademicPaper
	FeasibilityNote string
}

// SimilarProject is a discovered open-source project relevant to the spec.
type SimilarProject struct {
	Name, URL, Architecture string
	Stars                   int
	Strengths, Gaps         []string
}

// AcademicPaper is a research paper relevant to the spec's problem domain.
type AcademicPaper struct {
	Title, URL, Abstract string
	Year                 int
	Relevance            float64
}

// Conflict represents a consistency issue between sections
type Conflict struct {
	Type     ConflictType
	Severity Severity
	Message  string
	Sections []Phase // Which sections are in conflict
}

// ConflictType categorizes consistency issues
type ConflictType int

const (
	ConflictUserFeature ConflictType = iota // Feature doesn't match target users
	ConflictGoalFeature                     // Goal not supported by features
	ConflictScopeCreep                      // Feature contradicts non-goals
	ConflictAssumption                      // Assumption conflicts with other content
	ConflictVisionAlignment                 // PRD section misaligned with vision spec
)

// Severity indicates if the conflict blocks progress
type Severity int

const (
	SeverityBlocker Severity = iota // Must resolve before continuing
	SeverityWarning                 // Can dismiss with acknowledgment
)
