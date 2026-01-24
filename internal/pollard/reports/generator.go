// Package reports generates markdown research reports from collected sources.
package reports

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/patterns"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/sources"
)

// ReportType defines the type of report to generate.
type ReportType string

const (
	TypeLandscape   ReportType = "landscape"
	TypeCompetitive ReportType = "competitive"
	TypeTrends      ReportType = "trends"
	TypeResearch    ReportType = "research"
)

// Generator creates markdown reports from collected data.
type Generator struct {
	projectPath string
}

// NewGenerator creates a new report generator.
func NewGenerator(projectPath string) *Generator {
	return &Generator{projectPath: projectPath}
}

// Generate creates a report of the specified type and writes it to a file.
func (g *Generator) Generate(reportType ReportType) (string, error) {
	switch reportType {
	case TypeLandscape:
		return g.generateLandscapeReport()
	case TypeCompetitive:
		return g.generateCompetitiveReport()
	case TypeTrends:
		return g.generateTrendsReport()
	case TypeResearch:
		return g.generateResearchReport()
	default:
		return g.generateLandscapeReport()
	}
}

// generateLandscapeReport creates a comprehensive landscape overview.
func (g *Generator) generateLandscapeReport() (string, error) {
	var sb strings.Builder
	now := time.Now()

	// Load all data
	allInsights, _ := insights.LoadAll(g.projectPath)
	allPatterns, _ := patterns.LoadAll(g.projectPath)
	githubSources := g.loadGitHubSources()
	trendSources := g.loadTrendSources()
	researchSources := g.loadResearchSources()
	competitorSources := g.loadCompetitorSources()

	// Header
	sb.WriteString("# Landscape Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", now.Format("2006-01-02 15:04")))

	// Executive Summary
	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Insights**: %d total\n", len(allInsights)))
	sb.WriteString(fmt.Sprintf("- **Patterns**: %d identified\n", len(allPatterns)))
	sb.WriteString(fmt.Sprintf("- **GitHub Repos**: %d tracked\n", len(githubSources)))
	sb.WriteString(fmt.Sprintf("- **Trend Items**: %d captured\n", len(trendSources)))
	sb.WriteString(fmt.Sprintf("- **Research Papers**: %d indexed\n", len(researchSources)))
	sb.WriteString(fmt.Sprintf("- **Competitor Changes**: %d detected\n", len(competitorSources)))
	sb.WriteString("\n")

	// High-priority items
	sb.WriteString("## High Priority Items\n\n")

	// High relevance trends
	highTrends := filterHighRelevance(trendSources)
	if len(highTrends) > 0 {
		sb.WriteString("### Industry Signals\n\n")
		for _, t := range highTrends[:min(5, len(highTrends))] {
			sb.WriteString(fmt.Sprintf("- **[%s](%s)** (%d points)\n", t.Title, t.URL, t.Points))
			if t.Signal != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", t.Signal))
			}
		}
		sb.WriteString("\n")
	}

	// High relevance research
	highResearch := filterHighRelevancePapers(researchSources)
	if len(highResearch) > 0 {
		sb.WriteString("### Research Highlights\n\n")
		for _, p := range highResearch[:min(5, len(highResearch))] {
			sb.WriteString(fmt.Sprintf("- **[%s](%s)**\n", p.Title, p.URL))
			if p.Signal != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", p.Signal))
			}
		}
		sb.WriteString("\n")
	}

	// Competitor changes
	if len(competitorSources) > 0 {
		sb.WriteString("### Competitor Activity\n\n")
		for _, c := range competitorSources[:min(10, len(competitorSources))] {
			threatBadge := ""
			if c.ThreatLevel == "high" {
				threatBadge = " [HIGH THREAT]"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s%s\n", c.Competitor, c.Title, threatBadge))
			if c.Recommendation != nil {
				sb.WriteString(fmt.Sprintf("  - Recommendation (%s): %s\n", c.Recommendation.Priority, c.Recommendation.FeatureHint))
			}
		}
		sb.WriteString("\n")
	}

	// GitHub trending repos
	if len(githubSources) > 0 {
		sb.WriteString("## Trending Repositories\n\n")
		// Sort by stars
		sort.Slice(githubSources, func(i, j int) bool {
			return githubSources[i].Stars > githubSources[j].Stars
		})
		for _, r := range githubSources[:min(10, len(githubSources))] {
			sb.WriteString(fmt.Sprintf("- **[%s/%s](%s)** ⭐ %d\n", r.Owner, r.Name, r.URL, r.Stars))
			if r.Description != "" {
				desc := r.Description
				if len(desc) > 100 {
					desc = desc[:100] + "..."
				}
				sb.WriteString(fmt.Sprintf("  - %s\n", desc))
			}
		}
		sb.WriteString("\n")
	}

	// Insights summary
	if len(allInsights) > 0 {
		sb.WriteString("## Insights\n\n")
		competitive := insights.FilterByCategory(allInsights, insights.CategoryCompetitive)
		trends := insights.FilterByCategory(allInsights, insights.CategoryTrends)
		user := insights.FilterByCategory(allInsights, insights.CategoryUser)

		sb.WriteString(fmt.Sprintf("| Category | Count |\n"))
		sb.WriteString(fmt.Sprintf("|----------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| Competitive | %d |\n", len(competitive)))
		sb.WriteString(fmt.Sprintf("| Trends | %d |\n", len(trends)))
		sb.WriteString(fmt.Sprintf("| User Research | %d |\n", len(user)))
		sb.WriteString("\n")
	}

	// Patterns summary
	if len(allPatterns) > 0 {
		sb.WriteString("## Patterns\n\n")
		ui := patterns.FilterByCategory(allPatterns, patterns.CategoryUI)
		arch := patterns.FilterByCategory(allPatterns, patterns.CategoryArch)
		anti := patterns.FilterByCategory(allPatterns, patterns.CategoryAnti)

		sb.WriteString(fmt.Sprintf("| Category | Count |\n"))
		sb.WriteString(fmt.Sprintf("|----------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| UI Patterns | %d |\n", len(ui)))
		sb.WriteString(fmt.Sprintf("| Architecture | %d |\n", len(arch)))
		sb.WriteString(fmt.Sprintf("| Anti-patterns | %d |\n", len(anti)))
		sb.WriteString("\n")
	}

	// Write to file
	return g.writeReport("landscape", sb.String())
}

// generateCompetitiveReport creates a competitor-focused report.
func (g *Generator) generateCompetitiveReport() (string, error) {
	var sb strings.Builder
	now := time.Now()

	competitorSources := g.loadCompetitorSources()

	sb.WriteString("# Competitive Analysis Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", now.Format("2006-01-02 15:04")))

	if len(competitorSources) == 0 {
		sb.WriteString("No competitor changes detected.\n\n")
		sb.WriteString("Run `pollard scan --hunter competitor-tracker` to collect competitor intelligence.\n")
		return g.writeReport("competitive", sb.String())
	}

	// Group by competitor
	byCompetitor := make(map[string][]sources.CompetitorChange)
	for _, c := range competitorSources {
		byCompetitor[c.Competitor] = append(byCompetitor[c.Competitor], c)
	}

	// Summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Competitor | Changes | High Threat |\n")
	sb.WriteString("|------------|---------|-------------|\n")
	for comp, changes := range byCompetitor {
		highThreat := 0
		for _, c := range changes {
			if c.ThreatLevel == "high" {
				highThreat++
			}
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d |\n", comp, len(changes), highThreat))
	}
	sb.WriteString("\n")

	// Details by competitor
	sb.WriteString("## Details\n\n")
	for comp, changes := range byCompetitor {
		sb.WriteString(fmt.Sprintf("### %s\n\n", comp))
		for _, c := range changes {
			threatBadge := ""
			if c.ThreatLevel == "high" {
				threatBadge = " 🔴"
			} else if c.ThreatLevel == "medium" {
				threatBadge = " 🟡"
			}

			sb.WriteString(fmt.Sprintf("**%s**%s\n\n", c.Title, threatBadge))
			if c.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", c.Description))
			}
			if c.URL != "" {
				sb.WriteString(fmt.Sprintf("Source: [%s](%s)\n\n", c.URL, c.URL))
			}
			if c.Recommendation != nil {
				sb.WriteString(fmt.Sprintf("> **Recommendation** (%s): %s\n", c.Recommendation.Priority, c.Recommendation.FeatureHint))
				sb.WriteString(fmt.Sprintf("> \n> %s\n\n", c.Recommendation.Rationale))
			}
		}
	}

	return g.writeReport("competitive", sb.String())
}

// generateTrendsReport creates an industry trends report.
func (g *Generator) generateTrendsReport() (string, error) {
	var sb strings.Builder
	now := time.Now()

	trendSources := g.loadTrendSources()

	sb.WriteString("# Industry Trends Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", now.Format("2006-01-02 15:04")))

	if len(trendSources) == 0 {
		sb.WriteString("No trend data collected.\n\n")
		sb.WriteString("Run `pollard scan --hunter trend-watcher` to collect industry trends.\n")
		return g.writeReport("trends", sb.String())
	}

	// Sort by points
	sort.Slice(trendSources, func(i, j int) bool {
		return trendSources[i].Points > trendSources[j].Points
	})

	// High relevance section
	highRelevance := filterHighRelevance(trendSources)
	if len(highRelevance) > 0 {
		sb.WriteString("## High Relevance\n\n")
		for _, t := range highRelevance {
			sb.WriteString(fmt.Sprintf("### [%s](%s)\n\n", t.Title, t.URL))
			sb.WriteString(fmt.Sprintf("- Points: %d | Comments: %d\n", t.Points, t.Comments))
			sb.WriteString(fmt.Sprintf("- Source: %s\n", t.Source))
			if t.Signal != "" {
				sb.WriteString(fmt.Sprintf("- Signal: %s\n", t.Signal))
			}
			sb.WriteString("\n")
		}
	}

	// All items table
	sb.WriteString("## All Trends\n\n")
	sb.WriteString("| Title | Points | Comments | Source |\n")
	sb.WriteString("|-------|--------|----------|--------|\n")
	for _, t := range trendSources {
		title := t.Title
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		sb.WriteString(fmt.Sprintf("| [%s](%s) | %d | %d | %s |\n", title, t.URL, t.Points, t.Comments, t.Source))
	}
	sb.WriteString("\n")

	return g.writeReport("trends", sb.String())
}

// generateResearchReport creates an academic research report.
func (g *Generator) generateResearchReport() (string, error) {
	var sb strings.Builder
	now := time.Now()

	researchSources := g.loadResearchSources()

	sb.WriteString("# Research Papers Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", now.Format("2006-01-02 15:04")))

	if len(researchSources) == 0 {
		sb.WriteString("No research papers collected.\n\n")
		sb.WriteString("Run `pollard scan --hunter research-scout` to collect academic papers.\n")
		return g.writeReport("research", sb.String())
	}

	// Sort by relevance, then citations
	sort.Slice(researchSources, func(i, j int) bool {
		if researchSources[i].Relevance != researchSources[j].Relevance {
			return relevanceScore(researchSources[i].Relevance) > relevanceScore(researchSources[j].Relevance)
		}
		return researchSources[i].Citations > researchSources[j].Citations
	})

	// High relevance papers
	highRelevance := filterHighRelevancePapers(researchSources)
	if len(highRelevance) > 0 {
		sb.WriteString("## Key Papers\n\n")
		for _, p := range highRelevance {
			sb.WriteString(fmt.Sprintf("### [%s](%s)\n\n", p.Title, p.URL))
			sb.WriteString(fmt.Sprintf("- Authors: %s\n", strings.Join(p.Authors, ", ")))
			sb.WriteString(fmt.Sprintf("- Categories: %s\n", strings.Join(p.Categories, ", ")))
			if p.Citations > 0 {
				sb.WriteString(fmt.Sprintf("- Citations: %d\n", p.Citations))
			}
			if p.HasCode && p.CodeURL != "" {
				sb.WriteString(fmt.Sprintf("- Code: [%s](%s)\n", p.CodeURL, p.CodeURL))
			}
			if p.Signal != "" {
				sb.WriteString(fmt.Sprintf("- **Signal**: %s\n", p.Signal))
			}
			sb.WriteString("\n")

			if p.Abstract != "" {
				abstract := p.Abstract
				if len(abstract) > 300 {
					abstract = abstract[:300] + "..."
				}
				sb.WriteString(fmt.Sprintf("> %s\n\n", abstract))
			}
		}
	}

	// All papers table
	sb.WriteString("## All Papers\n\n")
	sb.WriteString("| Title | Categories | Citations | Relevance |\n")
	sb.WriteString("|-------|------------|-----------|------------|\n")
	for _, p := range researchSources {
		title := p.Title
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		cats := strings.Join(p.Categories, ", ")
		if len(cats) > 20 {
			cats = cats[:20] + "..."
		}
		sb.WriteString(fmt.Sprintf("| [%s](%s) | %s | %d | %s |\n", title, p.URL, cats, p.Citations, p.Relevance))
	}
	sb.WriteString("\n")

	return g.writeReport("research", sb.String())
}

// writeReport writes the report content to a file.
func (g *Generator) writeReport(reportType, content string) (string, error) {
	reportsDir := filepath.Join(g.projectPath, ".pollard", "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.md", reportType, time.Now().Format("2006-01-02"))
	filePath := filepath.Join(reportsDir, filename)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	return filePath, nil
}

// loadGitHubSources loads all GitHub sources from YAML files.
func (g *Generator) loadGitHubSources() []sources.GitHubRepo {
	var repos []sources.GitHubRepo

	githubDir := filepath.Join(g.projectPath, ".pollard", "sources", "github")
	_ = filepath.WalkDir(githubDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var output struct {
			Repos []sources.GitHubRepo `yaml:"repos"`
		}
		if err := yaml.Unmarshal(data, &output); err != nil {
			return nil
		}

		repos = append(repos, output.Repos...)
		return nil
	})

	return repos
}

// loadTrendSources loads all trend sources from YAML files.
func (g *Generator) loadTrendSources() []sources.TrendItem {
	var trends []sources.TrendItem

	// Check hackernews directory
	hnDir := filepath.Join(g.projectPath, ".pollard", "sources", "hackernews")
	_ = filepath.WalkDir(hnDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var output struct {
			Trends []sources.TrendItem `yaml:"trends"`
		}
		if err := yaml.Unmarshal(data, &output); err != nil {
			return nil
		}

		trends = append(trends, output.Trends...)
		return nil
	})

	return trends
}

// loadResearchSources loads all research papers from YAML files.
func (g *Generator) loadResearchSources() []sources.ResearchPaper {
	var papers []sources.ResearchPaper

	researchDir := filepath.Join(g.projectPath, ".pollard", "sources", "research")
	_ = filepath.WalkDir(researchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var output struct {
			Papers []sources.ResearchPaper `yaml:"papers"`
		}
		if err := yaml.Unmarshal(data, &output); err != nil {
			return nil
		}

		papers = append(papers, output.Papers...)
		return nil
	})

	return papers
}

// loadCompetitorSources loads all competitor changes from YAML files.
func (g *Generator) loadCompetitorSources() []sources.CompetitorChange {
	var changes []sources.CompetitorChange

	// Check competitive insights directory
	compDir := filepath.Join(g.projectPath, ".pollard", "insights", "competitive")
	_ = filepath.WalkDir(compDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var output struct {
			Changes []sources.CompetitorChange `yaml:"changes"`
		}
		if err := yaml.Unmarshal(data, &output); err != nil {
			return nil
		}

		changes = append(changes, output.Changes...)
		return nil
	})

	return changes
}

// Helper functions

func filterHighRelevance(items []sources.TrendItem) []sources.TrendItem {
	var high []sources.TrendItem
	for _, item := range items {
		if item.Relevance == "high" {
			high = append(high, item)
		}
	}
	return high
}

func filterHighRelevancePapers(items []sources.ResearchPaper) []sources.ResearchPaper {
	var high []sources.ResearchPaper
	for _, item := range items {
		if item.Relevance == "high" {
			high = append(high, item)
		}
	}
	return high
}

func relevanceScore(r string) int {
	switch r {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
