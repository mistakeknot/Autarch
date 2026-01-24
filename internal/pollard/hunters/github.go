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
	"regexp"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/pipeline"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/scoring"
	"gopkg.in/yaml.v3"
)

// GitHubScout searches GitHub for relevant repositories using the Search API.
// It implements the 4-stage pipeline: Search → Fetch → Synthesize → Score.
type GitHubScout struct {
	client      *http.Client
	rateLimiter *RateLimiter
	fetcher     *pipeline.Fetcher
	synthesizer *pipeline.Synthesizer
	scorer      *scoring.Scorer
}

// NewGitHubScout creates a new GitHub Scout hunter.
func NewGitHubScout() *GitHubScout {
	return &GitHubScout{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		fetcher: pipeline.NewFetcher(5),
		scorer:  scoring.NewDefaultScorer(),
	}
}

// Name returns the hunter's identifier.
func (g *GitHubScout) Name() string {
	return "github-scout"
}

// Hunt performs GitHub repository search based on the configuration.
// It uses the 4-stage pipeline (Search → Fetch → Synthesize → Score) based on mode.
func (g *GitHubScout) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: g.Name(),
		StartedAt:  time.Now(),
	}

	// Determine if we have authentication
	token := cfg.APIToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	authenticated := token != ""

	// Set up rate limiter based on authentication status
	// Unauthenticated: 10 requests per minute for search API
	// Authenticated: 30 requests per minute for search API
	if authenticated {
		g.rateLimiter = NewRateLimiter(30, time.Minute, true)
	} else {
		g.rateLimiter = NewRateLimiter(10, time.Minute, false)
	}

	// Configure synthesizer if pipeline options specify it
	if cfg.Pipeline.Synthesize && cfg.Pipeline.AgentCmd != "" {
		g.synthesizer = pipeline.NewSynthesizer(
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

	// Ensure output directory exists
	outputDir := filepath.Join(cfg.ProjectPath, ".pollard", "sources", "github")
	if cfg.OutputDir != "" {
		outputDir = filepath.Join(cfg.ProjectPath, cfg.OutputDir)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to create output directory: %w", err))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Process each query through the 4-stage pipeline
	for _, query := range cfg.Queries {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, nil
		default:
		}

		// Stage 1: SEARCH - Find repositories matching the query
		rawItems, err := g.search(ctx, query, token, cfg.MaxResults, cfg.MinStars)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("search %q failed: %w", query, err))
			continue
		}

		if len(rawItems) == 0 {
			continue
		}

		// Stage 2: FETCH - Get README and additional details
		fetchOpts := pipeline.FetchOpts{
			Mode:        mode,
			FetchREADME: cfg.Pipeline.FetchREADME && mode != pipeline.ModeQuick,
			Timeout:     30 * time.Second,
		}
		fetchedItems, err := g.fetcher.FetchBatch(ctx, rawItems, fetchOpts)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("fetch %q failed: %w", query, err))
			continue
		}

		// Stage 3: SYNTHESIZE - Use agent to analyze items (mode-dependent)
		var synthesizedItems []pipeline.SynthesizedItem
		if g.synthesizer != nil && mode != pipeline.ModeQuick {
			// Determine how many items to synthesize based on mode
			itemsToSynthesize := fetchedItems
			if mode == pipeline.ModeBalanced && cfg.Pipeline.SynthesizeLimit > 0 {
				if len(itemsToSynthesize) > cfg.Pipeline.SynthesizeLimit {
					itemsToSynthesize = itemsToSynthesize[:cfg.Pipeline.SynthesizeLimit]
				}
			}
			synthesizedItems, err = g.synthesizer.SynthesizeBatch(ctx, itemsToSynthesize, query)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("synthesize %q failed: %w", query, err))
				// Fall back to items without synthesis
				synthesizedItems = wrapWithoutSynthesis(fetchedItems)
			}
			// Add any remaining items that weren't synthesized
			if len(itemsToSynthesize) < len(fetchedItems) {
				remaining := wrapWithoutSynthesis(fetchedItems[len(itemsToSynthesize):])
				synthesizedItems = append(synthesizedItems, remaining...)
			}
		} else {
			synthesizedItems = wrapWithoutSynthesis(fetchedItems)
		}

		// Stage 4: SCORE - Calculate quality scores
		scoredItems := g.scorer.ScoreBatch(synthesizedItems, query)

		// Write results with scores
		outputFile, err := g.writeResultsWithScores(outputDir, query, scoredItems)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write results for %q: %w", query, err))
			continue
		}

		result.SourcesCollected += len(scoredItems)
		result.OutputFiles = append(result.OutputFiles, outputFile)
	}

	result.CompletedAt = time.Now()
	return result, nil
}

// wrapWithoutSynthesis wraps fetched items without synthesis data.
func wrapWithoutSynthesis(items []pipeline.FetchedItem) []pipeline.SynthesizedItem {
	result := make([]pipeline.SynthesizedItem, len(items))
	for i, item := range items {
		result[i] = pipeline.SynthesizedItem{
			Fetched:   item,
			Synthesis: pipeline.Synthesis{},
		}
	}
	return result
}

// search implements Stage 1 of the pipeline - finding repositories.
func (g *GitHubScout) search(ctx context.Context, query, token string, maxResults, minStars int) ([]pipeline.RawItem, error) {
	repos, err := g.searchRepositories(ctx, query, token, maxResults, minStars)
	if err != nil {
		return nil, err
	}

	// Convert to RawItem format
	items := make([]pipeline.RawItem, len(repos))
	now := time.Now()
	for i, repo := range repos {
		updatedAt, _ := time.Parse(time.RFC3339, repo.UpdatedAt)

		items[i] = pipeline.RawItem{
			ID:    fmt.Sprintf("github:%s/%s", repo.Owner.Login, repo.Name),
			Type:  "github_repo",
			Title: fmt.Sprintf("%s/%s", repo.Owner.Login, repo.Name),
			URL:   repo.HTMLURL,
			Metadata: map[string]any{
				"owner":       repo.Owner.Login,
				"name":        repo.Name,
				"description": repo.Description,
				"stars":       repo.StargazersCount,
				"language":    repo.Language,
				"topics":      repo.Topics,
				"updated_at":  updatedAt,
				"fork":        repo.Fork,
				"archived":    repo.Archived,
			},
			CollectedAt: now,
		}
	}

	return items, nil
}

// searchRepositories searches GitHub for repositories matching the query.
func (g *GitHubScout) searchRepositories(ctx context.Context, query, token string, maxResults, minStars int) ([]gitHubRepoResult, error) {
	var allRepos []gitHubRepoResult

	// Build the search query with optional stars filter
	searchQuery := query
	if minStars > 0 {
		searchQuery = fmt.Sprintf("%s stars:>=%d", query, minStars)
	}

	// GitHub search API returns max 100 per page, max 1000 total
	perPage := 100
	if maxResults > 0 && maxResults < perPage {
		perPage = maxResults
	}

	maxPages := 10 // GitHub limits to 1000 results (10 pages * 100)
	if maxResults > 0 {
		maxPages = (maxResults + perPage - 1) / perPage
	}

	for page := 1; page <= maxPages; page++ {
		// Wait for rate limiter
		if err := g.rateLimiter.Wait(ctx); err != nil {
			return allRepos, err
		}

		// Build request URL
		params := url.Values{}
		params.Set("q", searchQuery)
		params.Set("sort", "stars")
		params.Set("order", "desc")
		params.Set("per_page", fmt.Sprintf("%d", perPage))
		params.Set("page", fmt.Sprintf("%d", page))

		reqURL := fmt.Sprintf("https://api.github.com/search/repositories?%s", params.Encode())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return allRepos, fmt.Errorf("failed to create request: %w", err)
		}

		// Set required headers
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Pollard-GitHubScout/1.0")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		if token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}

		resp, err := g.client.Do(req)
		if err != nil {
			return allRepos, fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			// Rate limited - check headers for reset time
			if resetTime := resp.Header.Get("X-RateLimit-Reset"); resetTime != "" {
				return allRepos, fmt.Errorf("rate limited, resets at %s", resetTime)
			}
			return allRepos, fmt.Errorf("rate limited (status %d)", resp.StatusCode)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return allRepos, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		var searchResult gitHubSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
			resp.Body.Close()
			return allRepos, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		allRepos = append(allRepos, searchResult.Items...)

		// Check if we have enough results or no more pages
		if maxResults > 0 && len(allRepos) >= maxResults {
			allRepos = allRepos[:maxResults]
			break
		}

		if len(searchResult.Items) < perPage {
			// No more results
			break
		}

		// Don't exceed total count
		if len(allRepos) >= searchResult.TotalCount {
			break
		}
	}

	return allRepos, nil
}

// writeResultsWithScores writes scored results to a YAML file with full pipeline data.
func (g *GitHubScout) writeResultsWithScores(outputDir, query string, items []pipeline.ScoredItem) (string, error) {
	now := time.Now()

	// Convert to output format with scores
	output := gitHubSearchOutputV2{
		Query:       query,
		CollectedAt: now,
		Repos:       make([]gitHubRepoOutputV2, 0, len(items)),
	}

	for _, item := range items {
		raw := item.Synthesized.Fetched.Raw

		// Extract metadata
		owner, _ := raw.Metadata["owner"].(string)
		name, _ := raw.Metadata["name"].(string)
		description, _ := raw.Metadata["description"].(string)
		stars, _ := raw.Metadata["stars"].(int)
		language, _ := raw.Metadata["language"].(string)
		topics, _ := raw.Metadata["topics"].([]string)
		updatedAt, _ := raw.Metadata["updated_at"].(time.Time)

		repoOutput := gitHubRepoOutputV2{
			Owner:       owner,
			Name:        name,
			Description: description,
			URL:         raw.URL,
			Stars:       stars,
			Language:    language,
			Topics:      topics,
			UpdatedAt:   updatedAt,
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
			repoOutput.Synthesis = &synthesisOutput{
				Summary:            synthesis.Summary,
				KeyFeatures:        synthesis.KeyFeatures,
				RelevanceRationale: synthesis.RelevanceRationale,
				Recommendations:    synthesis.Recommendations,
				Confidence:         synthesis.Confidence,
			}
		}

		output.Repos = append(output.Repos, repoOutput)
	}

	// Generate filename: YYYY-MM-DD-<query-slug>.yaml
	slug := slugify(query)
	filename := fmt.Sprintf("%s-%s.yaml", now.Format("2006-01-02"), slug)
	filePath := filepath.Join(outputDir, filename)

	data, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// writeResults writes the search results to a YAML file (legacy format).
func (g *GitHubScout) writeResults(outputDir, query string, repos []gitHubRepoResult) (string, error) {
	now := time.Now()

	// Convert to output format
	output := gitHubSearchOutput{
		Query:       query,
		CollectedAt: now,
		Repos:       make([]gitHubRepoOutput, 0, len(repos)),
	}

	for _, repo := range repos {
		updatedAt, _ := time.Parse(time.RFC3339, repo.UpdatedAt)

		output.Repos = append(output.Repos, gitHubRepoOutput{
			Owner:       repo.Owner.Login,
			Name:        repo.Name,
			Description: repo.Description,
			URL:         repo.HTMLURL,
			Stars:       repo.StargazersCount,
			Language:    repo.Language,
			Topics:      repo.Topics,
			UpdatedAt:   updatedAt,
		})
	}

	// Generate filename: YYYY-MM-DD-<query-slug>.yaml
	slug := slugify(query)
	filename := fmt.Sprintf("%s-%s.yaml", now.Format("2006-01-02"), slug)
	filePath := filepath.Join(outputDir, filename)

	data, err := yaml.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// slugify converts a query string to a URL-safe slug.
func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and special chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	s = reg.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Limit length
	if len(s) > 50 {
		s = s[:50]
		// Don't end with a hyphen
		s = strings.TrimRight(s, "-")
	}

	return s
}

// GitHub API response structures

type gitHubSearchResponse struct {
	TotalCount        int                 `json:"total_count"`
	IncompleteResults bool                `json:"incomplete_results"`
	Items             []gitHubRepoResult  `json:"items"`
}

type gitHubRepoResult struct {
	ID              int              `json:"id"`
	Name            string           `json:"name"`
	FullName        string           `json:"full_name"`
	Owner           gitHubOwner      `json:"owner"`
	Description     string           `json:"description"`
	HTMLURL         string           `json:"html_url"`
	StargazersCount int              `json:"stargazers_count"`
	Language        string           `json:"language"`
	Topics          []string         `json:"topics"`
	UpdatedAt       string           `json:"updated_at"`
	PushedAt        string           `json:"pushed_at"`
	Fork            bool             `json:"fork"`
	Archived        bool             `json:"archived"`
}

type gitHubOwner struct {
	Login string `json:"login"`
}

// Output YAML structures

type gitHubSearchOutput struct {
	Query       string             `yaml:"query"`
	CollectedAt time.Time          `yaml:"collected_at"`
	Repos       []gitHubRepoOutput `yaml:"repos"`
}

type gitHubRepoOutput struct {
	Owner       string    `yaml:"owner"`
	Name        string    `yaml:"name"`
	Description string    `yaml:"description"`
	URL         string    `yaml:"url"`
	Stars       int       `yaml:"stars"`
	Language    string    `yaml:"language"`
	Topics      []string  `yaml:"topics"`
	UpdatedAt   time.Time `yaml:"updated_at"`
}

// V2 output structures with pipeline data

type gitHubSearchOutputV2 struct {
	Query       string               `yaml:"query"`
	CollectedAt time.Time            `yaml:"collected_at"`
	Repos       []gitHubRepoOutputV2 `yaml:"repos"`
}

type gitHubRepoOutputV2 struct {
	Owner        string             `yaml:"owner"`
	Name         string             `yaml:"name"`
	Description  string             `yaml:"description,omitempty"`
	URL          string             `yaml:"url"`
	Stars        int                `yaml:"stars"`
	Language     string             `yaml:"language,omitempty"`
	Topics       []string           `yaml:"topics,omitempty"`
	UpdatedAt    time.Time          `yaml:"updated_at"`
	QualityScore qualityScoreOutput `yaml:"quality_score"`
	Synthesis    *synthesisOutput   `yaml:"synthesis,omitempty"`
}

type qualityScoreOutput struct {
	Value      float64            `yaml:"value"`
	Level      string             `yaml:"level"`
	Factors    map[string]float64 `yaml:"factors,omitempty"`
	Confidence float64            `yaml:"confidence"`
}

type synthesisOutput struct {
	Summary            string   `yaml:"summary,omitempty"`
	KeyFeatures        []string `yaml:"key_features,omitempty"`
	RelevanceRationale string   `yaml:"relevance_rationale,omitempty"`
	Recommendations    []string `yaml:"recommendations,omitempty"`
	Confidence         float64  `yaml:"confidence,omitempty"`
}
