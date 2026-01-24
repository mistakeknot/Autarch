// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CompetitorTracker monitors competitor changelogs and updates.
type CompetitorTracker struct {
	client *http.Client
}

// NewCompetitorTracker creates a new competitor tracking hunter.
func NewCompetitorTracker() *CompetitorTracker {
	return &CompetitorTracker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the hunter's identifier.
func (c *CompetitorTracker) Name() string {
	return "competitor-tracker"
}

// Hunt performs the competitor tracking collection.
func (c *CompetitorTracker) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: c.Name(),
		StartedAt:  time.Now(),
	}

	if len(cfg.Targets) == 0 {
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Create output directory
	outputDir := filepath.Join(cfg.ProjectPath, ".pollard", "insights", "competitive")
	if cfg.OutputDir != "" {
		outputDir = filepath.Join(cfg.ProjectPath, cfg.OutputDir)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to create output directory: %w", err))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Rate limiter: 10 requests per minute
	limiter := NewRateLimiter(10, time.Minute, false)

	for _, target := range cfg.Targets {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, nil
		default:
		}

		if target.Changelog == "" {
			continue
		}

		if err := limiter.Wait(ctx); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}

		insight, err := c.fetchChangelog(ctx, target)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to fetch %s: %w", target.Name, err))
			continue
		}

		result.SourcesCollected++

		// Write output file
		filename := fmt.Sprintf("%s-%s.yaml",
			sanitizeName(target.Name),
			time.Now().Format("2006-01-02"))
		outputPath := filepath.Join(outputDir, filename)

		if err := c.writeInsight(outputPath, insight); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write %s: %w", filename, err))
			continue
		}

		result.InsightsCreated++
		result.OutputFiles = append(result.OutputFiles, outputPath)
	}

	result.CompletedAt = time.Now()
	return result, nil
}

// CompetitorInsight represents the collected competitor data.
type CompetitorInsight struct {
	Competitor   string           `yaml:"competitor"`
	CollectedAt  time.Time        `yaml:"collected_at"`
	ChangelogURL string           `yaml:"changelog_url"`
	Changes      []ChangelogEntry `yaml:"changes"`
}

// ChangelogEntry represents a single changelog item.
type ChangelogEntry struct {
	Date           string          `yaml:"date,omitempty"`
	Title          string          `yaml:"title"`
	Description    string          `yaml:"description,omitempty"`
	URL            string          `yaml:"url,omitempty"`
	Relevance      string          `yaml:"relevance"`
	ThreatLevel    string          `yaml:"threat_level"`
	Recommendation *Recommendation `yaml:"recommendation,omitempty"`
}

// Recommendation suggests how to respond to the change.
type Recommendation struct {
	FeatureHint string `yaml:"feature_hint"`
	Priority    string `yaml:"priority"`
	Rationale   string `yaml:"rationale"`
}

// fetchChangelog fetches and parses a competitor's changelog.
func (c *CompetitorTracker) fetchChangelog(ctx context.Context, target CompetitorTarget) (*CompetitorInsight, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target.Changelog, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pollard/1.0 (Research Agent)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	entries := c.parseChangelog(string(body), target.Changelog)

	return &CompetitorInsight{
		Competitor:   target.Name,
		CollectedAt:  time.Now().UTC(),
		ChangelogURL: target.Changelog,
		Changes:      entries,
	}, nil
}

// parseChangelog extracts changelog entries from HTML content.
func (c *CompetitorTracker) parseChangelog(html, baseURL string) []ChangelogEntry {
	var entries []ChangelogEntry

	// Extract sections - look for h2 or h3 as entry headers
	sections := c.extractSections(html)

	for _, section := range sections {
		entry := ChangelogEntry{
			Title:       section.title,
			Date:        section.date,
			Description: section.description,
			URL:         section.url,
			Relevance:   c.assessRelevance(section.title, section.description),
			ThreatLevel: c.assessThreatLevel(section.title, section.description),
		}

		// Resolve relative URLs
		if entry.URL != "" && !strings.HasPrefix(entry.URL, "http") {
			entry.URL = resolveURL(baseURL, entry.URL)
		}

		// Add recommendation for high relevance items
		if entry.Relevance == "high" {
			entry.Recommendation = c.generateRecommendation(section.title, section.description)
		}

		entries = append(entries, entry)
	}

	// Limit to most recent entries
	maxEntries := 10
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}

	return entries
}

// section represents a parsed changelog section.
type section struct {
	title       string
	date        string
	description string
	url         string
}

// extractSections parses HTML to find changelog entries.
func (c *CompetitorTracker) extractSections(html string) []section {
	var sections []section

	// Pattern for headings (h1, h2, h3)
	headingPattern := regexp.MustCompile(`(?i)<h[123][^>]*>(.*?)</h[123]>`)
	headingMatches := headingPattern.FindAllStringSubmatchIndex(html, -1)

	for i, match := range headingMatches {
		if len(match) < 4 {
			continue
		}

		// Extract heading text
		headingHTML := html[match[2]:match[3]]
		title := stripTags(headingHTML)
		title = strings.TrimSpace(title)

		if title == "" {
			continue
		}

		// Determine section boundaries
		sectionStart := match[1]
		var sectionEnd int
		if i+1 < len(headingMatches) {
			sectionEnd = headingMatches[i+1][0]
		} else {
			sectionEnd = len(html)
			if sectionEnd > sectionStart+5000 {
				sectionEnd = sectionStart + 5000
			}
		}

		sectionHTML := html[sectionStart:sectionEnd]

		sec := section{
			title:       title,
			date:        extractDate(sectionHTML),
			description: extractDescription(sectionHTML),
			url:         extractFirstLink(sectionHTML),
		}

		sections = append(sections, sec)
	}

	// If no headings found, try article-based extraction
	if len(sections) == 0 {
		sections = c.extractArticles(html)
	}

	return sections
}

// extractArticles tries to find article/entry elements.
func (c *CompetitorTracker) extractArticles(html string) []section {
	var sections []section

	// Try <article> tags
	articlePattern := regexp.MustCompile(`(?is)<article[^>]*>(.*?)</article>`)
	matches := articlePattern.FindAllStringSubmatch(html, 20)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		articleHTML := match[1]
		title := extractTitle(articleHTML)
		if title == "" {
			continue
		}

		sections = append(sections, section{
			title:       title,
			date:        extractDate(articleHTML),
			description: extractDescription(articleHTML),
			url:         extractFirstLink(articleHTML),
		})
	}

	// Try <li> items in lists if no articles found
	if len(sections) == 0 {
		liPattern := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
		matches = liPattern.FindAllStringSubmatch(html, 30)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			liHTML := match[1]
			title := extractTitle(liHTML)
			if title == "" {
				title = strings.TrimSpace(stripTags(liHTML))
			}
			if title == "" || len(title) < 10 {
				continue
			}

			// Truncate long titles
			if len(title) > 200 {
				title = title[:200] + "..."
			}

			sections = append(sections, section{
				title:       title,
				date:        extractDate(liHTML),
				description: "",
				url:         extractFirstLink(liHTML),
			})
		}
	}

	return sections
}

// extractTitle finds a title in HTML content.
func extractTitle(html string) string {
	// Try h tags first
	hPattern := regexp.MustCompile(`(?i)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	if match := hPattern.FindStringSubmatch(html); len(match) > 1 {
		return strings.TrimSpace(stripTags(match[1]))
	}

	// Try strong/b tags
	strongPattern := regexp.MustCompile(`(?i)<(?:strong|b)[^>]*>(.*?)</(?:strong|b)>`)
	if match := strongPattern.FindStringSubmatch(html); len(match) > 1 {
		title := strings.TrimSpace(stripTags(match[1]))
		if len(title) > 5 {
			return title
		}
	}

	// Try first link text
	linkPattern := regexp.MustCompile(`(?i)<a[^>]*>(.*?)</a>`)
	if match := linkPattern.FindStringSubmatch(html); len(match) > 1 {
		title := strings.TrimSpace(stripTags(match[1]))
		if len(title) > 5 {
			return title
		}
	}

	return ""
}

// extractDate finds a date in HTML content.
func extractDate(html string) string {
	// Try <time> element with datetime attribute
	timePattern := regexp.MustCompile(`(?i)<time[^>]*datetime=["']([^"']+)["'][^>]*>`)
	if match := timePattern.FindStringSubmatch(html); len(match) > 1 {
		return parseAndFormatDate(match[1])
	}

	// Try <time> element content
	timeContentPattern := regexp.MustCompile(`(?i)<time[^>]*>(.*?)</time>`)
	if match := timeContentPattern.FindStringSubmatch(html); len(match) > 1 {
		return parseAndFormatDate(stripTags(match[1]))
	}

	// Try common date patterns in text
	datePatterns := []string{
		// ISO format
		`\b(\d{4}-\d{2}-\d{2})\b`,
		// US format
		`\b((?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{1,2},?\s+\d{4})\b`,
		// European format
		`\b(\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{4})\b`,
		// Slash format
		`\b(\d{1,2}/\d{1,2}/\d{2,4})\b`,
	}

	plainText := stripTags(html)
	for _, pattern := range datePatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if match := re.FindStringSubmatch(plainText); len(match) > 1 {
			return parseAndFormatDate(match[1])
		}
	}

	return ""
}

// parseAndFormatDate attempts to parse and normalize a date string.
func parseAndFormatDate(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)

	// Already in YYYY-MM-DD format
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}`, dateStr); matched {
		return dateStr[:10]
	}

	// Try various formats
	formats := []string{
		"January 2, 2006",
		"Jan 2, 2006",
		"January 2 2006",
		"Jan 2 2006",
		"2 January 2006",
		"2 Jan 2006",
		"01/02/2006",
		"1/2/2006",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.Format("2006-01-02")
		}
	}

	// Return as-is if we can't parse it
	if len(dateStr) > 20 {
		return dateStr[:20]
	}
	return dateStr
}

// extractDescription finds a description paragraph.
func extractDescription(html string) string {
	// Try <p> tags
	pPattern := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	matches := pPattern.FindAllStringSubmatch(html, 5)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		text := strings.TrimSpace(stripTags(match[1]))
		// Skip empty or very short paragraphs
		if len(text) < 20 {
			continue
		}

		// Truncate long descriptions
		if len(text) > 300 {
			text = text[:300] + "..."
		}

		return text
	}

	return ""
}

// extractFirstLink finds the first link in HTML content.
func extractFirstLink(html string) string {
	linkPattern := regexp.MustCompile(`(?i)<a[^>]*href=["']([^"']+)["']`)
	if match := linkPattern.FindStringSubmatch(html); len(match) > 1 {
		href := match[1]
		// Skip anchor links and javascript
		if !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
			return href
		}
	}
	return ""
}

// stripTags removes HTML tags from a string.
func stripTags(html string) string {
	// Remove script and style content
	scriptPattern := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</\1>`)
	html = scriptPattern.ReplaceAllString(html, "")

	// Remove all tags
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	text := tagPattern.ReplaceAllString(html, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&#x27;", "'")

	// Normalize whitespace
	spacePattern := regexp.MustCompile(`\s+`)
	text = spacePattern.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// resolveURL resolves a relative URL against a base URL.
func resolveURL(baseURL, relURL string) string {
	if strings.HasPrefix(relURL, "//") {
		return "https:" + relURL
	}
	if strings.HasPrefix(relURL, "/") {
		// Extract base domain
		domainPattern := regexp.MustCompile(`^(https?://[^/]+)`)
		if match := domainPattern.FindStringSubmatch(baseURL); len(match) > 1 {
			return match[1] + relURL
		}
	}
	// For relative paths, append to base
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + relURL
	}
	// Remove last path segment
	lastSlash := strings.LastIndex(baseURL, "/")
	if lastSlash > 8 { // After https://
		return baseURL[:lastSlash+1] + relURL
	}
	return baseURL + "/" + relURL
}

// sanitizeName creates a safe filename from a competitor name.
func sanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace spaces and special chars with hyphens
	safePattern := regexp.MustCompile(`[^a-z0-9]+`)
	name = safePattern.ReplaceAllString(name, "-")
	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")
	return name
}

// assessRelevance determines the relevance of a changelog entry.
func (c *CompetitorTracker) assessRelevance(title, description string) string {
	combined := strings.ToLower(title + " " + description)

	// High relevance keywords
	highKeywords := []string{
		"agent", "multi-file", "autonomous", "ai", "llm",
		"code generation", "autocomplete", "copilot",
		"context", "codebase", "project-wide",
		"terminal", "cli", "command line",
		"vim", "neovim", "editor",
	}

	// Medium relevance keywords
	mediumKeywords := []string{
		"feature", "new", "major", "release",
		"performance", "speed", "faster",
		"api", "integration", "plugin",
		"model", "language",
	}

	for _, kw := range highKeywords {
		if strings.Contains(combined, kw) {
			return "high"
		}
	}

	for _, kw := range mediumKeywords {
		if strings.Contains(combined, kw) {
			return "medium"
		}
	}

	return "low"
}

// assessThreatLevel determines competitive threat level.
func (c *CompetitorTracker) assessThreatLevel(title, description string) string {
	combined := strings.ToLower(title + " " + description)

	// High threat - direct competitive features
	highThreatKeywords := []string{
		"agent mode", "agentic", "autonomous coding",
		"multi-file edit", "project-wide refactor",
		"code review", "pr review",
		"terminal integration", "shell",
	}

	// Medium threat - feature parity
	mediumThreatKeywords := []string{
		"context", "embeddings", "rag",
		"faster", "performance",
		"free tier", "pricing",
	}

	for _, kw := range highThreatKeywords {
		if strings.Contains(combined, kw) {
			return "high"
		}
	}

	for _, kw := range mediumThreatKeywords {
		if strings.Contains(combined, kw) {
			return "medium"
		}
	}

	return "low"
}

// generateRecommendation creates a recommendation for high-relevance items.
func (c *CompetitorTracker) generateRecommendation(title, description string) *Recommendation {
	combined := strings.ToLower(title + " " + description)

	// Match patterns to recommendations
	recommendations := []struct {
		keywords    []string
		featureHint string
		priority    string
		rationale   string
	}{
		{
			keywords:    []string{"agent", "agentic", "autonomous"},
			featureHint: "Agent capabilities enhancement",
			priority:    "p0",
			rationale:   "Core competitive differentiator - competitors shipping similar features",
		},
		{
			keywords:    []string{"multi-file", "project-wide", "codebase"},
			featureHint: "Multi-file editing support",
			priority:    "p1",
			rationale:   "Users expect cross-file operations for real-world refactoring",
		},
		{
			keywords:    []string{"context", "embeddings", "rag"},
			featureHint: "Context management improvements",
			priority:    "p1",
			rationale:   "Better context = better code suggestions",
		},
		{
			keywords:    []string{"terminal", "cli", "shell"},
			featureHint: "Terminal integration",
			priority:    "p1",
			rationale:   "CLI-native experience is key differentiator",
		},
		{
			keywords:    []string{"vim", "neovim", "editor"},
			featureHint: "Editor integration",
			priority:    "p2",
			rationale:   "Meet developers where they work",
		},
		{
			keywords:    []string{"performance", "speed", "faster"},
			featureHint: "Performance optimization",
			priority:    "p2",
			rationale:   "Speed directly impacts developer experience",
		},
	}

	for _, rec := range recommendations {
		for _, kw := range rec.keywords {
			if strings.Contains(combined, kw) {
				return &Recommendation{
					FeatureHint: rec.featureHint,
					Priority:    rec.priority,
					Rationale:   rec.rationale,
				}
			}
		}
	}

	// Default recommendation for high relevance items
	return &Recommendation{
		FeatureHint: "Feature parity consideration",
		Priority:    "p2",
		Rationale:   "Competitor shipping new capability - evaluate for roadmap",
	}
}

// writeInsight writes the competitor insight to a YAML file.
func (c *CompetitorTracker) writeInsight(path string, insight *CompetitorInsight) error {
	data, err := yaml.Marshal(insight)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
