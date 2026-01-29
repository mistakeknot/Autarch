package agent

type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationResult struct {
	OK     bool              `json:"ok"`
	Errors []ValidationError `json:"errors"`
}

type EvidenceItem struct {
	Type       string  `json:"type"`
	Path       string  `json:"path"`
	Quote      string  `json:"quote"`
	Confidence float64 `json:"confidence"`
}

type QualityScores struct {
	Clarity      float64 `json:"clarity"`
	Completeness float64 `json:"completeness"`
	Grounding    float64 `json:"grounding"`
	Consistency  float64 `json:"consistency"`
}

type ScanArtifactBase struct {
	Phase         string         `json:"phase"`
	Version       string         `json:"version"`
	Evidence      []EvidenceItem `json:"evidence"`
	OpenQuestions []string       `json:"open_questions"`
	Quality       QualityScores  `json:"quality"`
	Assumptions   []string       `json:"assumptions,omitempty"`
}

type VisionArtifact struct {
	ScanArtifactBase
	Summary  string   `json:"summary"`
	Goals    []string `json:"goals"`
	NonGoals []string `json:"non_goals"`
}

type ProblemArtifact struct {
	ScanArtifactBase
	Summary    string   `json:"summary"`
	PainPoints []string `json:"pain_points"`
	Impact     string   `json:"impact"`
}

type UsersArtifact struct {
	ScanArtifactBase
	Personas []Persona `json:"personas"`
}

type Persona struct {
	Name    string   `json:"name"`
	Needs   []string `json:"needs"`
	Context string   `json:"context"`
}

type FeaturesArtifact struct {
	ScanArtifactBase
	Features []string `json:"features"`
	Outcomes []string `json:"outcomes"`
}

type RequirementsArtifact struct {
	ScanArtifactBase
	Requirements []string `json:"requirements"`
}

type ScopeArtifact struct {
	ScanArtifactBase
	InScope    []string `json:"in_scope"`
	OutOfScope []string `json:"out_of_scope"`
}

type CUJArtifact struct {
	ScanArtifactBase
	Journeys []Journey `json:"journeys"`
}

type Journey struct {
	Name    string   `json:"name"`
	Steps   []string `json:"steps"`
	Success string   `json:"success"`
}

type AcceptanceArtifact struct {
	ScanArtifactBase
	Criteria []string `json:"criteria"`
}

type SynthesisArtifact struct {
	Version          string            `json:"version"`
	Inputs           []string          `json:"inputs"`
	ConsistencyNotes []ConsistencyNote `json:"consistency_notes"`
	UpdatesSuggested []SynthesisUpdate `json:"updates_suggested"`
	Quality          SynthesisQuality  `json:"quality"`
}

type ConsistencyNote struct {
	Type string `json:"type"`
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

type SynthesisUpdate struct {
	Phase string `json:"phase"`
	Patch string `json:"patch"`
}

type SynthesisQuality struct {
	CrossPhaseAlignment float64 `json:"cross_phase_alignment"`
}

type EvidenceLookup interface {
	Exists(path string) bool
	ContainsQuote(path, quote string) bool
}
