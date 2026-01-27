package specs

type StrategicContext struct {
	CUJID       string `yaml:"cuj_id"`
	CUJName     string `yaml:"cuj_name"`
	FeatureID   string `yaml:"feature_id"`
	MVPIncluded bool   `yaml:"mvp_included"`
}

type UserStory struct {
	Text string `yaml:"text"`
	Hash string `yaml:"hash"`
}

type AcceptanceCriterion struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
}

type FileChange struct {
	Action      string `yaml:"action"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type EvidenceRef struct {
	Path   string `yaml:"path"`
	Anchor string `yaml:"anchor"`
	Note   string `yaml:"note"`
}

type CriticalUserJourney struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Priority           string   `yaml:"priority"`
	Steps              []string `yaml:"steps"`
	SuccessCriteria    []string `yaml:"success_criteria"`
	LinkedRequirements []string `yaml:"linked_requirements"`
}

type MarketResearchItem struct {
	ID           string        `yaml:"id"`
	Claim        string        `yaml:"claim"`
	EvidenceRefs []EvidenceRef `yaml:"evidence_refs"`
	Confidence   string        `yaml:"confidence"`
	Date         string        `yaml:"date"`
}

type CompetitiveLandscapeItem struct {
	ID           string        `yaml:"id"`
	Name         string        `yaml:"name"`
	Positioning  string        `yaml:"positioning"`
	Strengths    []string      `yaml:"strengths"`
	Weaknesses   []string      `yaml:"weaknesses"`
	Risk         string        `yaml:"risk"`
	EvidenceRefs []EvidenceRef `yaml:"evidence_refs"`
}

type Metadata struct {
	ValidationWarnings []string `yaml:"validation_warnings"`
}

// Goal represents a measurable outcome the PRD aims to achieve.
// Goals should be specific, measurable, and time-bound where possible.
type Goal struct {
	ID          string `yaml:"id"`          // e.g., "GOAL-001"
	Description string `yaml:"description"` // What success looks like
	Metric      string `yaml:"metric"`      // How to measure (optional)
	Target      string `yaml:"target"`      // Target value (optional)
}

// NonGoal explicitly defines what is out of scope.
// Non-goals prevent scope creep and clarify boundaries.
type NonGoal struct {
	ID          string `yaml:"id"`          // e.g., "NG-001"
	Description string `yaml:"description"` // What we're NOT doing
	Rationale   string `yaml:"rationale"`   // Why it's out of scope
}

// Assumption represents a foundational belief the PRD relies on.
// Tracking assumptions enables early detection when they prove false.
// Confidence decays one level (high→medium→low) when age exceeds DecayDays
// without validation — checked on spec load, no background process.
type Assumption struct {
	ID            string `yaml:"id"`                          // e.g., "ASSM-001"
	Description   string `yaml:"description"`                // The assumption
	ImpactIfFalse string `yaml:"impact_if_false"`             // What breaks if wrong
	Confidence    string `yaml:"confidence"`                  // high, medium, low
	ValidatedAt   string `yaml:"validated_at,omitempty"`      // RFC3339 timestamp of last validation
	DecayDays     int    `yaml:"decay_days,omitempty"`        // default: 30
	LinkedInsight string `yaml:"linked_insight,omitempty"`    // Pollard insight ID
}

// Hypothesis represents a falsifiable outcome prediction tied to a feature.
// "If we build X, then Y will change by Z within T days."
type Hypothesis struct {
	ID          string `yaml:"id"`           // "HYP-001"
	FeatureRef  string `yaml:"feature_ref"`
	Statement   string `yaml:"statement"`    // "If we build X, then Y"
	Metric      string `yaml:"metric"`
	Baseline    string `yaml:"baseline"`
	Target      string `yaml:"target"`       // ">50 within 30 days"
	TimeboxDays int    `yaml:"timebox_days"`
	Status      string `yaml:"status"`       // "untested", "validated", "invalidated"
	Result      string `yaml:"result,omitempty"`
}

// Requirement represents a structured Given/When/Then requirement.
type Requirement struct {
	ID          string   `yaml:"id"`                       // "REQ-001"
	FeatureRef  string   `yaml:"feature_ref"`
	Type        string   `yaml:"type"`                     // "functional", "performance", "security"
	Given       string   `yaml:"given"`
	When        string   `yaml:"when"`
	Then        string   `yaml:"then"`
	Constraints []string `yaml:"constraints,omitempty"`    // "latency < 200ms"
	Status      string   `yaml:"status"`                   // "draft", "approved", "implemented"
}

// SpecType constants for the Type field.
const (
	SpecTypePRD    = "prd"
	SpecTypeVision = "vision"
)

type Spec struct {
	ID                   string                     `yaml:"id"`
	Title                string                     `yaml:"title"`
	Type                 string                     `yaml:"type,omitempty"` // "prd" (default), "vision"
	CreatedAt            string                     `yaml:"created_at"`
	Status               string                     `yaml:"status"`
	VisionRef            string                     `yaml:"vision_ref,omitempty"`          // ID of vision spec this PRD aligns to
	LastReviewedAt       string                     `yaml:"last_reviewed_at,omitempty"`    // RFC3339
	ReviewCadenceDays    int                        `yaml:"review_cadence_days,omitempty"` // default 30
	StrategicContext     StrategicContext           `yaml:"strategic_context"`
	UserStory            UserStory                  `yaml:"user_story"`
	Summary              string                     `yaml:"summary"`
	Goals                []Goal                     `yaml:"goals,omitempty"`
	NonGoals             []NonGoal                  `yaml:"non_goals,omitempty"`
	Assumptions          []Assumption               `yaml:"assumptions,omitempty"`
	Requirements         []string                   `yaml:"requirements"`
	Acceptance           []AcceptanceCriterion      `yaml:"acceptance_criteria"`
	FilesToModify        []FileChange               `yaml:"files_to_modify"`
	Research             []string                   `yaml:"research"`
	CriticalUserJourneys []CriticalUserJourney      `yaml:"critical_user_journeys"`
	MarketResearch       []MarketResearchItem       `yaml:"market_research"`
	CompetitiveLandscape []CompetitiveLandscapeItem `yaml:"competitive_landscape"`
	Hypotheses              []Hypothesis               `yaml:"hypotheses,omitempty"`
	StructuredRequirements  []Requirement              `yaml:"structured_requirements,omitempty"`
	Version                 int                        `yaml:"version,omitempty"`
	Metadata                Metadata                   `yaml:"metadata"`
	Complexity              string                     `yaml:"complexity"`
	EstimatedMinutes     int                        `yaml:"estimated_minutes"`
	Priority             int                        `yaml:"priority"`
	DismissedConflicts   []string                   `yaml:"dismissed_conflicts,omitempty"` // vision conflict IDs intentionally dismissed
}

// EffectiveType returns the spec type, defaulting to "prd" if empty.
func (s *Spec) EffectiveType() string {
	if s.Type == "" {
		return SpecTypePRD
	}
	return s.Type
}

// IsVision returns true if this is a vision-type spec.
func (s *Spec) IsVision() bool {
	return s.EffectiveType() == SpecTypeVision
}

// EffectiveReviewCadenceDays returns the review cadence, defaulting to 30.
func (s *Spec) EffectiveReviewCadenceDays() int {
	if s.ReviewCadenceDays <= 0 {
		return 30
	}
	return s.ReviewCadenceDays
}
