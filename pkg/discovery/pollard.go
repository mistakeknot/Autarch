package discovery

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// PollardInsight represents a research insight discovered by Pollard.
type PollardInsight struct {
	ID          string    `yaml:"id"`
	Title       string    `yaml:"title"`
	Category    string    `yaml:"category"` // competitive, trends, user
	CollectedAt time.Time `yaml:"collected_at"`
	Findings    []struct {
		Title       string `yaml:"title"`
		Relevance   string `yaml:"relevance"`
		Description string `yaml:"description"`
	} `yaml:"findings"`
	Recommendations []struct {
		FeatureHint string `yaml:"feature_hint"`
		Priority    string `yaml:"priority"`
		Rationale   string `yaml:"rationale"`
	} `yaml:"recommendations"`
	LinkedFeatures []string `yaml:"linked_features"`
}

// PollardSource represents a source collection from Pollard.
type PollardSource struct {
	AgentName   string    `yaml:"agent_name"`
	Query       string    `yaml:"query"`
	CollectedAt time.Time `yaml:"collected_at"`
	RepoCount   int       `yaml:"-"` // Computed
	PaperCount  int       `yaml:"-"` // Computed
	TrendCount  int       `yaml:"-"` // Computed
}

// PollardInsights loads all insights from a project's .pollard/insights directory.
// This recursively scans subdirectories (competitive/, trends/, etc.).
func PollardInsights(root string) ([]PollardInsight, error) {
	insightsDir := filepath.Join(root, ".pollard", "insights")
	return loadInsightsRecursive(insightsDir)
}

func loadInsightsRecursive(dir string) ([]PollardInsight, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardInsight{}, nil
		}
		return nil, err
	}

	var insights []PollardInsight
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recurse into subdirectories (competitive/, trends/, etc.)
			sub, err := loadInsightsRecursive(path)
			if err != nil {
				continue
			}
			insights = append(insights, sub...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var insight PollardInsight
		if err := yaml.Unmarshal(data, &insight); err != nil {
			continue
		}
		insights = append(insights, insight)
	}
	return insights, nil
}

// PollardSources loads source collections from .pollard/sources.
func PollardSources(root string) ([]PollardSource, error) {
	sourcesDir := filepath.Join(root, ".pollard", "sources")
	return loadSourcesRecursive(sourcesDir)
}

func loadSourcesRecursive(dir string) ([]PollardSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardSource{}, nil
		}
		return nil, err
	}

	var sources []PollardSource
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recurse into subdirectories
			sub, err := loadSourcesRecursive(path)
			if err != nil {
				continue
			}
			sources = append(sources, sub...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw struct {
			AgentName   string    `yaml:"agent_name"`
			Query       string    `yaml:"query"`
			CollectedAt time.Time `yaml:"collected_at"`
			Repos       []struct {
			} `yaml:"repos"`
			Papers []struct {
			} `yaml:"papers"`
			Trends []struct {
			} `yaml:"trends"`
		}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			continue
		}
		sources = append(sources, PollardSource{
			AgentName:   raw.AgentName,
			Query:       raw.Query,
			CollectedAt: raw.CollectedAt,
			RepoCount:   len(raw.Repos),
			PaperCount:  len(raw.Papers),
			TrendCount:  len(raw.Trends),
		})
	}
	return sources, nil
}

// CountPollardInsights returns the total number of insights available.
func CountPollardInsights(root string) int {
	insights, _ := PollardInsights(root)
	return len(insights)
}

// CountPollardSources returns the total number of source collections.
func CountPollardSources(root string) int {
	sources, _ := PollardSources(root)
	return len(sources)
}

// PollardHasData returns true if Pollard has collected any data.
func PollardHasData(root string) bool {
	return CountPollardInsights(root) > 0 || CountPollardSources(root) > 0
}

// RecentPollardInsights returns insights from the last n days.
func RecentPollardInsights(root string, days int) ([]PollardInsight, error) {
	insights, err := PollardInsights(root)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var recent []PollardInsight
	for _, i := range insights {
		if i.CollectedAt.After(cutoff) {
			recent = append(recent, i)
		}
	}
	return recent, nil
}

// PollardPattern represents a pattern identified by Pollard research.
type PollardPattern struct {
	ID                  string    `yaml:"id"`
	Title               string    `yaml:"title"`
	Category            string    `yaml:"category"` // architecture, ux, anti
	Description         string    `yaml:"description"`
	Examples            []string  `yaml:"examples"`
	ImplementationHints []string  `yaml:"implementation_hints"`
	AntiPatterns        []string  `yaml:"anti_patterns,omitempty"`
	CollectedAt         time.Time `yaml:"collected_at"`
}

// PollardPatterns loads all patterns from a project's .pollard/patterns directory.
func PollardPatterns(root string) ([]PollardPattern, error) {
	patternsDir := filepath.Join(root, ".pollard", "patterns")
	return loadPatternsRecursive(patternsDir)
}

func loadPatternsRecursive(dir string) ([]PollardPattern, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardPattern{}, nil
		}
		return nil, err
	}

	var patterns []PollardPattern
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			sub, err := loadPatternsRecursive(path)
			if err != nil {
				continue
			}
			patterns = append(patterns, sub...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var pattern PollardPattern
		if err := yaml.Unmarshal(data, &pattern); err != nil {
			continue
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// CountPollardPatterns returns the total number of patterns available.
func CountPollardPatterns(root string) int {
	patterns, _ := PollardPatterns(root)
	return len(patterns)
}

// PollardAntiPatterns returns only the anti-patterns.
func PollardAntiPatterns(root string) ([]PollardPattern, error) {
	patterns, err := PollardPatterns(root)
	if err != nil {
		return nil, err
	}

	var antiPatterns []PollardPattern
	for _, p := range patterns {
		if p.Category == "anti" || len(p.AntiPatterns) > 0 {
			antiPatterns = append(antiPatterns, p)
		}
	}
	return antiPatterns, nil
}
