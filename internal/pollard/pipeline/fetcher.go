// Package pipeline provides the research pipeline components.
package pipeline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Fetcher retrieves detailed content for raw items.
type Fetcher struct {
	client      *http.Client
	parallelism int
}

// NewFetcher creates a new fetcher with the given configuration.
func NewFetcher(parallelism int) *Fetcher {
	if parallelism <= 0 {
		parallelism = 5
	}
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		parallelism: parallelism,
	}
}

// FetchBatch fetches detailed content for multiple items concurrently.
func (f *Fetcher) FetchBatch(ctx context.Context, items []RawItem, opts FetchOpts) ([]FetchedItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	sem := make(chan struct{}, f.parallelism)
	results := make([]FetchedItem, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		go func(idx int, item RawItem) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = FetchedItem{
					Raw:          item,
					FetchSuccess: false,
					FetchError:   ctx.Err().Error(),
					FetchedAt:    time.Now(),
				}
				return
			}

			fetched := f.fetchOne(ctx, item, opts)
			results[idx] = fetched
		}(i, item)
	}

	wg.Wait()
	return results, nil
}

// fetchOne fetches content for a single item.
func (f *Fetcher) fetchOne(ctx context.Context, item RawItem, opts FetchOpts) FetchedItem {
	result := FetchedItem{
		Raw:       item,
		FetchedAt: time.Now(),
	}

	// Skip fetch for quick mode - just pass through
	if opts.Mode == ModeQuick {
		result.FetchSuccess = true
		return result
	}

	switch item.Type {
	case "github_repo":
		return f.fetchGitHubRepo(ctx, item, opts)
	case "hn_story":
		return f.fetchHNStory(ctx, item, opts)
	case "arxiv_paper":
		return f.fetchArxivPaper(ctx, item, opts)
	case "openalex_work":
		return f.fetchOpenAlexWork(ctx, item, opts)
	default:
		// For unknown types, just pass through
		result.FetchSuccess = true
		return result
	}
}

// fetchGitHubRepo fetches README content for a GitHub repository.
func (f *Fetcher) fetchGitHubRepo(ctx context.Context, item RawItem, opts FetchOpts) FetchedItem {
	result := FetchedItem{
		Raw:       item,
		FetchedAt: time.Now(),
	}

	if !opts.FetchREADME {
		result.FetchSuccess = true
		return result
	}

	// Extract owner/repo from metadata or URL
	owner, _ := item.Metadata["owner"].(string)
	name, _ := item.Metadata["name"].(string)

	if owner == "" || name == "" {
		// Try to parse from URL
		parts := strings.Split(strings.TrimPrefix(item.URL, "https://github.com/"), "/")
		if len(parts) >= 2 {
			owner = parts[0]
			name = parts[1]
		}
	}

	if owner == "" || name == "" {
		result.FetchSuccess = false
		result.FetchError = "could not determine owner/repo"
		return result
	}

	// Fetch README via GitHub API
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readmeURL, nil)
	if err != nil {
		result.FetchSuccess = false
		result.FetchError = err.Error()
		return result
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Pollard/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		result.FetchSuccess = false
		result.FetchError = err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.FetchSuccess = false
		result.FetchError = fmt.Sprintf("status %d", resp.StatusCode)
		return result
	}

	var readmeResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&readmeResp); err != nil {
		result.FetchSuccess = false
		result.FetchError = err.Error()
		return result
	}

	// Decode base64 content
	if readmeResp.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(readmeResp.Content, "\n", ""))
		if err != nil {
			result.FetchSuccess = false
			result.FetchError = "failed to decode README: " + err.Error()
			return result
		}
		result.Content = string(decoded)
	} else {
		result.Content = readmeResp.Content
	}

	result.ContentType = "readme"
	result.FetchSuccess = true
	return result
}

// fetchHNStory fetches the story text for a HackerNews item.
func (f *Fetcher) fetchHNStory(ctx context.Context, item RawItem, opts FetchOpts) FetchedItem {
	result := FetchedItem{
		Raw:       item,
		FetchedAt: time.Now(),
	}

	// For HN, we already have most data from search
	// The story_text is included in the Algolia response
	if storyText, ok := item.Metadata["story_text"].(string); ok && storyText != "" {
		result.Content = storyText
		result.ContentType = "story_text"
	}

	result.FetchSuccess = true
	return result
}

// fetchArxivPaper fetches abstract for an arXiv paper.
func (f *Fetcher) fetchArxivPaper(ctx context.Context, item RawItem, opts FetchOpts) FetchedItem {
	result := FetchedItem{
		Raw:       item,
		FetchedAt: time.Now(),
	}

	// Abstract should already be in metadata from search
	if abstract, ok := item.Metadata["abstract"].(string); ok && abstract != "" {
		result.Content = abstract
		result.ContentType = "abstract"
		result.FetchSuccess = true
		return result
	}

	// If not, we could fetch from arXiv API, but for now skip
	result.FetchSuccess = true
	return result
}

// fetchOpenAlexWork fetches abstract for an OpenAlex work.
func (f *Fetcher) fetchOpenAlexWork(ctx context.Context, item RawItem, opts FetchOpts) FetchedItem {
	result := FetchedItem{
		Raw:       item,
		FetchedAt: time.Now(),
	}

	// Abstract should already be in metadata from search
	if abstract, ok := item.Metadata["abstract"].(string); ok && abstract != "" {
		result.Content = abstract
		result.ContentType = "abstract"
		result.FetchSuccess = true
		return result
	}

	// OpenAlex API can provide abstract via inverted index
	// For now, just mark as success without additional fetch
	result.FetchSuccess = true
	return result
}

// FetchURL fetches content from a URL directly.
func (f *Fetcher) FetchURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Pollard/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return "", err
	}

	return string(body), nil
}
