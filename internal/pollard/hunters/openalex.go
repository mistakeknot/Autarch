// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/scoring"
	"gopkg.in/yaml.v3"
)

// OpenAlexHunter searches academic papers from OpenAlex.
// OpenAlex indexes 260M+ works across all academic disciplines.
// It implements the 4-stage pipeline: Search → Fetch → Synthesize → Score.
type OpenAlexHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	email       string // For polite pool access (faster rate limits)
	fetcher     *pipeline.Fetcher
	synthesizer *pipeline.Synthesizer
	scorer      *scoring.Scorer
}

// NewOpenAlexHunter creates a new OpenAlex research hunter.
func NewOpenAlexHunter() *OpenAlexHunter {
	email := os.Getenv("OPENALEX_EMAIL")
	return &OpenAlexHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// OpenAlex rate limit: 10 req/s for polite pool, 100k/day
		rateLimiter: NewRateLimiter(10, time.Second, email != ""),
		email:       email,
		fetcher:     pipeline.NewFetcher(5),
		scorer:      scoring.NewDefaultScorer(),
	}
}

// Name returns the hunter's identifier.
func (h *OpenAlexHunter) Name() string {
	return "openalex"
}

// Hunt performs the research collection from OpenAlex.
// It uses the 4-stage pipeline (Search → Fetch → Synthesize → Score) based on mode.
func (h *OpenAlexHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
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
		maxResults = 100
	}

	var errors []error
	seen := make(map[string]bool)
	var allRawItems []pipeline.RawItem

	// Stage 1: SEARCH - Find works matching queries
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

		rawItems, err := h.searchToRawItems(ctx, query, maxResults)
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

	// Stage 2: FETCH - Get additional content (abstracts may not be in search results)
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

	// Stage 3: SYNTHESIZE - Use agent to analyze works (mode-dependent)
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
func (h *OpenAlexHunter) searchToRawItems(ctx context.Context, query string, maxResults int) ([]pipeline.RawItem, error) {
	works, err := h.searchOpenAlex(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	items := make([]pipeline.RawItem, len(works))
	for i, work := range works {
		items[i] = pipeline.RawItem{
			ID:    work.ID,
			Type:  "openalex_work",
			Title: work.Title,
			URL:   work.URL,
			Metadata: map[string]any{
				"doi":          work.DOI,
				"authors":      work.Authors,
				"published_at": work.PublishedAt,
				"journal":      work.Journal,
				"citations":    work.Citations,
				"open_access":  work.OpenAccess,
				"pdf_url":      work.PDFURL,
				"topics":       work.Topics,
			},
			CollectedAt: now,
		}
	}
	return items, nil
}

// searchOpenAlex queries the OpenAlex API for works matching the query.
func (h *OpenAlexHunter) searchOpenAlex(ctx context.Context, query string, maxResults int) ([]OpenAlexWork, error) {
	// Construct the API URL
	apiURL := fmt.Sprintf(
		"https://api.openalex.org/works?search=%s&per_page=%d&sort=cited_by_count:desc",
		url.QueryEscape(query),
		min(maxResults, 200), // OpenAlex max is 200 per page
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add email for polite pool (faster rate limits)
	if h.email != "" {
		req.Header.Set("User-Agent", fmt.Sprintf("mailto:%s", h.email))
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAlex API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseOpenAlexResponse(body, query)
}

// openAlexResponse represents the API response structure.
type openAlexResponse struct {
	Results []openAlexResult `json:"results"`
}

// openAlexResult represents a single work in the response.
type openAlexResult struct {
	ID              string            `json:"id"`
	DOI             string            `json:"doi"`
	Title           string            `json:"title"`
	DisplayName     string            `json:"display_name"`
	PublicationDate string            `json:"publication_date"`
	CitedByCount    int               `json:"cited_by_count"`
	IsOpenAccess    bool              `json:"is_oa"`
	OpenAccess      openAlexOA        `json:"open_access"`
	Authorships     []openAlexAuthor  `json:"authorships"`
	PrimaryLocation *openAlexLocation `json:"primary_location"`
	Topics          []openAlexTopic   `json:"topics"`
	// abstract_inverted_index is a map, not a string - we ignore it
}

type openAlexOA struct {
	IsOA  bool   `json:"is_oa"`
	OAURL string `json:"oa_url"`
}

type openAlexAuthor struct {
	Author struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

type openAlexLocation struct {
	Source *struct {
		DisplayName string `json:"display_name"`
	} `json:"source"`
	PDFURL string `json:"pdf_url"`
}

type openAlexTopic struct {
	DisplayName string `json:"display_name"`
}

// parseOpenAlexResponse parses the JSON response from OpenAlex.
func parseOpenAlexResponse(data []byte, originalQuery string) ([]OpenAlexWork, error) {
	var response openAlexResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	works := make([]OpenAlexWork, 0, len(response.Results))
	for _, r := range response.Results {
		// Extract authors
		authors := make([]string, 0, len(r.Authorships))
		for _, a := range r.Authorships {
			if a.Author.DisplayName != "" {
				authors = append(authors, a.Author.DisplayName)
			}
		}

		// Extract topics
		topics := make([]string, 0, len(r.Topics))
		for _, t := range r.Topics {
			topics = append(topics, t.DisplayName)
		}

		// Extract journal
		var journal string
		if r.PrimaryLocation != nil && r.PrimaryLocation.Source != nil {
			journal = r.PrimaryLocation.Source.DisplayName
		}

		// Extract PDF URL
		var pdfURL string
		if r.PrimaryLocation != nil {
			pdfURL = r.PrimaryLocation.PDFURL
		}
		if pdfURL == "" && r.OpenAccess.OAURL != "" {
			pdfURL = r.OpenAccess.OAURL
		}

		// Parse publication date
		var pubDate time.Time
		if r.PublicationDate != "" {
			pubDate, _ = time.Parse("2006-01-02", r.PublicationDate)
		}

		// Use display_name if title is empty
		title := r.Title
		if title == "" {
			title = r.DisplayName
		}

		// Clean up DOI (remove https://doi.org/ prefix if present)
		doi := r.DOI
		doi = strings.TrimPrefix(doi, "https://doi.org/")

		work := OpenAlexWork{
			ID:          r.ID,
			DOI:         doi,
			Title:       title,
			Authors:     authors,
			PublishedAt: pubDate.Format("2006-01-02"),
			Journal:     journal,
			Citations:   r.CitedByCount,
			OpenAccess:  r.OpenAccess.IsOA || r.IsOpenAccess,
			URL:         r.ID, // OpenAlex URL
			PDFURL:      pdfURL,
			Topics:      topics,
			Relevance:   assessOpenAlexRelevance(r, originalQuery),
		}
		works = append(works, work)
	}

	return works, nil
}

// assessOpenAlexRelevance determines the relevance level of a work.
func assessOpenAlexRelevance(r openAlexResult, query string) string {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(r.Title)

	// High citations is a good signal
	if r.CitedByCount >= 100 {
		return "high"
	}

	// Title match with decent citations
	if strings.Contains(titleLower, queryLower) && r.CitedByCount >= 10 {
		return "high"
	}

	// Title match or good citations
	if strings.Contains(titleLower, queryLower) || r.CitedByCount >= 50 {
		return "medium"
	}

	return "low"
}

// OpenAlexWork represents a work in the output YAML format.
type OpenAlexWork struct {
	ID          string   `yaml:"id"`
	DOI         string   `yaml:"doi,omitempty"`
	Title       string   `yaml:"title"`
	Authors     []string `yaml:"authors"`
	PublishedAt string   `yaml:"published_at"`
	Journal     string   `yaml:"journal,omitempty"`
	Citations   int      `yaml:"citations"`
	OpenAccess  bool     `yaml:"open_access"`
	URL         string   `yaml:"url"`
	PDFURL      string   `yaml:"pdf_url,omitempty"`
	Topics      []string `yaml:"topics,omitempty"`
	Relevance   string   `yaml:"relevance"`
}

// OpenAlexOutput represents the complete output YAML structure.
type OpenAlexOutput struct {
	Query       string         `yaml:"query"`
	CollectedAt time.Time      `yaml:"collected_at"`
	Works       []OpenAlexWork `yaml:"works"`
}

// V2 output structures with pipeline data

type openAlexOutputV2 struct {
	Query       string           `yaml:"query"`
	CollectedAt time.Time        `yaml:"collected_at"`
	Works       []openAlexWorkV2 `yaml:"works"`
}

type openAlexWorkV2 struct {
	ID           string             `yaml:"id"`
	DOI          string             `yaml:"doi,omitempty"`
	Title        string             `yaml:"title"`
	Authors      []string           `yaml:"authors"`
	PublishedAt  string             `yaml:"published_at"`
	Journal      string             `yaml:"journal,omitempty"`
	Citations    int                `yaml:"citations"`
	OpenAccess   bool               `yaml:"open_access"`
	URL          string             `yaml:"url"`
	PDFURL       string             `yaml:"pdf_url,omitempty"`
	Topics       []string           `yaml:"topics,omitempty"`
	QualityScore qualityScoreOutput `yaml:"quality_score"`
	Synthesis    *synthesisOutput   `yaml:"synthesis,omitempty"`
}

// saveResultsWithScores saves scored works to a YAML file.
func (h *OpenAlexHunter) saveResultsWithScores(cfg HunterConfig, items []pipeline.ScoredItem, queries []string) (string, error) {
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/openalex"
	}

	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	filename := fmt.Sprintf("%s-openalex.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	output := openAlexOutputV2{
		Query:       strings.Join(queries, ", "),
		CollectedAt: time.Now().UTC(),
		Works:       make([]openAlexWorkV2, 0, len(items)),
	}

	for _, item := range items {
		raw := item.Synthesized.Fetched.Raw
		doi, _ := raw.Metadata["doi"].(string)
		authors, _ := raw.Metadata["authors"].([]string)
		publishedAt, _ := raw.Metadata["published_at"].(string)
		journal, _ := raw.Metadata["journal"].(string)
		citations, _ := raw.Metadata["citations"].(int)
		openAccess, _ := raw.Metadata["open_access"].(bool)
		pdfURL, _ := raw.Metadata["pdf_url"].(string)
		topics, _ := raw.Metadata["topics"].([]string)

		work := openAlexWorkV2{
			ID:          raw.ID,
			DOI:         doi,
			Title:       raw.Title,
			Authors:     authors,
			PublishedAt: publishedAt,
			Journal:     journal,
			Citations:   citations,
			OpenAccess:  openAccess,
			URL:         raw.URL,
			PDFURL:      pdfURL,
			Topics:      topics,
			QualityScore: qualityScoreOutput{
				Value:      item.Score.Value,
				Level:      item.Score.Level,
				Factors:    item.Score.Factors,
				Confidence: item.Score.Confidence,
			},
		}

		synthesis := item.Synthesized.Synthesis
		if synthesis.Summary != "" {
			work.Synthesis = &synthesisOutput{
				Summary:            synthesis.Summary,
				KeyFeatures:        synthesis.KeyFeatures,
				RelevanceRationale: synthesis.RelevanceRationale,
				Recommendations:    synthesis.Recommendations,
				Confidence:         synthesis.Confidence,
			}
		}

		output.Works = append(output.Works, work)
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

// saveResults saves the collected works to a YAML file (legacy).
func (h *OpenAlexHunter) saveResults(cfg HunterConfig, works []OpenAlexWork, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/openalex"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-openalex.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := OpenAlexOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Works:       works,
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
