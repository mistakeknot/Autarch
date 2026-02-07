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

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

// CustomHunterSpec defines a runtime-configurable hunter.
type CustomHunterSpec struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description"`
	APIEndpoint    string            `yaml:"api_endpoint,omitempty"`
	Method         string            `yaml:"method,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	QueryParam     string            `yaml:"query_param,omitempty"`
	ResultsPath    string            `yaml:"results_path,omitempty"`
	Mappings       FieldMappings     `yaml:"mappings,omitempty"`
	NoAPI          bool              `yaml:"no_api,omitempty"`
	Recommendation string            `yaml:"recommendation,omitempty"`
}

// FieldMappings maps response fields to output fields.
type FieldMappings struct {
	Title       string `yaml:"title"`
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
	Date        string `yaml:"date,omitempty"`
}

// CustomHunter executes a runtime-configured hunter spec.
type CustomHunter struct {
	spec   CustomHunterSpec
	client *http.Client
}

// NewCustomHunter creates a hunter from a spec file.
func NewCustomHunter(specPath string) (*CustomHunter, error) {
	var spec CustomHunterSpec
	if _, err := yamlsafe.UnmarshalFile(specPath, &spec); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	return &CustomHunter{
		spec: spec,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// NewCustomHunterFromSpec creates a hunter from a spec struct.
func NewCustomHunterFromSpec(spec CustomHunterSpec) *CustomHunter {
	return &CustomHunter{
		spec: spec,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the hunter's identifier.
func (h *CustomHunter) Name() string {
	return h.spec.Name
}

// Hunt performs the research collection using the custom spec.
func (h *CustomHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	// If no API, suggest using agent research
	if h.spec.NoAPI {
		result.Errors = append(result.Errors, fmt.Errorf("%s", h.spec.Recommendation))
		result.CompletedAt = time.Now()
		return result, nil
	}

	var allResults []CustomResult
	var errors []error

	for _, query := range cfg.Queries {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, ctx.Err()
		default:
		}

		results, err := h.executeQuery(ctx, query)
		if err != nil {
			errors = append(errors, fmt.Errorf("query %q: %w", query, err))
			continue
		}

		allResults = append(allResults, results...)
	}

	// Limit results
	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	// Save results
	if len(allResults) > 0 {
		outputFile, err := h.saveResults(cfg, allResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(allResults)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// executeQuery runs a single query against the API.
func (h *CustomHunter) executeQuery(ctx context.Context, query string) ([]CustomResult, error) {
	// Build URL
	apiURL := h.spec.APIEndpoint
	if h.spec.QueryParam != "" {
		if strings.Contains(apiURL, "?") {
			apiURL += "&"
		} else {
			apiURL += "?"
		}
		apiURL += fmt.Sprintf("%s=%s", h.spec.QueryParam, url.QueryEscape(query))
	}

	method := h.spec.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add headers
	for key, value := range h.spec.Headers {
		req.Header.Set(key, value)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return h.parseResponse(body)
}

// CustomResult represents a result from a custom hunter.
type CustomResult struct {
	Title       string `yaml:"title"`
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
	Date        string `yaml:"date,omitempty"`
	Relevance   string `yaml:"relevance"`
}

// parseResponse extracts results from API response.
func (h *CustomHunter) parseResponse(data []byte) ([]CustomResult, error) {
	var rawData interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	// Navigate to results path
	results := h.navigateToResults(rawData)
	if results == nil {
		return nil, fmt.Errorf("no results found at path: %s", h.spec.ResultsPath)
	}

	// Parse results array
	resultsArray, ok := results.([]interface{})
	if !ok {
		return nil, fmt.Errorf("results is not an array")
	}

	var customResults []CustomResult
	for _, item := range resultsArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		result := CustomResult{
			Title:       h.extractField(itemMap, h.spec.Mappings.Title),
			URL:         h.extractField(itemMap, h.spec.Mappings.URL),
			Description: h.extractField(itemMap, h.spec.Mappings.Description),
			Date:        h.extractField(itemMap, h.spec.Mappings.Date),
			Relevance:   "medium",
		}

		if result.Title != "" || result.URL != "" {
			customResults = append(customResults, result)
		}
	}

	return customResults, nil
}

// navigateToResults navigates to the results path in the response.
func (h *CustomHunter) navigateToResults(data interface{}) interface{} {
	if h.spec.ResultsPath == "" {
		return data
	}

	parts := strings.Split(h.spec.ResultsPath, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// extractField extracts a field value from the item.
func (h *CustomHunter) extractField(item map[string]interface{}, fieldPath string) string {
	if fieldPath == "" {
		return ""
	}

	parts := strings.Split(fieldPath, ".")
	var current interface{} = item

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return ""
		}
	}

	if current == nil {
		return ""
	}

	return fmt.Sprintf("%v", current)
}

// saveResults saves the collected results to a YAML file.
func (h *CustomHunter) saveResults(cfg HunterConfig, results []CustomResult) (string, error) {
	outputDir := filepath.Join(cfg.ProjectPath, ".pollard", "sources", "custom")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s-%s.yaml", time.Now().Format("2006-01-02"), h.spec.Name)
	fullPath := filepath.Join(outputDir, filename)

	output := struct {
		Hunter      string         `yaml:"hunter"`
		CollectedAt time.Time      `yaml:"collected_at"`
		Results     []CustomResult `yaml:"results"`
	}{
		Hunter:      h.spec.Name,
		CollectedAt: time.Now().UTC(),
		Results:     results,
	}

	data, err := yaml.Marshal(&output)
	if err != nil {
		return "", err
	}

	return fullPath, os.WriteFile(fullPath, data, 0644)
}

// LoadCustomHunters loads all custom hunter specs from a directory.
func LoadCustomHunters(projectPath string) ([]*CustomHunter, error) {
	customDir := filepath.Join(projectPath, ".pollard", "hunters", "custom")
	if _, err := os.Stat(customDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(customDir)
	if err != nil {
		return nil, err
	}

	var hunters []*CustomHunter
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		specPath := filepath.Join(customDir, entry.Name())
		hunter, err := NewCustomHunter(specPath)
		if err != nil {
			continue // Skip invalid specs
		}

		hunters = append(hunters, hunter)
	}

	return hunters, nil
}
