// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/scoring"
	"gopkg.in/yaml.v3"
)

// ArxivHunter searches academic papers from arXiv.
// It implements the 4-stage pipeline: Search → Fetch → Synthesize → Score.
type ArxivHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	fetcher     *pipeline.Fetcher
	synthesizer *pipeline.Synthesizer
	scorer      *scoring.Scorer
}

// NewArxivHunter creates a new arXiv research hunter.
func NewArxivHunter() *ArxivHunter {
	return &ArxivHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// arXiv rate limit: 1 request per 3 seconds
		rateLimiter: NewRateLimiter(1, 3*time.Second, false),
		fetcher:     pipeline.NewFetcher(5),
		scorer:      scoring.NewDefaultScorer(),
	}
}

// Name returns the hunter's identifier.
func (h *ArxivHunter) Name() string {
	return "arxiv-scout"
}

// Hunt performs the research collection from arXiv.
// It uses the 4-stage pipeline (Search → Fetch → Synthesize → Score) based on mode.
func (h *ArxivHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	// Configure synthesizer if pipeline options specify it
	if cfg.Pipeline.Synthesize && cfg.Pipeline.AgentCmd != "" {
		h.synthesizer = pipeline.NewSynthesizer(
			cfg.Pipeline.AgentCmd,
			cfg.Pipeline.AgentParallelism,
			cfg.Pipeline.AgentTimeout,
		)
	}

	// Determine pipeline mode
	mode := pipeline.ModeBalanced
	switch cfg.Mode {
	case "quick":
		mode = pipeline.ModeQuick
	case "deep":
		mode = pipeline.ModeDeep
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	var errors []error
	seen := make(map[string]bool)
	var allRawItems []pipeline.RawItem

	// Stage 1: SEARCH - Find papers matching queries
	for _, query := range cfg.Queries {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, ctx.Err()
		default:
		}

		if err := h.rateLimiter.Wait(ctx); err != nil {
			errors = append(errors, fmt.Errorf("rate limit wait for query %q: %w", query, err))
			continue
		}

		rawItems, err := h.searchToRawItems(ctx, query, cfg.Categories, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		// Deduplicate
		for _, item := range rawItems {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			allRawItems = append(allRawItems, item)
		}
	}

	if len(allRawItems) == 0 {
		result.Errors = errors
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Stage 2: FETCH - Get additional content (abstracts already included from search)
	fetchOpts := pipeline.FetchOpts{
		Mode:      mode,
		FetchDocs: true,
		Timeout:   30 * time.Second,
	}
	fetchedItems, err := h.fetcher.FetchBatch(ctx, allRawItems, fetchOpts)
	if err != nil {
		errors = append(errors, fmt.Errorf("fetch failed: %w", err))
		fetchedItems = make([]pipeline.FetchedItem, len(allRawItems))
		for i, item := range allRawItems {
			fetchedItems[i] = pipeline.FetchedItem{Raw: item, FetchSuccess: true}
		}
	}

	// Stage 3: SYNTHESIZE - Use agent to analyze papers (mode-dependent)
	var synthesizedItems []pipeline.SynthesizedItem
	if h.synthesizer != nil && mode != pipeline.ModeQuick {
		itemsToSynthesize := fetchedItems
		if mode == pipeline.ModeBalanced && cfg.Pipeline.SynthesizeLimit > 0 {
			if len(itemsToSynthesize) > cfg.Pipeline.SynthesizeLimit {
				itemsToSynthesize = itemsToSynthesize[:cfg.Pipeline.SynthesizeLimit]
			}
		}
		synthesizedItems, err = h.synthesizer.SynthesizeBatch(ctx, itemsToSynthesize, strings.Join(cfg.Queries, " "))
		if err != nil {
			errors = append(errors, fmt.Errorf("synthesize failed: %w", err))
			synthesizedItems = wrapWithoutSynthesis(fetchedItems)
		}
		if len(itemsToSynthesize) < len(fetchedItems) {
			remaining := wrapWithoutSynthesis(fetchedItems[len(itemsToSynthesize):])
			synthesizedItems = append(synthesizedItems, remaining...)
		}
	} else {
		synthesizedItems = wrapWithoutSynthesis(fetchedItems)
	}

	// Stage 4: SCORE - Calculate quality scores
	scoredItems := h.scorer.ScoreBatch(synthesizedItems, strings.Join(cfg.Queries, " "))

	// Save results with scores
	outputFile, err := h.saveResultsWithScores(cfg, scoredItems, cfg.Queries)
	if err != nil {
		errors = append(errors, fmt.Errorf("save results: %w", err))
	} else {
		result.OutputFiles = append(result.OutputFiles, outputFile)
	}

	result.SourcesCollected = len(scoredItems)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// searchToRawItems performs search and converts to RawItem format.
func (h *ArxivHunter) searchToRawItems(ctx context.Context, query string, categories []string, maxResults int) ([]pipeline.RawItem, error) {
	papers, err := h.searchArxiv(ctx, query, categories, maxResults)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	items := make([]pipeline.RawItem, len(papers))
	for i, paper := range papers {
		items[i] = pipeline.RawItem{
			ID:    "arxiv:" + paper.ArxivID,
			Type:  "arxiv_paper",
			Title: paper.Title,
			URL:   paper.URL,
			Metadata: map[string]any{
				"arxiv_id":   paper.ArxivID,
				"authors":    paper.Authors,
				"abstract":   paper.Abstract,
				"pdf_url":    paper.PDFURL,
				"published":  paper.Published,
				"categories": paper.Categories,
			},
			CollectedAt: now,
		}
	}
	return items, nil
}

// searchArxiv queries the arXiv API for papers matching the query.
func (h *ArxivHunter) searchArxiv(ctx context.Context, query string, categories []string, maxResults int) ([]ArxivPaper, error) {
	// Build the search query
	searchQuery := buildArxivQuery(query, categories)

	// Construct the API URL
	apiURL := fmt.Sprintf(
		"http://export.arxiv.org/api/query?search_query=%s&max_results=%d&sortBy=submittedDate&sortOrder=descending",
		url.QueryEscape(searchQuery),
		maxResults,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("arXiv API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseArxivResponse(body, query)
}

// buildArxivQuery constructs an arXiv API search query.
func buildArxivQuery(query string, categories []string) string {
	// Base query: search in title and abstract
	baseQuery := fmt.Sprintf("all:%s", query)

	if len(categories) == 0 {
		return baseQuery
	}

	// Add category filters with OR
	catParts := make([]string, len(categories))
	for i, cat := range categories {
		catParts[i] = fmt.Sprintf("cat:%s", cat)
	}
	catQuery := strings.Join(catParts, "+OR+")

	return fmt.Sprintf("(%s)+AND+(%s)", baseQuery, catQuery)
}

// parseArxivResponse parses the Atom/XML response from arXiv.
func parseArxivResponse(data []byte, originalQuery string) ([]ArxivPaper, error) {
	var feed arxivFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	papers := make([]ArxivPaper, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		paper := ArxivPaper{
			ArxivID:    extractArxivID(entry.ID),
			Title:      cleanText(entry.Title),
			Authors:    extractAuthors(entry.Authors),
			Abstract:   cleanText(entry.Summary),
			URL:        entry.ID,
			PDFURL:     extractPDFURL(entry.Links),
			Published:  parseArxivDate(entry.Published),
			Categories: extractCategories(entry.Categories),
			Relevance:  assessRelevance(entry, originalQuery),
			Signal:     generateSignal(entry, originalQuery),
		}
		papers = append(papers, paper)
	}

	return papers, nil
}

// arxivFeed represents the root Atom feed from arXiv.
type arxivFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

// arxivEntry represents a single paper entry in the Atom feed.
type arxivEntry struct {
	ID         string          `xml:"id"`
	Title      string          `xml:"title"`
	Summary    string          `xml:"summary"`
	Published  string          `xml:"published"`
	Updated    string          `xml:"updated"`
	Authors    []arxivAuthor   `xml:"author"`
	Categories []arxivCategory `xml:"category"`
	Links      []arxivLink     `xml:"link"`
}

// arxivAuthor represents an author in the feed.
type arxivAuthor struct {
	Name string `xml:"name"`
}

// arxivCategory represents a category in the feed.
type arxivCategory struct {
	Term string `xml:"term,attr"`
}

// arxivLink represents a link in the feed.
type arxivLink struct {
	Href  string `xml:"href,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
	Rel   string `xml:"rel,attr"`
}

// ArxivPaper represents a paper in the output YAML format.
type ArxivPaper struct {
	ArxivID    string   `yaml:"arxiv_id"`
	Title      string   `yaml:"title"`
	Authors    []string `yaml:"authors"`
	Abstract   string   `yaml:"abstract"`
	URL        string   `yaml:"url"`
	PDFURL     string   `yaml:"pdf_url"`
	Published  string   `yaml:"published"`
	Categories []string `yaml:"categories"`
	Relevance  string   `yaml:"relevance"`
	Signal     string   `yaml:"signal"`
}

// ArxivOutput represents the complete output YAML structure.
type ArxivOutput struct {
	Query       string       `yaml:"query"`
	CollectedAt time.Time    `yaml:"collected_at"`
	Papers      []ArxivPaper `yaml:"papers"`
}

// V2 output structures with pipeline data

type arxivOutputV2 struct {
	Query       string         `yaml:"query"`
	CollectedAt time.Time      `yaml:"collected_at"`
	Papers      []arxivPaperV2 `yaml:"papers"`
}

type arxivPaperV2 struct {
	ArxivID      string             `yaml:"arxiv_id"`
	Title        string             `yaml:"title"`
	Authors      []string           `yaml:"authors"`
	Abstract     string             `yaml:"abstract,omitempty"`
	URL          string             `yaml:"url"`
	PDFURL       string             `yaml:"pdf_url,omitempty"`
	Published    string             `yaml:"published"`
	Categories   []string           `yaml:"categories,omitempty"`
	QualityScore qualityScoreOutput `yaml:"quality_score"`
	Synthesis    *synthesisOutput   `yaml:"synthesis,omitempty"`
}

// extractArxivID extracts the arXiv ID from the full URL.
// Example: "http://arxiv.org/abs/2401.12345v1" -> "2401.12345"
func extractArxivID(idURL string) string {
	re := regexp.MustCompile(`arxiv\.org/abs/(\d+\.\d+)`)
	matches := re.FindStringSubmatch(idURL)
	if len(matches) > 1 {
		return matches[1]
	}
	// Fallback: try to extract from end of URL
	parts := strings.Split(idURL, "/")
	if len(parts) > 0 {
		id := parts[len(parts)-1]
		// Remove version suffix (v1, v2, etc.)
		if idx := strings.Index(id, "v"); idx > 0 {
			id = id[:idx]
		}
		return id
	}
	return idURL
}

// extractAuthors extracts author names from the entries.
func extractAuthors(authors []arxivAuthor) []string {
	names := make([]string, len(authors))
	for i, a := range authors {
		names[i] = cleanText(a.Name)
	}
	return names
}

// extractPDFURL finds the PDF link from the entry links.
func extractPDFURL(links []arxivLink) string {
	for _, link := range links {
		if link.Title == "pdf" || link.Type == "application/pdf" {
			return link.Href
		}
	}
	// Fallback: construct PDF URL from abs URL
	for _, link := range links {
		if strings.Contains(link.Href, "/abs/") {
			return strings.Replace(link.Href, "/abs/", "/pdf/", 1) + ".pdf"
		}
	}
	return ""
}

// extractCategories extracts category terms from the entry.
func extractCategories(cats []arxivCategory) []string {
	terms := make([]string, len(cats))
	for i, c := range cats {
		terms[i] = c.Term
	}
	return terms
}

// parseArxivDate parses and formats the arXiv date.
func parseArxivDate(dateStr string) string {
	// arXiv dates are in RFC3339 format
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("2006-01-02")
}

// cleanText removes excess whitespace and newlines from text.
func cleanText(s string) string {
	// Replace newlines and multiple spaces with single space
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	// Collapse multiple spaces
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}

// assessRelevance determines the relevance level of a paper.
func assessRelevance(entry arxivEntry, query string) string {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(entry.Title)
	summaryLower := strings.ToLower(entry.Summary)

	// Check for AI agent-related keywords
	agentKeywords := []string{"agent", "llm", "language model", "gpt", "claude", "autonomous", "reasoning", "tool use"}
	keywordHits := 0
	for _, kw := range agentKeywords {
		if strings.Contains(titleLower, kw) || strings.Contains(summaryLower, kw) {
			keywordHits++
		}
	}

	// Title match is highest priority
	if strings.Contains(titleLower, queryLower) {
		if keywordHits >= 2 {
			return "high"
		}
		return "medium"
	}

	// Abstract match with multiple keyword hits
	if strings.Contains(summaryLower, queryLower) && keywordHits >= 2 {
		return "medium"
	}

	// Some relevance
	if keywordHits >= 1 {
		return "medium"
	}

	return "low"
}

// generateSignal creates a brief relevance note for the paper.
func generateSignal(entry arxivEntry, query string) string {
	titleLower := strings.ToLower(entry.Title)
	summaryLower := strings.ToLower(entry.Summary)

	signals := []string{}

	// Check for specific topics
	topicSignals := map[string]string{
		"agent":        "agent architecture",
		"llm":          "LLM-based approach",
		"reasoning":    "reasoning capabilities",
		"tool":         "tool use/integration",
		"multi-agent":  "multi-agent systems",
		"autonomous":   "autonomous operation",
		"benchmark":    "benchmark/evaluation",
		"code":         "code generation",
		"planning":     "planning capabilities",
		"memory":       "memory systems",
		"retrieval":    "retrieval augmentation",
		"fine-tun":     "fine-tuning approach",
		"prompt":       "prompting techniques",
		"chain":        "chain-of-thought",
		"reinforcement": "RL integration",
	}

	for keyword, signal := range topicSignals {
		if strings.Contains(titleLower, keyword) || strings.Contains(summaryLower, keyword) {
			signals = append(signals, signal)
			if len(signals) >= 2 {
				break
			}
		}
	}

	if len(signals) == 0 {
		return fmt.Sprintf("Matches query: %s", query)
	}

	return strings.Join(signals, ", ")
}

// saveResultsWithScores saves scored papers to a YAML file.
func (h *ArxivHunter) saveResultsWithScores(cfg HunterConfig, items []pipeline.ScoredItem, queries []string) (string, error) {
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/research"
	}

	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	filename := fmt.Sprintf("%s-arxiv.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	output := arxivOutputV2{
		Query:       strings.Join(queries, ", "),
		CollectedAt: time.Now().UTC(),
		Papers:      make([]arxivPaperV2, 0, len(items)),
	}

	for _, item := range items {
		raw := item.Synthesized.Fetched.Raw
		arxivID, _ := raw.Metadata["arxiv_id"].(string)
		authors, _ := raw.Metadata["authors"].([]string)
		abstract, _ := raw.Metadata["abstract"].(string)
		pdfURL, _ := raw.Metadata["pdf_url"].(string)
		published, _ := raw.Metadata["published"].(string)
		categories, _ := raw.Metadata["categories"].([]string)

		paper := arxivPaperV2{
			ArxivID:    arxivID,
			Title:      raw.Title,
			Authors:    authors,
			Abstract:   abstract,
			URL:        raw.URL,
			PDFURL:     pdfURL,
			Published:  published,
			Categories: categories,
			QualityScore: qualityScoreOutput{
				Value:      item.Score.Value,
				Level:      item.Score.Level,
				Factors:    item.Score.Factors,
				Confidence: item.Score.Confidence,
			},
		}

		synthesis := item.Synthesized.Synthesis
		if synthesis.Summary != "" {
			paper.Synthesis = &synthesisOutput{
				Summary:            synthesis.Summary,
				KeyFeatures:        synthesis.KeyFeatures,
				RelevanceRationale: synthesis.RelevanceRationale,
				Recommendations:    synthesis.Recommendations,
				Confidence:         synthesis.Confidence,
			}
		}

		output.Papers = append(output.Papers, paper)
	}

	data, err := yaml.Marshal(&output)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fullPath, nil
}

// saveResults saves the collected papers to a YAML file (legacy).
func (h *ArxivHunter) saveResults(cfg HunterConfig, papers []ArxivPaper, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/research"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-arxiv.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := ArxivOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Papers:      papers,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&output)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fullPath, nil
}
