package specs

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// PRDStatus represents the status of a PRD
type PRDStatus string

const (
	PRDStatusDraft      PRDStatus = "draft"
	PRDStatusApproved   PRDStatus = "approved"
	PRDStatusInProgress PRDStatus = "in_progress"
	PRDStatusDone       PRDStatus = "done"
)

// FeatureStatus represents the status of a feature
type FeatureStatus string

const (
	FeatureStatusDraft      FeatureStatus = "draft"
	FeatureStatusApproved   FeatureStatus = "approved"
	FeatureStatusInProgress FeatureStatus = "in_progress"
	FeatureStatusDone       FeatureStatus = "done"
)

// Feature represents a major capability within a PRD
type Feature struct {
	ID                   string                `yaml:"id"`       // "FEAT-001"
	Title                string                `yaml:"title"`
	Status               FeatureStatus         `yaml:"status"`
	Summary              string                `yaml:"summary"`
	Requirements         []string              `yaml:"requirements"`
	AcceptanceCriteria   []AcceptanceCriterion `yaml:"acceptance_criteria"`
	FilesToModify        []FileChange          `yaml:"files_to_modify"`
	CriticalUserJourneys []CriticalUserJourney `yaml:"critical_user_journeys"`
	Complexity           string                `yaml:"complexity"` // low, medium, high
	Priority             int                   `yaml:"priority"`   // 0-4
}

// PRD represents a Product Requirements Document (version scope)
type PRD struct {
	ID        string       `yaml:"id"`         // "MVP", "V1", "V2"
	Title     string       `yaml:"title"`      // "Vauxhall MVP"
	Version   string       `yaml:"version"`    // "mvp", "v1", "v2"
	Status    PRDStatus    `yaml:"status"`
	CreatedAt string       `yaml:"created_at"`
	UpdatedAt string       `yaml:"updated_at,omitempty"`
	Features  []Feature    `yaml:"features"`
}

// LoadPRD reads a PRD from a YAML file
func LoadPRD(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var prd PRD
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, err
	}
	return &prd, nil
}

// Save writes a PRD to a YAML file
func (p *PRD) Save(projectPath string) error {
	specsDir := filepath.Join(projectPath, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		return err
	}

	p.UpdatedAt = time.Now().Format(time.RFC3339)

	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}

	filename := p.Version + ".yaml"
	return os.WriteFile(filepath.Join(specsDir, filename), data, 0644)
}

// LoadAllPRDs reads all PRDs from a project's .praude/specs directory
func LoadAllPRDs(projectPath string) ([]*PRD, error) {
	specsDir := filepath.Join(projectPath, ".praude", "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*PRD{}, nil
		}
		return nil, err
	}

	var prds []*PRD
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		prd, err := LoadPRD(filepath.Join(specsDir, entry.Name()))
		if err != nil {
			continue // Skip invalid files
		}
		// Only include files with the new PRD structure (has Version field)
		if prd.Version != "" {
			prds = append(prds, prd)
		}
	}
	return prds, nil
}

// GetFeature returns a feature by ID
func (p *PRD) GetFeature(id string) *Feature {
	for i := range p.Features {
		if p.Features[i].ID == id {
			return &p.Features[i]
		}
	}
	return nil
}

// GetApprovedFeatures returns features with approved or in_progress status
func (p *PRD) GetApprovedFeatures() []Feature {
	var approved []Feature
	for _, f := range p.Features {
		if f.Status == FeatureStatusApproved || f.Status == FeatureStatusInProgress {
			approved = append(approved, f)
		}
	}
	return approved
}

// CountByStatus returns counts of features by status
func (p *PRD) CountByStatus() map[FeatureStatus]int {
	counts := make(map[FeatureStatus]int)
	for _, f := range p.Features {
		counts[f.Status]++
	}
	return counts
}

// MigrateSpecToPRD converts a legacy Spec to a Feature within a PRD
func MigrateSpecToPRD(spec Spec, prdVersion string) Feature {
	return Feature{
		ID:                   spec.ID,
		Title:                spec.Title,
		Status:               FeatureStatus(spec.Status),
		Summary:              spec.Summary,
		Requirements:         spec.Requirements,
		AcceptanceCriteria:   spec.Acceptance,
		FilesToModify:        spec.FilesToModify,
		CriticalUserJourneys: spec.CriticalUserJourneys,
		Complexity:           spec.Complexity,
		Priority:             spec.Priority,
	}
}

// NewPRD creates a new PRD with default values
func NewPRD(version, title string) *PRD {
	return &PRD{
		ID:        version,
		Title:     title,
		Version:   version,
		Status:    PRDStatusDraft,
		CreatedAt: time.Now().Format(time.RFC3339),
		Features:  []Feature{},
	}
}

// AddFeature adds a new feature to the PRD
func (p *PRD) AddFeature(f Feature) {
	p.Features = append(p.Features, f)
}
