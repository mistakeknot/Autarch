// Package insights handles research insights and findings.
package insights

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
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

// watchCompetitorFile is the YAML schema written by the competitor watch hunter.
type watchCompetitorFile struct {
	Competitor   string          `yaml:"competitor"`
	CollectedAt  time.Time       `yaml:"collected_at"`
	ChangelogURL string          `yaml:"changelog_url"`
	Changes      []watchChange   `yaml:"changes"`
}

type watchChange struct {
	Title          string           `yaml:"title"`
	URL            string           `yaml:"url,omitempty"`
	Relevance      string           `yaml:"relevance"`
	ThreatLevel    string           `yaml:"threat_level"`
	Recommendation *Recommendation  `yaml:"recommendation,omitempty"`
}

// watchTrendsFile is the YAML schema written by the HackerNews/trends hunter.
type watchTrendsFile struct {
	CollectedAt time.Time    `yaml:"collected_at"`
	Trends      []watchTrend `yaml:"trends"`
}

type watchTrend struct {
	Title     string    `yaml:"title"`
	Source    string    `yaml:"source"`
	URL       string    `yaml:"url"`
	Points    int       `yaml:"points"`
	Comments  int       `yaml:"comments"`
	Relevance string    `yaml:"relevance"`
	Signal    string    `yaml:"signal,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
}

// Load reads an insight from a YAML file. Handles both the native Insight
// format and watch/hunter formats (competitive, trends) by auto-detecting.
func Load(path string) (*Insight, error) {
	var insight Insight
	if _, err := yamlsafe.UnmarshalFile(path, &insight); err != nil {
		return nil, err
	}
	// Native format — has ID or title
	if insight.ID != "" || insight.Title != "" {
		return &insight, nil
	}
	// Try watch formats
	if i := tryLoadCompetitor(path); i != nil {
		return i, nil
	}
	if i := tryLoadTrends(path); i != nil {
		return i, nil
	}
	// Unknown format — skip
	return nil, fmt.Errorf("unrecognized insight format: %s", path)
}

func tryLoadCompetitor(path string) *Insight {
	var wf watchCompetitorFile
	if _, err := yamlsafe.UnmarshalFile(path, &wf); err != nil || wf.Competitor == "" {
		return nil
	}
	// Synthesize ID from filename
	base := strings.TrimSuffix(filepath.Base(path), ".yaml")
	insight := &Insight{
		ID:          base,
		Title:       wf.Competitor + " — competitive watch",
		Category:    CategoryCompetitive,
		CollectedAt: wf.CollectedAt,
		Sources:     []Source{{URL: wf.ChangelogURL, Type: "product"}},
	}
	for _, c := range wf.Changes {
		if c.Relevance == "low" {
			continue // Only surface high/medium findings
		}
		insight.Findings = append(insight.Findings, Finding{
			Title:     c.Title,
			Relevance: Relevance(c.Relevance),
		})
		if c.Recommendation != nil {
			insight.Recommendations = append(insight.Recommendations, *c.Recommendation)
		}
	}
	return insight
}

func tryLoadTrends(path string) *Insight {
	var wf watchTrendsFile
	if _, err := yamlsafe.UnmarshalFile(path, &wf); err != nil || len(wf.Trends) == 0 {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(path), ".yaml")
	insight := &Insight{
		ID:          base,
		Title:       "Trends — " + base,
		Category:    CategoryTrends,
		CollectedAt: wf.CollectedAt,
	}
	for _, t := range wf.Trends {
		if t.Relevance == "low" {
			continue
		}
		insight.Findings = append(insight.Findings, Finding{
			Title:       t.Title,
			Relevance:   Relevance(t.Relevance),
			Description: t.Signal,
		})
		insight.Sources = append(insight.Sources, Source{URL: t.URL, Type: t.Source})
	}
	return insight
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
	if _, err := os.Stat(insightsDir); os.IsNotExist(err) {
		return []*Insight{}, nil
	}

	var insights []*Insight
	err := filepath.WalkDir(insightsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
			return nil
		}
		insight, err := Load(path)
		if err != nil {
			return nil // Skip invalid files
		}
		insights = append(insights, insight)
		return nil
	})
	if err != nil {
		return nil, err
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
