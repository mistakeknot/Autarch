package quick

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/pollard/hunters"
	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

// Scanner performs quick research scans for PRD context
type Scanner struct {
	github hunters.Hunter
	hn     hunters.Hunter
}

// NewScanner creates a scanner with default hunters
func NewScanner() *Scanner {
	return &Scanner{
		github: hunters.NewGitHubScout(),
		hn:     hunters.NewHackerNewsHunter(),
	}
}

// Scan runs github-scout and hackernews in parallel with 30s timeout
func (s *Scanner) Scan(ctx context.Context, topic string, projectPath string) (*arbiter.QuickScanResult, error) {
	// Apply 30 second timeout for quick scan
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Create temp output dir for hunter results
	outputDir := filepath.Join(projectPath, ".pollard", "quickscan")
	os.MkdirAll(outputDir, 0755)

	result := &arbiter.QuickScanResult{
		Topic:     topic,
		ScannedAt: time.Now(),
	}

	cfg := hunters.HunterConfig{
		Queries:     []string{topic},
		MaxResults:  5,
		OutputDir:   "quickscan",
		ProjectPath: projectPath,
		Mode:        "quick",
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Run GitHub scout
	wg.Add(1)
	go func() {
		defer wg.Done()
		ghResult, err := s.github.Hunt(ctx, cfg)
		if err != nil || len(ghResult.OutputFiles) == 0 {
			return
		}

		findings := parseGitHubOutput(ghResult.OutputFiles)
		mu.Lock()
		result.GitHubHits = findings
		mu.Unlock()
	}()

	// Run HackerNews hunter
	wg.Add(1)
	go func() {
		defer wg.Done()
		hnResult, err := s.hn.Hunt(ctx, cfg)
		if err != nil || len(hnResult.OutputFiles) == 0 {
			return
		}

		findings := parseHNOutput(hnResult.OutputFiles)
		mu.Lock()
		result.HNHits = findings
		mu.Unlock()
	}()

	wg.Wait()

	// Generate summary
	result.Summary = synthesizeSummary(result)

	return result, nil
}

// gitHubOutput matches the YAML structure written by GitHubScout
type gitHubOutput struct {
	Repos []struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		URL         string `yaml:"url"`
		Stars       int    `yaml:"stars"`
	} `yaml:"repos"`
}

// hnOutput matches the YAML structure written by HackerNewsHunter
type hnOutput struct {
	Trends []struct {
		Title    string `yaml:"title"`
		URL      string `yaml:"url"`
		Points   int    `yaml:"points"`
		Comments int    `yaml:"comments"`
	} `yaml:"trends"`
}

func parseGitHubOutput(files []string) []arbiter.GitHubFinding {
	var findings []arbiter.GitHubFinding
	for _, f := range files {
		var output gitHubOutput
		if _, err := yamlsafe.UnmarshalFile(f, &output); err != nil {
			continue
		}

		for _, repo := range output.Repos {
			findings = append(findings, arbiter.GitHubFinding{
				Name:        repo.Name,
				Description: repo.Description,
				Stars:       repo.Stars,
				URL:         repo.URL,
			})
		}
	}
	return findings
}

func parseHNOutput(files []string) []arbiter.HNFinding {
	var findings []arbiter.HNFinding
	for _, f := range files {
		var output hnOutput
		if _, err := yamlsafe.UnmarshalFile(f, &output); err != nil {
			continue
		}

		for _, trend := range output.Trends {
			findings = append(findings, arbiter.HNFinding{
				Title:    trend.Title,
				Points:   trend.Points,
				Comments: trend.Comments,
				URL:      trend.URL,
			})
		}
	}
	return findings
}

func synthesizeSummary(result *arbiter.QuickScanResult) string {
	if len(result.GitHubHits) == 0 && len(result.HNHits) == 0 {
		return "No relevant results found."
	}

	summary := ""
	if len(result.GitHubHits) > 0 {
		summary += fmt.Sprintf("Found %d relevant GitHub projects", len(result.GitHubHits))
		if result.GitHubHits[0].Stars > 1000 {
			summary += " (including popular ones with 1k+ stars)"
		}
		summary += ". "
	}

	if len(result.HNHits) > 0 {
		summary += fmt.Sprintf("Found %d HackerNews discussions", len(result.HNHits))
		totalComments := 0
		for _, h := range result.HNHits {
			totalComments += h.Comments
		}
		if totalComments > 100 {
			summary += " with active community engagement"
		}
		summary += "."
	}

	return summary
}
