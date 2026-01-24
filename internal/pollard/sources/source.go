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

// SourceCollection holds collected data from a research run
type SourceCollection struct {
	AgentName   string       `yaml:"agent_name"`
	CollectedAt time.Time    `yaml:"collected_at"`
	Repos       []GitHubRepo `yaml:"repos,omitempty"`
	Articles    []Article    `yaml:"articles,omitempty"`
	Screenshots []Screenshot `yaml:"screenshots,omitempty"`
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
		filepath.Join(projectPath, ".pollard", "patterns"),
		filepath.Join(projectPath, ".pollard", "sources"),
		filepath.Join(projectPath, ".pollard", "sources", "github"),
		filepath.Join(projectPath, ".pollard", "sources", "articles"),
		filepath.Join(projectPath, ".pollard", "sources", "screenshots"),
		filepath.Join(projectPath, ".pollard", "reports"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
