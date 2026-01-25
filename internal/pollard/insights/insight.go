// Package insights handles research insights and findings.
package insights

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Category represents the type of insight
type Category string

const (
	CategoryCompetitive Category = "competitive"
	CategoryTrends      Category = "trends"
	CategoryUser        Category = "user"
)

// Relevance indicates how relevant a finding is
type Relevance string

const (
	RelevanceHigh   Relevance = "high"
	RelevanceMedium Relevance = "medium"
	RelevanceLow    Relevance = "low"
)

// Source represents where the insight was gathered from
type Source struct {
	URL  string `yaml:"url"`
	Type string `yaml:"type"` // product, github, article, docs
}

// Finding represents a specific discovery within an insight
type Finding struct {
	Title       string    `yaml:"title"`
	Relevance   Relevance `yaml:"relevance"`
	Description string    `yaml:"description"`
	Evidence    []string  `yaml:"evidence,omitempty"` // screenshots, links
}

// Recommendation suggests how to apply the insight
type Recommendation struct {
	FeatureHint string `yaml:"feature_hint"`
	Priority    string `yaml:"priority"` // p0, p1, p2, p3
	Rationale   string `yaml:"rationale"`
}

// Insight represents a research finding
type Insight struct {
	ID              string           `yaml:"id"`
	Title           string           `yaml:"title"`
	Category        Category         `yaml:"category"`
	CollectedAt     time.Time        `yaml:"collected_at"`
	Sources         []Source         `yaml:"sources"`
	Findings        []Finding        `yaml:"findings"`
	Recommendations []Recommendation `yaml:"recommendations,omitempty"`
	LinkedFeatures  []string         `yaml:"linked_features,omitempty"` // FEAT-001, FEAT-002 (Gurgeh spec IDs)
	InitiativeRef   string           `yaml:"initiative_ref,omitempty"`  // Link to Initiative ID
	LinkedBy        string           `yaml:"linked_by,omitempty"`       // Agent or user who created the link
	LinkedAt        *time.Time       `yaml:"linked_at,omitempty"`       // When the link was created
}

// LinkToInitiative sets the initiative reference with metadata
func (i *Insight) LinkToInitiative(initiativeID, linkedBy string) {
	i.InitiativeRef = initiativeID
	i.LinkedBy = linkedBy
	now := time.Now()
	i.LinkedAt = &now
}

// Load reads an insight from a YAML file
func Load(path string) (*Insight, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var insight Insight
	if err := yaml.Unmarshal(data, &insight); err != nil {
		return nil, err
	}
	return &insight, nil
}

// Save writes an insight to a YAML file
func (i *Insight) Save(projectPath string) error {
	insightsDir := filepath.Join(projectPath, ".pollard", "insights")
	if err := os.MkdirAll(insightsDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(i)
	if err != nil {
		return err
	}

	filename := i.ID + ".yaml"
	return os.WriteFile(filepath.Join(insightsDir, filename), data, 0644)
}

// LoadAll reads all insights from a project's .pollard/insights directory
func LoadAll(projectPath string) ([]*Insight, error) {
	insightsDir := filepath.Join(projectPath, ".pollard", "insights")
	entries, err := os.ReadDir(insightsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Insight{}, nil
		}
		return nil, err
	}

	var insights []*Insight
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		insight, err := Load(filepath.Join(insightsDir, entry.Name()))
		if err != nil {
			continue // Skip invalid files
		}
		insights = append(insights, insight)
	}
	return insights, nil
}

// FilterByCategory returns insights matching the given category
func FilterByCategory(insights []*Insight, category Category) []*Insight {
	var filtered []*Insight
	for _, i := range insights {
		if i.Category == category {
			filtered = append(filtered, i)
		}
	}
	return filtered
}
