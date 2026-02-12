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
	PhaseCUJs             // Moved up: user journeys flow from users + features
	PhaseRequirements     // Requirements derived from CUJs
	PhaseScopeAssumptions
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
		PhaseCUJs,             // User journeys flow from users + features
		PhaseRequirements,     // Requirements derived from CUJs
		PhaseScopeAssumptions,
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
		"Critical User Journeys",
		"Requirements",
		"Scope + Assumptions",
		"Acceptance Criteria",
	}
	if p >= 0 && int(p) < len(names) {
		return names[p]
	}
	return "Unknown"
}

// DefaultModelTiers returns the default per-phase model assignment.
//
// All phases use the CLI default (Opus) because every LLM call in the arbiter
// is substantive content generation — there are no routing-only calls. Non-LLM
// operations (consistency checks, confidence scoring, research scans) don't
// dispatch to a model at all.
//
// The ModelOverrides field on SprintState lets users trade quality for speed on
// specific phases during iteration (e.g., set Vision to Haiku for fast drafts).
func DefaultModelTiers() map[Phase]string {
	return map[Phase]string{}
}

// ModelForPhase returns the model to use for the given phase, checking
// overrides first, then returning empty string (CLI default) if none set.
func ModelForPhase(overrides map[Phase]string, phase Phase) string {
	if overrides != nil {
		if model, ok := overrides[phase]; ok {
			return model
		}
	}
	return ""
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
	Preamble      string      // LLM thinking preamble (not displayed to user)
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
	ModelOverrides  map[Phase]string            // per-phase model tier (empty = CLI default)
	ScanArtifacts     *scan.Artifacts             // lossless kickoff scan results (nil if no scan)
	ExplorationResult    map[string]any              // raw Claude Code exploration output (reused across phases)
	ExplorationSessionID string                     // Claude Code session for reuse in later phases
	StartedAt            time.Time
	UpdatedAt         time.Time
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

// --- Clone methods for concurrency-safe snapshots ---

// Clone returns a deep copy of the SectionDraft.
func (d *SectionDraft) Clone() SectionDraft {
	out := *d
	out.Options = append([]string(nil), d.Options...)
	out.ActiveSignals = append([]string(nil), d.ActiveSignals...)
	out.UserEdits = make([]Edit, len(d.UserEdits))
	copy(out.UserEdits, d.UserEdits)
	return out
}

// Clone returns a deep copy of the QuickScanResult.
func (q *QuickScanResult) Clone() QuickScanResult {
	out := *q
	out.GitHubHits = make([]GitHubFinding, len(q.GitHubHits))
	copy(out.GitHubHits, q.GitHubHits)
	out.HNHits = make([]HNFinding, len(q.HNHits))
	copy(out.HNHits, q.HNHits)
	return out
}

// Clone returns a deep copy of the VisionContext.
func (vc *VisionContext) Clone() VisionContext {
	out := *vc
	out.Goals = append([]string(nil), vc.Goals...)
	out.Assumptions = append([]string(nil), vc.Assumptions...)
	out.CUJs = append([]string(nil), vc.CUJs...)
	out.Hypotheses = append([]string(nil), vc.Hypotheses...)
	return out
}

// Clone returns a deep copy of the SprintState.
// ScanArtifacts is shared (immutable after creation) and not deep-copied.
func (s *SprintState) Clone() SprintState {
	out := *s

	// Deep copy Sections map
	out.Sections = make(map[Phase]*SectionDraft, len(s.Sections))
	for k, v := range s.Sections {
		c := v.Clone()
		out.Sections[k] = &c
	}

	// Deep copy slices with interior slices/structs
	out.Conflicts = cloneConflicts(s.Conflicts)
	out.Findings = cloneFindings(s.Findings)

	// Deep copy optional pointers
	if s.ResearchCtx != nil {
		rc := s.ResearchCtx.Clone()
		out.ResearchCtx = &rc
	}
	if s.VisionContext != nil {
		vc := s.VisionContext.Clone()
		out.VisionContext = &vc
	}

	// ShapeOverrides: map of int→int values, needs shallow map copy
	if s.ShapeOverrides != nil {
		out.ShapeOverrides = make(map[Phase]thinking.Shape, len(s.ShapeOverrides))
		for k, v := range s.ShapeOverrides {
			out.ShapeOverrides[k] = v
		}
	}

	// ModelOverrides: map of int→string values, needs shallow map copy
	if s.ModelOverrides != nil {
		out.ModelOverrides = make(map[Phase]string, len(s.ModelOverrides))
		for k, v := range s.ModelOverrides {
			out.ModelOverrides[k] = v
		}
	}

	// ScanArtifacts: shared pointer (immutable after creation), not cloned
	return out
}

func cloneConflicts(src []Conflict) []Conflict {
	if src == nil {
		return nil
	}
	out := make([]Conflict, len(src))
	for i, c := range src {
		out[i] = c
		out[i].Sections = append([]Phase(nil), c.Sections...)
	}
	return out
}

func cloneFindings(src []ResearchFinding) []ResearchFinding {
	if src == nil {
		return nil
	}
	out := make([]ResearchFinding, len(src))
	for i, f := range src {
		out[i] = f
		out[i].Tags = append([]string(nil), f.Tags...)
	}
	return out
}
