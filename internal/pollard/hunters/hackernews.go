// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	hackerNewsName    = "hackernews-trendwatcher"
	hnAlgoliaAPI      = "https://hn.algolia.com/api/v1/search"
	hnItemURL         = "https://news.ycombinator.com/item?id="
	defaultHitsPerPage = 50
)

// HackerNewsHunter searches HackerNews for trending discussions.
type HackerNewsHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
}

// NewHackerNewsHunter creates a new HackerNews hunter.
func NewHackerNewsHunter() *HackerNewsHunter {
	return &HackerNewsHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// HN Algolia API is generous: ~10,000 requests/hour
		// Use conservative limit: 60 requests per minute
		rateLimiter: NewRateLimiter(60, time.Minute, false),
	}
}

// Name returns the hunter's identifier.
func (h *HackerNewsHunter) Name() string {
	return hackerNewsName
}

// hnAlgoliaResponse represents the Algolia API response.
type hnAlgoliaResponse struct {
	Hits             []hnHit `json:"hits"`
	NbHits           int     `json:"nbHits"`
	Page             int     `json:"page"`
	NbPages          int     `json:"nbPages"`
	HitsPerPage      int     `json:"hitsPerPage"`
	ProcessingTimeMS int     `json:"processingTimeMS"`
}

// hnHit represents a single story from the Algolia response.
type hnHit struct {
	ObjectID    string    `json:"objectID"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Author      string    `json:"author"`
	Points      int       `json:"points"`
	NumComments int       `json:"num_comments"`
	CreatedAt   time.Time `json:"created_at"`
	StoryText   string    `json:"story_text,omitempty"`
}

// hnTrend represents a single trend in the output YAML.
type hnTrend struct {
	Title     string    `yaml:"title"`
	Source    string    `yaml:"source"`
	URL       string    `yaml:"url"`
	Points    int       `yaml:"points"`
	Comments  int       `yaml:"comments"`
	Author    string    `yaml:"author"`
	CreatedAt time.Time `yaml:"created_at"`
	Relevance string    `yaml:"relevance"`
	Signal    string    `yaml:"signal"`
}

// hnTrendsOutput represents the output YAML structure.
type hnTrendsOutput struct {
	CollectedAt time.Time `yaml:"collected_at"`
	Trends      []hnTrend `yaml:"trends"`
}

// Hunt performs the HackerNews research collection.
func (h *HackerNewsHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	var allHits []hnHit
	seenIDs := make(map[string]bool)

	// Search for each query
	for _, query := range cfg.Queries {
		if err := h.rateLimiter.Wait(ctx); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("rate limit wait: %w", err))
			continue
		}

		hits, err := h.search(ctx, query, cfg.MaxResults)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		// Deduplicate and filter by MinPoints
		for _, hit := range hits {
			if seenIDs[hit.ObjectID] {
				continue
			}
			if cfg.MinPoints > 0 && hit.Points < cfg.MinPoints {
				continue
			}
			seenIDs[hit.ObjectID] = true
			allHits = append(allHits, hit)
		}

		result.SourcesCollected += len(hits)
	}

	// Convert hits to trends
	trends := h.convertToTrends(allHits, cfg.Queries)

	// Prepare output
	output := hnTrendsOutput{
		CollectedAt: time.Now().UTC(),
		Trends:      trends,
	}

	// Write output file
	outputPath, err := h.writeOutput(cfg, output)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("write output: %w", err))
	} else {
		result.OutputFiles = append(result.OutputFiles, outputPath)
		result.InsightsCreated = len(trends)
	}

	result.CompletedAt = time.Now()
	return result, nil
}

// search performs a single search query against the Algolia API.
func (h *HackerNewsHunter) search(ctx context.Context, query string, maxResults int) ([]hnHit, error) {
	hitsPerPage := defaultHitsPerPage
	if maxResults > 0 && maxResults < hitsPerPage {
		hitsPerPage = maxResults
	}

	// Build request URL
	params := url.Values{}
	params.Set("query", query)
	params.Set("tags", "story")
	params.Set("hitsPerPage", fmt.Sprintf("%d", hitsPerPage))

	reqURL := fmt.Sprintf("%s?%s", hnAlgoliaAPI, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Pollard/1.0 (HackerNews TrendWatcher)")
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var algoliaResp hnAlgoliaResponse
	if err := json.NewDecoder(resp.Body).Decode(&algoliaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return algoliaResp.Hits, nil
}

// convertToTrends converts Algolia hits to trend entries.
func (h *HackerNewsHunter) convertToTrends(hits []hnHit, queries []string) []hnTrend {
	trends := make([]hnTrend, 0, len(hits))

	for _, hit := range hits {
		// Determine the story URL (use HN discussion if no external URL)
		storyURL := hit.URL
		if storyURL == "" {
			storyURL = fmt.Sprintf("%s%s", hnItemURL, hit.ObjectID)
		}

		trend := hnTrend{
			Title:     hit.Title,
			Source:    "hackernews",
			URL:       fmt.Sprintf("%s%s", hnItemURL, hit.ObjectID),
			Points:    hit.Points,
			Comments:  hit.NumComments,
			Author:    hit.Author,
			CreatedAt: hit.CreatedAt,
			Relevance: h.calculateRelevance(hit),
			Signal:    h.generateSignal(hit, queries),
		}

		trends = append(trends, trend)
	}

	return trends
}

// calculateRelevance determines the relevance level based on points and comments.
func (h *HackerNewsHunter) calculateRelevance(hit hnHit) string {
	// High engagement = high relevance
	if hit.Points >= 500 || hit.NumComments >= 200 {
		return "high"
	}
	if hit.Points >= 100 || hit.NumComments >= 50 {
		return "medium"
	}
	return "low"
}

// generateSignal creates a brief description of why this trend matters.
func (h *HackerNewsHunter) generateSignal(hit hnHit, queries []string) string {
	var signals []string

	// Engagement signals
	if hit.Points >= 500 {
		signals = append(signals, "viral discussion")
	} else if hit.Points >= 100 {
		signals = append(signals, "popular discussion")
	}

	if hit.NumComments >= 200 {
		signals = append(signals, "highly commented")
	} else if hit.NumComments >= 50 {
		signals = append(signals, "active discussion")
	}

	// Match signals - which query matched
	titleLower := strings.ToLower(hit.Title)
	for _, q := range queries {
		if strings.Contains(titleLower, strings.ToLower(q)) {
			signals = append(signals, fmt.Sprintf("matches query '%s'", q))
			break
		}
	}

	if len(signals) == 0 {
		return "Trending story on HackerNews"
	}

	return strings.Join(signals, "; ")
}

// writeOutput writes the trends to a YAML file.
func (h *HackerNewsHunter) writeOutput(cfg HunterConfig, output hnTrendsOutput) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "insights/trends"
	}

	// Make path absolute if project path is provided
	if cfg.ProjectPath != "" {
		outputDir = filepath.Join(cfg.ProjectPath, outputDir)
	}

	// Create directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Generate filename: YYYY-MM-DD-hackernews.yaml
	filename := fmt.Sprintf("%s-hackernews.yaml", time.Now().Format("2006-01-02"))
	outputPath := filepath.Join(outputDir, filename)

	// Marshal to YAML
	data, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return outputPath, nil
}
