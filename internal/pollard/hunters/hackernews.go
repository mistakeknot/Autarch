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

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/scoring"
	"gopkg.in/yaml.v3"
)

const (
	hackerNewsName    = "hackernews-trendwatcher"
	hnAlgoliaAPI      = "https://hn.algolia.com/api/v1/search"
	hnItemURL         = "https://news.ycombinator.com/item?id="
	defaultHitsPerPage = 50
)

// HackerNewsHunter searches HackerNews for trending discussions.
// It implements the 4-stage pipeline: Search → Fetch → Synthesize → Score.
type HackerNewsHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	fetcher     *pipeline.Fetcher
	synthesizer *pipeline.Synthesizer
	scorer      *scoring.Scorer
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
		fetcher:     pipeline.NewFetcher(5),
		scorer:      scoring.NewDefaultScorer(),
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

// V2 output structures with pipeline data

type hnTrendsOutputV2 struct {
	CollectedAt time.Time   `yaml:"collected_at"`
	Trends      []hnTrendV2 `yaml:"trends"`
}

type hnTrendV2 struct {
	Title        string             `yaml:"title"`
	Source       string             `yaml:"source"`
	URL          string             `yaml:"url"`
	StoryURL     string             `yaml:"story_url,omitempty"`
	Points       int                `yaml:"points"`
	Comments     int                `yaml:"comments"`
	Author       string             `yaml:"author"`
	CreatedAt    time.Time          `yaml:"created_at"`
	QualityScore qualityScoreOutput `yaml:"quality_score"`
	Synthesis    *synthesisOutput   `yaml:"synthesis,omitempty"`
}

// Hunt performs the HackerNews research collection.
// It uses the 4-stage pipeline (Search → Fetch → Synthesize → Score) based on mode.
func (h *HackerNewsHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
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

	seenIDs := make(map[string]bool)
	var allRawItems []pipeline.RawItem

	// Stage 1: SEARCH - Find stories matching queries
	for _, query := range cfg.Queries {
		if err := h.rateLimiter.Wait(ctx); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("rate limit wait: %w", err))
			continue
		}

		rawItems, err := h.searchToRawItems(ctx, query, cfg.MaxResults, cfg.MinPoints)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		// Deduplicate
		for _, item := range rawItems {
			if seenIDs[item.ID] {
				continue
			}
			seenIDs[item.ID] = true
			allRawItems = append(allRawItems, item)
		}
	}

	if len(allRawItems) == 0 {
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Stage 2: FETCH - Get additional content (story text)
	fetchOpts := pipeline.FetchOpts{
		Mode:    mode,
		Timeout: 30 * time.Second,
	}
	fetchedItems, err := h.fetcher.FetchBatch(ctx, allRawItems, fetchOpts)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("fetch failed: %w", err))
		fetchedItems = make([]pipeline.FetchedItem, len(allRawItems))
		for i, item := range allRawItems {
			fetchedItems[i] = pipeline.FetchedItem{Raw: item, FetchSuccess: true}
		}
	}

	// Stage 3: SYNTHESIZE - Use agent to analyze items (mode-dependent)
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
			result.Errors = append(result.Errors, fmt.Errorf("synthesize failed: %w", err))
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

	// Write output file with scores
	outputPath, err := h.writeOutputWithScores(cfg, scoredItems)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("write output: %w", err))
	} else {
		result.OutputFiles = append(result.OutputFiles, outputPath)
		result.InsightsCreated = len(scoredItems)
	}

	result.SourcesCollected = len(scoredItems)
	result.CompletedAt = time.Now()
	return result, nil
}

// searchToRawItems performs search and converts to RawItem format.
func (h *HackerNewsHunter) searchToRawItems(ctx context.Context, query string, maxResults, minPoints int) ([]pipeline.RawItem, error) {
	hits, err := h.search(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	items := make([]pipeline.RawItem, 0, len(hits))
	for _, hit := range hits {
		if minPoints > 0 && hit.Points < minPoints {
			continue
		}

		items = append(items, pipeline.RawItem{
			ID:    "hn:" + hit.ObjectID,
			Type:  "hn_story",
			Title: hit.Title,
			URL:   fmt.Sprintf("%s%s", hnItemURL, hit.ObjectID),
			Metadata: map[string]any{
				"points":     hit.Points,
				"comments":   hit.NumComments,
				"author":     hit.Author,
				"created_at": hit.CreatedAt,
				"story_text": hit.StoryText,
				"story_url":  hit.URL,
			},
			CollectedAt: now,
		})
	}
	return items, nil
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

// writeOutputWithScores writes scored trends to a YAML file.
func (h *HackerNewsHunter) writeOutputWithScores(cfg HunterConfig, items []pipeline.ScoredItem) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "insights/trends"
	}

	if cfg.ProjectPath != "" {
		outputDir = filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Create output structure
	output := hnTrendsOutputV2{
		CollectedAt: time.Now().UTC(),
		Trends:      make([]hnTrendV2, 0, len(items)),
	}

	for _, item := range items {
		raw := item.Synthesized.Fetched.Raw
		points, _ := raw.Metadata["points"].(int)
		comments, _ := raw.Metadata["comments"].(int)
		author, _ := raw.Metadata["author"].(string)
		createdAt, _ := raw.Metadata["created_at"].(time.Time)
		storyURL, _ := raw.Metadata["story_url"].(string)

		trend := hnTrendV2{
			Title:     raw.Title,
			Source:    "hackernews",
			URL:       raw.URL,
			StoryURL:  storyURL,
			Points:    points,
			Comments:  comments,
			Author:    author,
			CreatedAt: createdAt,
			QualityScore: qualityScoreOutput{
				Value:      item.Score.Value,
				Level:      item.Score.Level,
				Factors:    item.Score.Factors,
				Confidence: item.Score.Confidence,
			},
		}

		// Add synthesis if available
		synthesis := item.Synthesized.Synthesis
		if synthesis.Summary != "" {
			trend.Synthesis = &synthesisOutput{
				Summary:            synthesis.Summary,
				KeyFeatures:        synthesis.KeyFeatures,
				RelevanceRationale: synthesis.RelevanceRationale,
				Recommendations:    synthesis.Recommendations,
				Confidence:         synthesis.Confidence,
			}
		}

		output.Trends = append(output.Trends, trend)
	}

	filename := fmt.Sprintf("%s-hackernews.yaml", time.Now().Format("2006-01-02"))
	outputPath := filepath.Join(outputDir, filename)

	data, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return outputPath, nil
}

// writeOutput writes the trends to a YAML file (legacy).
func (h *HackerNewsHunter) writeOutput(cfg HunterConfig, output hnTrendsOutput) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "insights/trends"
	}

	// Make path absolute if project path is provided, always under .pollard/
	if cfg.ProjectPath != "" {
		outputDir = filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
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
