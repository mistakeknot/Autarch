// Package sources handles raw collected data from research agents.
package sources

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Type represents the source type
type Type string

const (
	TypeGitHub      Type = "github"
	TypeArticle     Type = "article"
	TypeProduct     Type = "product"
	TypeScreenshot  Type = "screenshot"
	TypeHackerNews  Type = "hackernews"
	TypeProductHunt Type = "producthunt"
	TypeArxiv       Type = "arxiv"
	TypeCompetitor  Type = "competitor"
	TypeOpenAlex    Type = "openalex"
	TypePubMed      Type = "pubmed"
	TypeNutrition   Type = "nutrition"
	TypeLegal       Type = "legal"
	TypeEconomics   Type = "economics"
	TypeWiki        Type = "wiki"
)

// GitHubRepo represents a GitHub repository source
type GitHubRepo struct {
	Owner       string    `yaml:"owner"`
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	URL         string    `yaml:"url"`
	Stars       int       `yaml:"stars"`
	Language    string    `yaml:"language"`
	Topics      []string  `yaml:"topics"`
	UpdatedAt   time.Time `yaml:"updated_at"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// Article represents an article or blog post source
type Article struct {
	Title       string    `yaml:"title"`
	URL         string    `yaml:"url"`
	Author      string    `yaml:"author,omitempty"`
	PublishedAt time.Time `yaml:"published_at,omitempty"`
	Summary     string    `yaml:"summary,omitempty"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// Screenshot represents a captured screenshot
type Screenshot struct {
	Filename    string    `yaml:"filename"`
	URL         string    `yaml:"url"` // Source URL
	Description string    `yaml:"description,omitempty"`
	CapturedAt  time.Time `yaml:"captured_at"`
}

// ResearchPaper represents an academic paper from arXiv or Semantic Scholar
type ResearchPaper struct {
	ArxivID     string    `yaml:"arxiv_id"`
	Title       string    `yaml:"title"`
	Authors     []string  `yaml:"authors"`
	Abstract    string    `yaml:"abstract"`
	URL         string    `yaml:"url"`
	PDFURL      string    `yaml:"pdf_url,omitempty"`
	Published   time.Time `yaml:"published"`
	Categories  []string  `yaml:"categories"`
	Citations   int       `yaml:"citations,omitempty"`
	Relevance   string    `yaml:"relevance"` // high, medium, low
	HasCode     bool      `yaml:"has_code,omitempty"`
	CodeURL     string    `yaml:"code_url,omitempty"`
	Signal      string    `yaml:"signal,omitempty"` // Brief note on why this matters
	CollectedAt time.Time `yaml:"collected_at"`
}

// TrendItem represents a trending discussion from HackerNews or similar
type TrendItem struct {
	Title       string    `yaml:"title"`
	Source      string    `yaml:"source"` // hackernews, reddit, producthunt
	URL         string    `yaml:"url"`
	SourceURL   string    `yaml:"source_url"` // HN/Reddit discussion URL
	Points      int       `yaml:"points"`
	Comments    int       `yaml:"comments"`
	Author      string    `yaml:"author,omitempty"`
	CreatedAt   time.Time `yaml:"created_at"`
	Relevance   string    `yaml:"relevance"` // high, medium, low
	Signal      string    `yaml:"signal,omitempty"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// CompetitorChange represents a detected change from a competitor
type CompetitorChange struct {
	Competitor  string    `yaml:"competitor"`
	Date        time.Time `yaml:"date,omitempty"`
	Title       string    `yaml:"title"`
	Description string    `yaml:"description,omitempty"`
	URL         string    `yaml:"url,omitempty"`
	Relevance   string    `yaml:"relevance"` // high, medium, low
	ThreatLevel string    `yaml:"threat_level,omitempty"` // high, medium, low
	Recommendation *CompetitorRecommendation `yaml:"recommendation,omitempty"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// CompetitorRecommendation suggests action based on competitor change
type CompetitorRecommendation struct {
	FeatureHint string `yaml:"feature_hint"`
	Priority    string `yaml:"priority"` // p0, p1, p2, p3
	Rationale   string `yaml:"rationale"`
}

// AcademicWork represents an academic paper from OpenAlex/CrossRef
type AcademicWork struct {
	ID          string    `yaml:"id"`
	DOI         string    `yaml:"doi,omitempty"`
	Title       string    `yaml:"title"`
	Authors     []string  `yaml:"authors"`
	Abstract    string    `yaml:"abstract,omitempty"`
	PublishedAt time.Time `yaml:"published_at"`
	Journal     string    `yaml:"journal,omitempty"`
	Citations   int       `yaml:"citations"`
	OpenAccess  bool      `yaml:"open_access"`
	URL         string    `yaml:"url"`
	PDFURL      string    `yaml:"pdf_url,omitempty"`
	Topics      []string  `yaml:"topics,omitempty"`
	Relevance   string    `yaml:"relevance"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// MedicalArticle represents a medical/biomedical article from PubMed
type MedicalArticle struct {
	PMID        string    `yaml:"pmid"`
	Title       string    `yaml:"title"`
	Authors     []string  `yaml:"authors"`
	Abstract    string    `yaml:"abstract"`
	Journal     string    `yaml:"journal"`
	PublishedAt time.Time `yaml:"published_at"`
	MeSHTerms   []string  `yaml:"mesh_terms"`
	Keywords    []string  `yaml:"keywords"`
	URL         string    `yaml:"url"`
	FullTextURL string    `yaml:"full_text_url,omitempty"`
	Relevance   string    `yaml:"relevance"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// Nutrient represents a nutrient value in food
type Nutrient struct {
	Name   string  `yaml:"name"`
	Amount float64 `yaml:"amount"`
	Unit   string  `yaml:"unit"`
}

// FoodItem represents a food/nutrition item from USDA FoodData Central
type FoodItem struct {
	FDCID       int       `yaml:"fdc_id"`
	Description string    `yaml:"description"`
	DataType    string    `yaml:"data_type"`
	BrandOwner  string    `yaml:"brand_owner,omitempty"`
	Nutrients   []Nutrient `yaml:"nutrients,omitempty"`
	Allergens   []string  `yaml:"allergens,omitempty"`
	Ingredients string    `yaml:"ingredients,omitempty"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// CourtCase represents a court decision from CourtListener
type CourtCase struct {
	ID          string    `yaml:"id"`
	CaseName    string    `yaml:"case_name"`
	Court       string    `yaml:"court"`
	DateFiled   time.Time `yaml:"date_filed"`
	Docket      string    `yaml:"docket,omitempty"`
	Citation    string    `yaml:"citation,omitempty"`
	Summary     string    `yaml:"summary,omitempty"`
	URL         string    `yaml:"url"`
	Relevance   string    `yaml:"relevance"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// EconomicIndicator represents an economic indicator from OECD/World Bank
type EconomicIndicator struct {
	Indicator   string    `yaml:"indicator"`
	Country     string    `yaml:"country"`
	Value       float64   `yaml:"value"`
	Unit        string    `yaml:"unit"`
	Period      string    `yaml:"period"`
	Source      string    `yaml:"source"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// WikiEntity represents an entity from Wikidata/Wikipedia
type WikiEntity struct {
	QID          string            `yaml:"qid,omitempty"`
	Title        string            `yaml:"title"`
	Description  string            `yaml:"description"`
	URL          string            `yaml:"url"`
	WikipediaURL string            `yaml:"wikipedia_url,omitempty"`
	Properties   map[string]string `yaml:"properties,omitempty"`
	CollectedAt  time.Time         `yaml:"collected_at"`
}

// SourceCollection holds collected data from a research run
type SourceCollection struct {
	AgentName   string             `yaml:"agent_name"`
	Query       string             `yaml:"query,omitempty"`
	CollectedAt time.Time          `yaml:"collected_at"`
	Repos       []GitHubRepo       `yaml:"repos,omitempty"`
	Articles    []Article          `yaml:"articles,omitempty"`
	Screenshots []Screenshot       `yaml:"screenshots,omitempty"`
	Papers      []ResearchPaper    `yaml:"papers,omitempty"`
	Trends      []TrendItem        `yaml:"trends,omitempty"`
	Changes     []CompetitorChange `yaml:"changes,omitempty"`
}

// Save writes the collection to a YAML file
func (c *SourceCollection) Save(projectPath, filename string) error {
	sourcesDir := filepath.Join(projectPath, ".pollard", "sources")
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(sourcesDir, filename), data, 0644)
}

// Load reads a source collection from a YAML file
func Load(path string) (*SourceCollection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var collection SourceCollection
	if err := yaml.Unmarshal(data, &collection); err != nil {
		return nil, err
	}
	return &collection, nil
}

// EnsureDirectories creates the .pollard directory structure
func EnsureDirectories(projectPath string) error {
	dirs := []string{
		filepath.Join(projectPath, ".pollard"),
		filepath.Join(projectPath, ".pollard", "insights"),
		filepath.Join(projectPath, ".pollard", "insights", "trends"),
		filepath.Join(projectPath, ".pollard", "insights", "competitive"),
		filepath.Join(projectPath, ".pollard", "patterns"),
		filepath.Join(projectPath, ".pollard", "sources"),
		filepath.Join(projectPath, ".pollard", "sources", "github"),
		filepath.Join(projectPath, ".pollard", "sources", "hackernews"),
		filepath.Join(projectPath, ".pollard", "sources", "research"),
		filepath.Join(projectPath, ".pollard", "sources", "articles"),
		filepath.Join(projectPath, ".pollard", "sources", "screenshots"),
		filepath.Join(projectPath, ".pollard", "sources", "openalex"),
		filepath.Join(projectPath, ".pollard", "sources", "pubmed"),
		filepath.Join(projectPath, ".pollard", "sources", "nutrition"),
		filepath.Join(projectPath, ".pollard", "sources", "legal"),
		filepath.Join(projectPath, ".pollard", "sources", "economics"),
		filepath.Join(projectPath, ".pollard", "sources", "wiki"),
		filepath.Join(projectPath, ".pollard", "reports"),
		filepath.Join(projectPath, ".pollard", "inbox"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
