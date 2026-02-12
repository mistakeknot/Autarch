package specs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

type ValidationMode string

const (
	ValidationHard ValidationMode = "hard"
	ValidationSoft ValidationMode = "soft"
)

type ValidationOptions struct {
	Mode ValidationMode
	Root string
}

type ValidationResult struct {
	Errors   []string
	Warnings []string
}

func Validate(raw []byte, opts ValidationOptions) (ValidationResult, error) {
	res := ValidationResult{}
	if opts.Mode == "" {
		opts.Mode = ValidationSoft
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	var doc Spec
	if err := yamlsafe.Decode(raw, &doc); err != nil {
		return res, err
	}
	if doc.ID == "" || doc.Title == "" || doc.Summary == "" {
		res.Errors = append(res.Errors, "missing required fields")
	}
	if doc.Status != "" && !validStatus(doc.Status) {
		res.Warnings = append(res.Warnings, "invalid status: "+doc.Status)
	}
	if doc.Type != "" && !validSpecType(doc.Type) {
		res.Errors = append(res.Errors, "invalid spec type: "+doc.Type)
	}

	// Vision specs skip PRD-specific validations (CUJs, market research, etc.)
	if doc.EffectiveType() == SpecTypeVision {
		return res, nil
	}

	reqIDs := requirementIDs(doc.Requirements)
	validateCUJs(&res, doc.CriticalUserJourneys, reqIDs, opts.Mode)
	validateMarketResearch(&res, doc.MarketResearch, opts)
	validateCompetitiveLandscape(&res, doc.CompetitiveLandscape, opts)
	validateAcceptanceCriteria(&res, doc.Acceptance, opts.Mode)
	validateFilesToModify(&res, doc.FilesToModify, opts.Mode)
	validateComplexityPriority(&res, doc.Complexity, doc.Priority, opts.Mode)
	validateGoals(&res, doc.Goals, opts.Mode)
	validateNonGoals(&res, doc.NonGoals, opts.Mode)
	validateStructuredRequirements(&res, doc.StructuredRequirements, opts.Mode)
	validateHypotheses(&res, doc.Hypotheses, opts.Mode)
	validateAssumptions(&res, doc.Assumptions, opts.Mode)
	validateUserStoryHash(&res, doc.UserStory, opts.Mode)
	validateVisionRef(&res, doc.VisionRef, opts)
	return res, nil
}

func validateCUJs(res *ValidationResult, cujs []CriticalUserJourney, reqIDs map[string]struct{}, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, cuj := range cujs {
		if cuj.ID == "" {
			res.Errors = append(res.Errors, "cuj id is required")
		} else {
			if _, ok := seen[cuj.ID]; ok {
				res.Errors = append(res.Errors, "duplicate cuj id: "+cuj.ID)
			}
			seen[cuj.ID] = struct{}{}
		}
		if !validCUJPriority(cuj.Priority) {
			res.Errors = append(res.Errors, "invalid cuj priority: "+cuj.Priority)
		}
		if len(cuj.LinkedRequirements) == 0 {
			addModeIssue(res, mode, "cuj missing linked requirements: "+cuj.ID)
			continue
		}
		for _, link := range cuj.LinkedRequirements {
			if _, ok := reqIDs[link]; !ok {
				addModeIssue(res, mode, "cuj linked requirement not found: "+link)
			}
		}
	}
}

func validateMarketResearch(res *ValidationResult, items []MarketResearchItem, opts ValidationOptions) {
	if len(items) == 0 {
		res.Warnings = append(res.Warnings, "market research missing")
		return
	}
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.ID == "" {
			res.Errors = append(res.Errors, "market research id is required")
		} else {
			if _, ok := seen[item.ID]; ok {
				res.Errors = append(res.Errors, "duplicate market research id: "+item.ID)
			}
			seen[item.ID] = struct{}{}
		}
		validateEvidenceRefs(res, item.EvidenceRefs, opts, "market_research")
	}
}

func validateCompetitiveLandscape(res *ValidationResult, items []CompetitiveLandscapeItem, opts ValidationOptions) {
	if len(items) == 0 {
		res.Warnings = append(res.Warnings, "competitive landscape missing")
		return
	}
	seen := make(map[string]struct{})
	for _, item := range items {
		if item.ID == "" {
			res.Errors = append(res.Errors, "competitive landscape id is required")
		} else {
			if _, ok := seen[item.ID]; ok {
				res.Errors = append(res.Errors, "duplicate competitive landscape id: "+item.ID)
			}
			seen[item.ID] = struct{}{}
		}
		validateEvidenceRefs(res, item.EvidenceRefs, opts, "competitive_landscape")
	}
}

func validateEvidenceRefs(res *ValidationResult, refs []EvidenceRef, opts ValidationOptions, section string) {
	if len(refs) == 0 {
		addModeIssue(res, opts.Mode, section+" missing evidence refs")
		return
	}
	for _, ref := range refs {
		if ref.Path == "" {
			addModeIssue(res, opts.Mode, section+" evidence ref missing path")
			continue
		}
		if !isResearchPath(ref.Path) {
			addModeIssue(res, opts.Mode, section+" evidence ref outside research dir: "+ref.Path)
			continue
		}
		full := filepath.Join(opts.Root, filepath.Clean(ref.Path))
		if _, err := os.Stat(full); err != nil {
			addModeIssue(res, opts.Mode, section+" evidence ref missing file: "+ref.Path)
		}
	}
}

func addModeIssue(res *ValidationResult, mode ValidationMode, msg string) {
	if mode == ValidationHard {
		res.Errors = append(res.Errors, msg)
		return
	}
	res.Warnings = append(res.Warnings, msg)
}

func validCUJPriority(priority string) bool {
	switch strings.ToLower(priority) {
	case "critical", "high", "med", "low":
		return true
	default:
		return false
	}
}

func validSpecType(t string) bool {
	switch strings.ToLower(t) {
	case SpecTypePRD, SpecTypeVision:
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "interview", "draft", "research", "suggestions", "validated", "archived":
		return true
	default:
		return false
	}
}

func requirementIDs(requirements []string) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, req := range requirements {
		fields := strings.Fields(req)
		if len(fields) == 0 {
			continue
		}
		token := strings.TrimSuffix(fields[0], ":")
		if strings.HasPrefix(token, "REQ-") {
			ids[token] = struct{}{}
		}
	}
	return ids
}

func validateAcceptanceCriteria(res *ValidationResult, items []AcceptanceCriterion, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, ac := range items {
		if ac.ID == "" {
			res.Errors = append(res.Errors, "acceptance criterion id is required")
		} else {
			if _, ok := seen[ac.ID]; ok {
				res.Errors = append(res.Errors, "duplicate acceptance criterion id: "+ac.ID)
			}
			seen[ac.ID] = struct{}{}
		}
	}
}

func validateFilesToModify(res *ValidationResult, items []FileChange, mode ValidationMode) {
	for _, fc := range items {
		if fc.Path == "" {
			res.Errors = append(res.Errors, "file change path is required")
		}
		if fc.Action != "" && !validFileAction(fc.Action) {
			res.Errors = append(res.Errors, "invalid file action: "+fc.Action)
		}
	}
}

func validFileAction(action string) bool {
	switch strings.ToLower(action) {
	case "create", "modify", "delete":
		return true
	default:
		return false
	}
}

func validateComplexityPriority(res *ValidationResult, complexity string, priority int, mode ValidationMode) {
	if complexity != "" && !validComplexity(complexity) {
		res.Errors = append(res.Errors, "invalid complexity: "+complexity)
	}
	if priority < 0 || priority > 4 {
		addModeIssue(res, mode, "priority out of range 0-4")
	}
}

func validComplexity(c string) bool {
	switch strings.ToLower(c) {
	case "trivial", "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func validateGoals(res *ValidationResult, items []Goal, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, g := range items {
		if g.ID == "" {
			res.Errors = append(res.Errors, "goal id is required")
		} else {
			if _, ok := seen[g.ID]; ok {
				res.Errors = append(res.Errors, "duplicate goal id: "+g.ID)
			}
			seen[g.ID] = struct{}{}
		}
		if g.Description == "" {
			addModeIssue(res, mode, "goal missing description: "+g.ID)
		}
	}
}

func validateNonGoals(res *ValidationResult, items []NonGoal, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, ng := range items {
		if ng.ID == "" {
			res.Errors = append(res.Errors, "non-goal id is required")
		} else {
			if _, ok := seen[ng.ID]; ok {
				res.Errors = append(res.Errors, "duplicate non-goal id: "+ng.ID)
			}
			seen[ng.ID] = struct{}{}
		}
	}
}

func validateStructuredRequirements(res *ValidationResult, items []Requirement, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, r := range items {
		if r.ID == "" {
			res.Errors = append(res.Errors, "structured requirement id is required")
		} else {
			if _, ok := seen[r.ID]; ok {
				res.Errors = append(res.Errors, "duplicate structured requirement id: "+r.ID)
			}
			seen[r.ID] = struct{}{}
		}
		if r.Type != "" && !validRequirementType(r.Type) {
			res.Errors = append(res.Errors, "invalid requirement type: "+r.Type)
		}
		if r.Given == "" && r.When == "" && r.Then == "" {
			addModeIssue(res, mode, "structured requirement has empty GWT: "+r.ID)
		}
		if r.Status != "" && !validRequirementStatus(r.Status) {
			addModeIssue(res, mode, "invalid requirement status: "+r.Status)
		}
	}
}

func validRequirementType(t string) bool {
	switch strings.ToLower(t) {
	case "functional", "performance", "security":
		return true
	default:
		return false
	}
}

func validRequirementStatus(s string) bool {
	switch strings.ToLower(s) {
	case "draft", "approved", "implemented":
		return true
	default:
		return false
	}
}

func validateHypotheses(res *ValidationResult, items []Hypothesis, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, h := range items {
		if h.ID == "" {
			res.Errors = append(res.Errors, "hypothesis id is required")
		} else {
			if _, ok := seen[h.ID]; ok {
				res.Errors = append(res.Errors, "duplicate hypothesis id: "+h.ID)
			}
			seen[h.ID] = struct{}{}
		}
		if h.Statement == "" {
			addModeIssue(res, mode, "hypothesis missing statement: "+h.ID)
		}
		if h.Status != "" && !validHypothesisStatus(h.Status) {
			res.Errors = append(res.Errors, "invalid hypothesis status: "+h.Status)
		}
	}
}

func validHypothesisStatus(s string) bool {
	switch strings.ToLower(s) {
	case "untested", "validated", "invalidated":
		return true
	default:
		return false
	}
}

func validateAssumptions(res *ValidationResult, items []Assumption, mode ValidationMode) {
	seen := make(map[string]struct{})
	for _, a := range items {
		if a.ID == "" {
			res.Errors = append(res.Errors, "assumption id is required")
		} else {
			if _, ok := seen[a.ID]; ok {
				res.Errors = append(res.Errors, "duplicate assumption id: "+a.ID)
			}
			seen[a.ID] = struct{}{}
		}
		if a.Description == "" {
			addModeIssue(res, mode, "assumption missing description: "+a.ID)
		}
		if a.Confidence != "" && !validAssumptionConfidence(a.Confidence) {
			res.Errors = append(res.Errors, "invalid assumption confidence: "+a.Confidence)
		}
		if a.DecayDays < 0 {
			addModeIssue(res, mode, "assumption decay_days is negative: "+a.ID)
		}
	}
}

func validAssumptionConfidence(c string) bool {
	switch strings.ToLower(c) {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}

func validateUserStoryHash(res *ValidationResult, us UserStory, mode ValidationMode) {
	if us.Text != "" && us.Hash == "" {
		addModeIssue(res, mode, "user story has text but no hash")
	}
}

func validateVisionRef(res *ValidationResult, visionRef string, opts ValidationOptions) {
	if visionRef == "" {
		return
	}
	for _, dir := range []string{gurgDir, legacyPraudeDir} {
		specPath := filepath.Join(opts.Root, dir, "specs", visionRef+".yaml")
		if _, err := os.Stat(specPath); err == nil {
			return
		}
	}
	res.Warnings = append(res.Warnings, "vision_ref not found: "+visionRef)
}

func isResearchPath(path string) bool {
	if filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return false
	}
	for _, dir := range []string{gurgDir, legacyPraudeDir} {
		prefix := filepath.Clean(filepath.Join(dir, "research"))
		if clean == prefix || strings.HasPrefix(clean, prefix+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
