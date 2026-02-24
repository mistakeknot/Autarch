// Package plan provides shared types for the plan/apply pattern across Autarch tools.
package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Version is the current plan format version.
const Version = "1.0"

// Severity indicates how important a recommendation is.
type Severity string

const (
	SeverityError   Severity = "error"   // Must be addressed before applying
	SeverityWarning Severity = "warning" // Should be addressed but can proceed
	SeverityInfo    Severity = "info"    // Informational, no action required
)

// RecommendationType categorizes recommendations.
type RecommendationType string

const (
	TypePrereq      RecommendationType = "prereq"      // Something should be done first
	TypeMissing     RecommendationType = "missing"     // Required field is empty/weak
	TypeEnhancement RecommendationType = "enhancement" // Optional improvement
	TypeQuality     RecommendationType = "quality"     // Content quality issue
	TypeIntegration RecommendationType = "integration" // Cross-tool suggestion
	TypeValidation  RecommendationType = "validation"  // Schema/format warning
)

// Recommendation represents a suggestion or warning about the plan.
type Recommendation struct {
	Type        RecommendationType     `json:"type"`
	Severity    Severity               `json:"severity"`
	SourceTool  string                 `json:"source_tool,omitempty"`  // Tool that generated this (for cross-tool)
	Field       string                 `json:"field,omitempty"`        // Field this relates to
	Message     string                 `json:"message"`                // Human-readable description
	Suggestion  string                 `json:"suggestion,omitempty"`   // Actionable command/step
	Context     map[string]interface{} `json:"context,omitempty"`      // Additional data
	AutoFixable bool                   `json:"auto_fixable,omitempty"` // Can be fixed automatically
}

// Plan represents a generic plan that can be applied.
type Plan struct {
	Tool      string    `json:"tool"`       // praude, pollard, coldwine
	Action    string    `json:"action"`     // interview, scan, init, etc.
	Version   string    `json:"version"`    // Plan format version
	CreatedAt time.Time `json:"created_at"` // When the plan was generated
	Summary   string    `json:"summary"`    // Human-readable summary

	// Tool-specific data (varies by tool/action)
	Items json.RawMessage `json:"items,omitempty"`

	// Recommendations and validation
	Recommendations []Recommendation `json:"recommendations,omitempty"`
	Ready           bool             `json:"ready"` // Whether plan can be applied
}

// NewPlan creates a new plan with common fields initialized.
func NewPlan(tool, action string) *Plan {
	return &Plan{
		Tool:      tool,
		Action:    action,
		Version:   Version,
		CreatedAt: time.Now(),
		Ready:     true,
	}
}

// SetItems marshals the items into the plan.
func (p *Plan) SetItems(items interface{}) error {
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	p.Items = data
	return nil
}

// GetItems unmarshals the items from the plan.
func (p *Plan) GetItems(v interface{}) error {
	return json.Unmarshal(p.Items, v)
}

// AddRecommendation adds a recommendation to the plan.
func (p *Plan) AddRecommendation(r Recommendation) {
	p.Recommendations = append(p.Recommendations, r)
	// Mark as not ready if there's an error-level recommendation
	if r.Severity == SeverityError {
		p.Ready = false
	}
}

// HasErrors returns true if there are any error-level recommendations.
func (p *Plan) HasErrors() bool {
	for _, r := range p.Recommendations {
		if r.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if there are any warning-level recommendations.
func (p *Plan) HasWarnings() bool {
	for _, r := range p.Recommendations {
		if r.Severity == SeverityWarning {
			return true
		}
	}
	return false
}

// FilterBySeverity returns recommendations matching the given severity.
func (p *Plan) FilterBySeverity(severity Severity) []Recommendation {
	var filtered []Recommendation
	for _, r := range p.Recommendations {
		if r.Severity == severity {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterByType returns recommendations matching the given type.
func (p *Plan) FilterByType(t RecommendationType) []Recommendation {
	var filtered []Recommendation
	for _, r := range p.Recommendations {
		if r.Type == t {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Save writes the plan to the pending directory.
func (p *Plan) Save(projectRoot string) (string, error) {
	pendingDir := PendingDir(projectRoot, p.Tool)
	if err := os.MkdirAll(pendingDir, 0755); err != nil {
		return "", err
	}

	filename := p.Action + "-plan.json"
	path := filepath.Join(pendingDir, filename)

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}

// Load reads a plan from a file.
func Load(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	return &p, nil
}

// LoadPending loads the pending plan for a tool/action.
func LoadPending(projectRoot, tool, action string) (*Plan, error) {
	pendingDir := PendingDir(projectRoot, tool)
	filename := action + "-plan.json"
	return Load(filepath.Join(pendingDir, filename))
}

// PendingDir returns the pending plans directory for a tool.
func PendingDir(projectRoot, tool string) string {
	return filepath.Join(projectRoot, "."+tool, "pending")
}

// ClearPending removes the pending plan file after applying.
func ClearPending(projectRoot, tool, action string) error {
	pendingDir := PendingDir(projectRoot, tool)
	filename := action + "-plan.json"
	path := filepath.Join(pendingDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already cleared
	}
	return os.Remove(path)
}

// ListPending returns all pending plan files for a tool.
func ListPending(projectRoot, tool string) ([]string, error) {
	pendingDir := PendingDir(projectRoot, tool)
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var plans []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			plans = append(plans, filepath.Join(pendingDir, entry.Name()))
		}
	}
	return plans, nil
}
