package specs

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return res, err
	}
	if doc.ID == "" || doc.Title == "" || doc.Summary == "" {
		res.Errors = append(res.Errors, "missing required fields")
	}
	if doc.Status != "" && !validStatus(doc.Status) {
		res.Warnings = append(res.Warnings, "invalid status: "+doc.Status)
	}
	reqIDs := requirementIDs(doc.Requirements)
	validateCUJs(&res, doc.CriticalUserJourneys, reqIDs, opts.Mode)
	validateMarketResearch(&res, doc.MarketResearch, opts)
	validateCompetitiveLandscape(&res, doc.CompetitiveLandscape, opts)
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

func isResearchPath(path string) bool {
	if filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return false
	}
	prefix := filepath.Clean(filepath.Join(".praude", "research"))
	if clean == prefix {
		return true
	}
	return strings.HasPrefix(clean, prefix+string(os.PathSeparator))
}
