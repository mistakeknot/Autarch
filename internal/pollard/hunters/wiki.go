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

// WikiHunter searches Wikipedia and Wikidata for general knowledge.
// Provides access to millions of entities across all domains.
type WikiHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
}

// NewWikiHunter creates a new Wikipedia/Wikidata research hunter.
func NewWikiHunter() *WikiHunter {
	return &WikiHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Wikipedia rate limit: 200 req/s but be polite
		rateLimiter: NewRateLimiter(5, time.Second, false),
	}
}

// Name returns the hunter's identifier.
func (h *WikiHunter) Name() string {
	return "wiki"
}

// Hunt performs the research collection from Wikipedia and Wikidata.
func (h *WikiHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 20
	}

	var allEntities []WikiEntity
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

		// Search Wikipedia
		wikiEntities, err := h.searchWikipedia(ctx, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("wikipedia search %q: %w", query, err))
		} else {
			allEntities = append(allEntities, wikiEntities...)
		}

		// Also search Wikidata for structured data
		if err := h.rateLimiter.Wait(ctx); err != nil {
			continue
		}

		wdEntities, err := h.searchWikidata(ctx, query, maxResults/2)
		if err != nil {
			errors = append(errors, fmt.Errorf("wikidata search %q: %w", query, err))
		} else {
			allEntities = append(allEntities, wdEntities...)
		}
	}

	// Deduplicate entities by title
	seen := make(map[string]bool)
	uniqueEntities := make([]WikiEntity, 0, len(allEntities))
	for _, e := range allEntities {
		key := strings.ToLower(e.Title)
		if !seen[key] {
			seen[key] = true
			uniqueEntities = append(uniqueEntities, e)
		}
	}

	// Save results
	if len(uniqueEntities) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueEntities, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueEntities)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// searchWikipedia searches Wikipedia for articles matching the query.
func (h *WikiHunter) searchWikipedia(ctx context.Context, query string, maxResults int) ([]WikiEntity, error) {
	// Use Wikipedia's OpenSearch API
	apiURL := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&srlimit=%d&format=json&srprop=snippet|titlesnippet",
		url.QueryEscape(query),
		maxResults,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Be a good citizen
	req.Header.Set("User-Agent", "Pollard/1.0 (research tool; contact: pollard@example.com)")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wikipedia API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseWikipediaResponse(body)
}

// wikipediaResponse represents the Wikipedia API response.
type wikipediaResponse struct {
	Query struct {
		Search []struct {
			PageID  int    `json:"pageid"`
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"query"`
}

// parseWikipediaResponse parses the JSON response from Wikipedia.
func parseWikipediaResponse(data []byte) ([]WikiEntity, error) {
	var response wikipediaResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	entities := make([]WikiEntity, 0, len(response.Query.Search))
	for _, r := range response.Query.Search {
		// Clean HTML from snippet
		description := cleanHTML(r.Snippet)

		entities = append(entities, WikiEntity{
			Title:        r.Title,
			Description:  description,
			WikipediaURL: fmt.Sprintf("https://en.wikipedia.org/wiki/%s", url.PathEscape(strings.ReplaceAll(r.Title, " ", "_"))),
			Source:       "wikipedia",
		})
	}

	return entities, nil
}

// searchWikidata searches Wikidata for entities matching the query.
func (h *WikiHunter) searchWikidata(ctx context.Context, query string, maxResults int) ([]WikiEntity, error) {
	// Use Wikidata's wbsearchentities API
	apiURL := fmt.Sprintf(
		"https://www.wikidata.org/w/api.php?action=wbsearchentities&search=%s&language=en&limit=%d&format=json",
		url.QueryEscape(query),
		maxResults,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Pollard/1.0 (research tool; contact: pollard@example.com)")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wikidata API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseWikidataResponse(body)
}

// wikidataResponse represents the Wikidata API response.
type wikidataResponse struct {
	Search []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Label       string `json:"label"`
		Description string `json:"description"`
		URL         string `json:"url"`
	} `json:"search"`
}

// parseWikidataResponse parses the JSON response from Wikidata.
func parseWikidataResponse(data []byte) ([]WikiEntity, error) {
	var response wikidataResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	entities := make([]WikiEntity, 0, len(response.Search))
	for _, r := range response.Search {
		entities = append(entities, WikiEntity{
			QID:         r.ID,
			Title:       r.Label,
			Description: r.Description,
			URL:         r.URL,
			Source:      "wikidata",
		})
	}

	return entities, nil
}

// WikiEntity represents a Wikipedia/Wikidata entity in the output YAML format.
type WikiEntity struct {
	QID          string            `yaml:"qid,omitempty"`
	Title        string            `yaml:"title"`
	Description  string            `yaml:"description"`
	URL          string            `yaml:"url,omitempty"`
	WikipediaURL string            `yaml:"wikipedia_url,omitempty"`
	Properties   map[string]string `yaml:"properties,omitempty"`
	Source       string            `yaml:"source"` // wikipedia or wikidata
}

// WikiOutput represents the complete output YAML structure.
type WikiOutput struct {
	Query       string       `yaml:"query"`
	CollectedAt time.Time    `yaml:"collected_at"`
	Entities    []WikiEntity `yaml:"entities"`
}

// saveResults saves the collected entities to a YAML file.
func (h *WikiHunter) saveResults(cfg HunterConfig, entities []WikiEntity, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/wiki"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-wiki.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := WikiOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Entities:    entities,
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
