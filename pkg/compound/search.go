package compound

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Search finds solutions matching the given criteria.
// Solutions are returned sorted by relevance score (highest first).
func Search(projectPath string, opts SearchOptions) ([]SearchResult, error) {
	solutionsPath := filepath.Join(projectPath, SolutionsDir)

	// Check if solutions directory exists
	if _, err := os.Stat(solutionsPath); os.IsNotExist(err) {
		return nil, nil // No solutions yet
	}

	var results []SearchResult

	// Walk all solution files
	err := filepath.Walk(solutionsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-markdown files
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Skip template
		if info.Name() == "TEMPLATE.md" {
			return nil
		}

		// Parse solution file
		result, err := parseSolutionFile(path, projectPath)
		if err != nil {
			// Log but don't fail on individual parse errors
			return nil
		}

		// Apply filters
		if matches, score := matchesFilter(result, opts); matches {
			result.Score = score
			results = append(results, result)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk solutions: %w", err)
	}

	// Sort by score descending
	sortByScore(results)

	return results, nil
}

// parseSolutionFile reads and parses a solution markdown file.
func parseSolutionFile(path, projectRoot string) (SearchResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SearchResult{}, fmt.Errorf("read file: %w", err)
	}

	result := SearchResult{
		Path: relPath(path, projectRoot),
	}

	// Extract YAML frontmatter
	frontmatter, body, err := extractFrontmatter(content)
	if err != nil {
		return SearchResult{}, fmt.Errorf("extract frontmatter: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(frontmatter, &result.Solution); err != nil {
		return SearchResult{}, fmt.Errorf("parse yaml: %w", err)
	}

	// Extract title from markdown H1
	result.Title = extractTitle(body)

	return result, nil
}

// extractFrontmatter separates YAML frontmatter from markdown body.
func extractFrontmatter(content []byte) ([]byte, []byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))

	// Check for opening ---
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, nil, fmt.Errorf("no frontmatter delimiter")
	}

	var frontmatter bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		frontmatter.WriteString(line + "\n")
	}

	// Rest is body
	var body bytes.Buffer
	for scanner.Scan() {
		body.WriteString(scanner.Text() + "\n")
	}

	return frontmatter.Bytes(), body.Bytes(), scanner.Err()
}

// extractTitle finds the first H1 header in markdown.
func extractTitle(body []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// matchesFilter checks if a solution matches the search options.
// Returns match status and relevance score.
func matchesFilter(result SearchResult, opts SearchOptions) (bool, float64) {
	sol := result.Solution
	score := 1.0

	// Module filter (exact match)
	if opts.Module != "" && sol.Module != opts.Module {
		return false, 0
	}

	// Tags filter (any match)
	if len(opts.Tags) > 0 {
		matched := false
		for _, searchTag := range opts.Tags {
			for _, solTag := range sol.Tags {
				if strings.EqualFold(searchTag, solTag) {
					matched = true
					score += 0.5 // Boost for tag match
				}
			}
		}
		if !matched {
			return false, 0
		}
	}

	// Severity filter (match or higher)
	if opts.Severity != "" {
		solLevel := severityLevel(sol.Severity)
		optLevel := severityLevel(opts.Severity)
		if solLevel < optLevel {
			return false, 0
		}
		// Boost higher severity matches
		score += float64(solLevel) * 0.25
	}

	// ProblemType filter (exact match)
	if opts.ProblemType != "" && sol.ProblemType != opts.ProblemType {
		return false, 0
	}

	// Since filter (date comparison)
	if !opts.Since.IsZero() {
		solDate, err := time.Parse("2006-01-02", sol.Date)
		if err != nil || solDate.Before(opts.Since) {
			return false, 0
		}
		// Boost more recent solutions
		daysSince := time.Since(solDate).Hours() / 24
		if daysSince < 30 {
			score += 0.5
		}
	}

	// Query filter (full-text search)
	if opts.Query != "" {
		query := strings.ToLower(opts.Query)
		found := false

		// Search in symptoms
		for _, symptom := range sol.Symptoms {
			if strings.Contains(strings.ToLower(symptom), query) {
				found = true
				score += 1.0
			}
		}

		// Search in root cause
		if strings.Contains(strings.ToLower(sol.RootCause), query) {
			found = true
			score += 1.0
		}

		// Search in title
		if strings.Contains(strings.ToLower(result.Title), query) {
			found = true
			score += 1.5
		}

		// Search in tags
		for _, tag := range sol.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				found = true
				score += 0.5
			}
		}

		if !found {
			return false, 0
		}
	}

	return true, score
}

// severityLevel returns numeric level for severity comparison.
func severityLevel(severity string) int {
	switch severity {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

// relPath returns the relative path from projectRoot.
func relPath(path, projectRoot string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		return path
	}
	return rel
}

// sortByScore sorts results by score descending (bubble sort for simplicity).
func sortByScore(results []SearchResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
