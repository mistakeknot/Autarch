// Package hunters provides research agent implementations for Pollard.
package hunters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Context7Hunter queries Context7 MCP for framework documentation.
// Context7 provides documentation access for 100+ popular libraries
// including React, Vue, Angular, Tailwind, Prisma, and more.
type Context7Hunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	endpoint    string
}

// NewContext7Hunter creates a new Context7 documentation hunter.
func NewContext7Hunter() *Context7Hunter {
	return &Context7Hunter{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		// Context7 is relatively permissive but be polite
		rateLimiter: NewRateLimiter(10, time.Second, false),
		endpoint:    "https://mcp.context7.com/mcp",
	}
}

// Name returns the hunter's identifier.
func (h *Context7Hunter) Name() string {
	return "context7"
}

// Hunt performs documentation collection from Context7.
func (h *Context7Hunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	var allDocs []Context7Doc
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

		// Step 1: Resolve library ID from query
		libraryID, err := h.resolveLibraryID(ctx, query)
		if err != nil {
			errors = append(errors, fmt.Errorf("resolve library %q: %w", query, err))
			continue
		}

		if libraryID == "" {
			// No library found for this query, skip
			continue
		}

		// Wait for rate limiter
		if err := h.rateLimiter.Wait(ctx); err != nil {
			errors = append(errors, fmt.Errorf("rate limit wait for docs %q: %w", query, err))
			continue
		}

		// Step 2: Query docs for this library
		docs, err := h.queryDocs(ctx, libraryID, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("query docs for %q: %w", libraryID, err))
			continue
		}

		allDocs = append(allDocs, docs...)
	}

	// Deduplicate docs by title
	seen := make(map[string]bool)
	uniqueDocs := make([]Context7Doc, 0, len(allDocs))
	for _, d := range allDocs {
		key := strings.ToLower(d.Library + "/" + d.Title)
		if !seen[key] {
			seen[key] = true
			uniqueDocs = append(uniqueDocs, d)
		}
	}

	// Save results
	if len(uniqueDocs) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueDocs, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueDocs)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// resolveLibraryID calls Context7's resolve-library-id tool to find the library.
func (h *Context7Hunter) resolveLibraryID(ctx context.Context, query string) (string, error) {
	// Build MCP tool call request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "resolve-library-id",
			"arguments": map[string]interface{}{
				"libraryName": query,
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Pollard/1.0 (research tool)")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Context7 API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Parse response to extract library ID
	var result struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("MCP error: %s", result.Error.Message)
	}

	// Extract library ID from content
	for _, c := range result.Result.Content {
		if c.Type == "text" && strings.Contains(c.Text, "/") {
			// The response typically contains "owner/repo" format
			return strings.TrimSpace(c.Text), nil
		}
	}

	return "", nil // No library found
}

// queryDocs calls Context7's query-docs tool to fetch documentation.
func (h *Context7Hunter) queryDocs(ctx context.Context, libraryID, topic string, maxTokens int) ([]Context7Doc, error) {
	// Build MCP tool call request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "query-docs",
			"arguments": map[string]interface{}{
				"context7CompatibleLibraryID": libraryID,
				"topic":                       topic,
				"tokens":                      maxTokens * 500, // Convert to rough token count
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Pollard/1.0 (research tool)")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Context7 API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Parse response to extract documentation
	var result struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", result.Error.Message)
	}

	// Convert content to docs
	var docs []Context7Doc
	for _, c := range result.Result.Content {
		if c.Type == "text" && c.Text != "" {
			doc := Context7Doc{
				Library:     libraryID,
				Title:       topic,
				Content:     c.Text,
				CollectedAt: time.Now().UTC(),
			}
			docs = append(docs, doc)
		}
	}

	return docs, nil
}

// Context7Doc represents a documentation entry from Context7.
type Context7Doc struct {
	Library     string    `yaml:"library"`
	Title       string    `yaml:"title"`
	Content     string    `yaml:"content"`
	URL         string    `yaml:"url,omitempty"`
	CollectedAt time.Time `yaml:"collected_at"`
}

// Context7Output represents the complete output YAML structure.
type Context7Output struct {
	Queries     []string      `yaml:"queries"`
	CollectedAt time.Time     `yaml:"collected_at"`
	Docs        []Context7Doc `yaml:"docs"`
}

// saveResults saves the collected documentation to a YAML file.
func (h *Context7Hunter) saveResults(cfg HunterConfig, docs []Context7Doc, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/context7"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-context7.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	output := Context7Output{
		Queries:     queries,
		CollectedAt: time.Now().UTC(),
		Docs:        docs,
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
