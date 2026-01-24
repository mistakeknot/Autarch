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

	"gopkg.in/yaml.v3"
)

// OpenAlexHunter searches academic papers from OpenAlex.
// OpenAlex indexes 260M+ works across all academic disciplines.
type OpenAlexHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	email       string // For polite pool access (faster rate limits)
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
	}
}

// Name returns the hunter's identifier.
func (h *OpenAlexHunter) Name() string {
	return "openalex"
}

// Hunt performs the research collection from OpenAlex.
func (h *OpenAlexHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	var allWorks []OpenAlexWork
	var errors []error

	for _, query := range cfg.Queries {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, ctx.Err()
		default:
		}

		// Wait for rate limiter
		if err := h.rateLimiter.Wait(ctx); err != nil {
			errors = append(errors, fmt.Errorf("rate limit wait for query %q: %w", query, err))
			continue
		}

		works, err := h.searchOpenAlex(ctx, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		allWorks = append(allWorks, works...)
	}

	// Deduplicate works by OpenAlex ID
	seen := make(map[string]bool)
	uniqueWorks := make([]OpenAlexWork, 0, len(allWorks))
	for _, w := range allWorks {
		if !seen[w.ID] {
			seen[w.ID] = true
			uniqueWorks = append(uniqueWorks, w)
		}
	}

	// Save results
	if len(uniqueWorks) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueWorks, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueWorks)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
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

// saveResults saves the collected works to a YAML file.
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
