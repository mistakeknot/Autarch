package discovery

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
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
// Parse errors are silently ignored. Use PollardInsightsWithErrors for error details.
func PollardInsights(root string) ([]PollardInsight, error) {
	insights, _ := PollardInsightsWithErrors(root)
	return insights, nil
}

// PollardInsightsWithErrors loads all insights and returns both successfully parsed insights
// and any parse errors encountered.
func PollardInsightsWithErrors(root string) ([]PollardInsight, []ParseError) {
	insightsDir := filepath.Join(root, ".pollard", "insights")
	return loadInsightsRecursiveWithErrors(insightsDir)
}

func loadInsightsRecursiveWithErrors(dir string) ([]PollardInsight, []ParseError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardInsight{}, nil
		}
		return nil, []ParseError{{Path: dir, Err: err}}
	}

	var insights []PollardInsight
	var errs []ParseError
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recurse into subdirectories (competitive/, trends/, etc.)
			sub, subErrs := loadInsightsRecursiveWithErrors(path)
			insights = append(insights, sub...)
			errs = append(errs, subErrs...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		var insight PollardInsight
		if _, err := yamlsafe.UnmarshalFile(path, &insight); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		insights = append(insights, insight)
	}
	return insights, errs
}

// PollardSources loads source collections from .pollard/sources.
// Parse errors are silently ignored. Use PollardSourcesWithErrors for error details.
func PollardSources(root string) ([]PollardSource, error) {
	sources, _ := PollardSourcesWithErrors(root)
	return sources, nil
}

// PollardSourcesWithErrors loads all sources and returns both successfully parsed sources
// and any parse errors encountered.
func PollardSourcesWithErrors(root string) ([]PollardSource, []ParseError) {
	sourcesDir := filepath.Join(root, ".pollard", "sources")
	return loadSourcesRecursiveWithErrors(sourcesDir)
}

func loadSourcesRecursiveWithErrors(dir string) ([]PollardSource, []ParseError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardSource{}, nil
		}
		return nil, []ParseError{{Path: dir, Err: err}}
	}

	var sources []PollardSource
	var errs []ParseError
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recurse into subdirectories
			sub, subErrs := loadSourcesRecursiveWithErrors(path)
			sources = append(sources, sub...)
			errs = append(errs, subErrs...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
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
		if _, err := yamlsafe.UnmarshalFile(path, &raw); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
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
	return sources, errs
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
// Parse errors are silently ignored. Use PollardPatternsWithErrors for error details.
func PollardPatterns(root string) ([]PollardPattern, error) {
	patterns, _ := PollardPatternsWithErrors(root)
	return patterns, nil
}

// PollardPatternsWithErrors loads all patterns and returns both successfully parsed patterns
// and any parse errors encountered.
func PollardPatternsWithErrors(root string) ([]PollardPattern, []ParseError) {
	patternsDir := filepath.Join(root, ".pollard", "patterns")
	return loadPatternsRecursiveWithErrors(patternsDir)
}

func loadPatternsRecursiveWithErrors(dir string) ([]PollardPattern, []ParseError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PollardPattern{}, nil
		}
		return nil, []ParseError{{Path: dir, Err: err}}
	}

	var patterns []PollardPattern
	var errs []ParseError
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			sub, subErrs := loadPatternsRecursiveWithErrors(path)
			patterns = append(patterns, sub...)
			errs = append(errs, subErrs...)
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		var pattern PollardPattern
		if _, err := yamlsafe.UnmarshalFile(path, &pattern); err != nil {
			errs = append(errs, ParseError{Path: path, Err: err})
			continue
		}
		patterns = append(patterns, pattern)
	}
	return patterns, errs
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
