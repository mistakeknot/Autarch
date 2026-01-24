// Package patterns handles implementation patterns collected from research.
package patterns

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Category represents the type of pattern
type Category string

const (
	CategoryUI   Category = "ui"
	CategoryArch Category = "arch"
	CategoryAnti Category = "anti" // anti-patterns
)

// Example shows a concrete implementation of the pattern
type Example struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url,omitempty"`
	Screenshot string `yaml:"screenshot,omitempty"`
	Notes      string `yaml:"notes,omitempty"`
}

// Pattern represents a reusable implementation pattern
type Pattern struct {
	ID                  string    `yaml:"id"`
	Title               string    `yaml:"title"`
	Category            Category  `yaml:"category"`
	CollectedAt         time.Time `yaml:"collected_at"`
	Description         string    `yaml:"description"`
	Examples            []Example `yaml:"examples"`
	ImplementationHints []string  `yaml:"implementation_hints,omitempty"`
	AntiPatterns        []string  `yaml:"anti_patterns,omitempty"`
	LinkedEpics         []string  `yaml:"linked_epics,omitempty"` // EPIC-001, EPIC-002
}

// Load reads a pattern from a YAML file
func Load(path string) (*Pattern, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pattern Pattern
	if err := yaml.Unmarshal(data, &pattern); err != nil {
		return nil, err
	}
	return &pattern, nil
}

// Save writes a pattern to a YAML file
func (p *Pattern) Save(projectPath string) error {
	patternsDir := filepath.Join(projectPath, ".pollard", "patterns")
	if err := os.MkdirAll(patternsDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}

	filename := p.ID + ".yaml"
	return os.WriteFile(filepath.Join(patternsDir, filename), data, 0644)
}

// LoadAll reads all patterns from a project's .pollard/patterns directory
func LoadAll(projectPath string) ([]*Pattern, error) {
	patternsDir := filepath.Join(projectPath, ".pollard", "patterns")
	entries, err := os.ReadDir(patternsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Pattern{}, nil
		}
		return nil, err
	}

	var patterns []*Pattern
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		pattern, err := Load(filepath.Join(patternsDir, entry.Name()))
		if err != nil {
			continue // Skip invalid files
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// FilterByCategory returns patterns matching the given category
func FilterByCategory(patterns []*Pattern, category Category) []*Pattern {
	var filtered []*Pattern
	for _, p := range patterns {
		if p.Category == category {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// Search finds patterns matching a query string in title or description
func Search(patterns []*Pattern, query string) []*Pattern {
	if query == "" {
		return patterns
	}
	var matches []*Pattern
	for _, p := range patterns {
		if containsIgnoreCase(p.Title, query) || containsIgnoreCase(p.Description, query) {
			matches = append(matches, p)
		}
	}
	return matches
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) > 0 && (containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	// Simple case-insensitive contains
	sLower := make([]byte, len(s))
	subLower := make([]byte, len(substr))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		sLower[i] = c
	}
	for i := 0; i < len(substr); i++ {
		c := substr[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		subLower[i] = c
	}
	return indexOf(string(sLower), string(subLower)) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
