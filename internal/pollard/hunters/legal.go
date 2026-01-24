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

// LegalHunter searches court decisions from CourtListener.
// CourtListener indexes 9M+ US court decisions.
type LegalHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	apiKey      string
}

// NewLegalHunter creates a new legal research hunter.
func NewLegalHunter() *LegalHunter {
	apiKey := os.Getenv("COURTLISTENER_API_KEY")
	return &LegalHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// CourtListener has generous rate limits
		rateLimiter: NewRateLimiter(5, time.Second, apiKey != ""),
		apiKey:      apiKey,
	}
}

// Name returns the hunter's identifier.
func (h *LegalHunter) Name() string {
	return "legal"
}

// Hunt performs the research collection from CourtListener.
func (h *LegalHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	if h.apiKey == "" {
		result.Errors = append(result.Errors, fmt.Errorf("COURTLISTENER_API_KEY environment variable not set"))
		result.CompletedAt = time.Now()
		return result, nil
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	var allCases []LegalCase
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

		cases, err := h.searchCourtListener(ctx, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		allCases = append(allCases, cases...)
	}

	// Deduplicate cases by ID
	seen := make(map[string]bool)
	uniqueCases := make([]LegalCase, 0, len(allCases))
	for _, c := range allCases {
		if !seen[c.ID] {
			seen[c.ID] = true
			uniqueCases = append(uniqueCases, c)
		}
	}

	// Save results
	if len(uniqueCases) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueCases, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueCases)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// searchCourtListener queries the CourtListener API for opinions.
func (h *LegalHunter) searchCourtListener(ctx context.Context, query string, maxResults int) ([]LegalCase, error) {
	// Search opinions endpoint
	apiURL := fmt.Sprintf(
		"https://www.courtlistener.com/api/rest/v3/search/?q=%s&type=o&order_by=score+desc&page_size=%d",
		url.QueryEscape(query),
		min(maxResults, 100), // API reasonable page size
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Token "+h.apiKey)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CourtListener API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseCourtListenerResponse(body, query)
}

// courtListenerResponse represents the API response structure.
type courtListenerResponse struct {
	Results []courtListenerResult `json:"results"`
}

type courtListenerResult struct {
	ID             int    `json:"id"`
	AbsoluteURL    string `json:"absolute_url"`
	CaseName       string `json:"caseName"`
	Court          string `json:"court"`
	CourtID        string `json:"court_id"`
	DateFiled      string `json:"dateFiled"`
	DocketNumber   string `json:"docketNumber"`
	Citation       string `json:"citation"`
	Snippet        string `json:"snippet"`
	SuitNature     string `json:"suitNature,omitempty"`
	Status         string `json:"status,omitempty"`
}

// parseCourtListenerResponse parses the JSON response from CourtListener.
func parseCourtListenerResponse(data []byte, originalQuery string) ([]LegalCase, error) {
	var response courtListenerResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	cases := make([]LegalCase, 0, len(response.Results))
	for _, r := range response.Results {
		// Parse date filed
		var dateFiled string
		if r.DateFiled != "" {
			dateFiled = r.DateFiled
		}

		// Build full URL
		caseURL := r.AbsoluteURL
		if !strings.HasPrefix(caseURL, "http") {
			caseURL = "https://www.courtlistener.com" + caseURL
		}

		// Clean up snippet (remove HTML tags)
		summary := cleanHTML(r.Snippet)

		// Map court ID to friendly name
		court := getCourtName(r.CourtID)
		if court == "" {
			court = r.Court
		}

		c := LegalCase{
			ID:         fmt.Sprintf("%d", r.ID),
			CaseName:   r.CaseName,
			Court:      court,
			DateFiled:  dateFiled,
			Docket:     r.DocketNumber,
			Citation:   r.Citation,
			Summary:    summary,
			URL:        caseURL,
			Relevance:  assessLegalRelevance(r.CaseName, r.Snippet, originalQuery),
		}
		cases = append(cases, c)
	}

	return cases, nil
}

// cleanHTML removes HTML tags from a string.
func cleanHTML(s string) string {
	// Simple HTML tag removal
	result := s
	for {
		start := strings.Index(result, "<")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}
	return strings.TrimSpace(result)
}

// getCourtName maps court IDs to friendly names.
func getCourtName(courtID string) string {
	courts := map[string]string{
		"scotus":    "Supreme Court of the United States",
		"ca1":       "First Circuit Court of Appeals",
		"ca2":       "Second Circuit Court of Appeals",
		"ca3":       "Third Circuit Court of Appeals",
		"ca4":       "Fourth Circuit Court of Appeals",
		"ca5":       "Fifth Circuit Court of Appeals",
		"ca6":       "Sixth Circuit Court of Appeals",
		"ca7":       "Seventh Circuit Court of Appeals",
		"ca8":       "Eighth Circuit Court of Appeals",
		"ca9":       "Ninth Circuit Court of Appeals",
		"ca10":      "Tenth Circuit Court of Appeals",
		"ca11":      "Eleventh Circuit Court of Appeals",
		"cadc":      "D.C. Circuit Court of Appeals",
		"cafc":      "Federal Circuit Court of Appeals",
	}
	if name, ok := courts[courtID]; ok {
		return name
	}
	return courtID
}

// assessLegalRelevance determines the relevance level of a case.
func assessLegalRelevance(caseName, snippet, query string) string {
	queryLower := strings.ToLower(query)
	caseNameLower := strings.ToLower(caseName)
	snippetLower := strings.ToLower(snippet)

	// Query words in case name is high signal
	queryWords := strings.Fields(queryLower)
	nameMatches := 0
	for _, word := range queryWords {
		if len(word) > 3 && strings.Contains(caseNameLower, word) {
			nameMatches++
		}
	}

	if nameMatches >= 2 {
		return "high"
	}

	// Good snippet match
	snippetMatches := 0
	for _, word := range queryWords {
		if len(word) > 3 && strings.Contains(snippetLower, word) {
			snippetMatches++
		}
	}

	if snippetMatches >= 3 || nameMatches >= 1 {
		return "medium"
	}

	return "low"
}

// LegalCase represents a court case in the output YAML format.
type LegalCase struct {
	ID        string `yaml:"id"`
	CaseName  string `yaml:"case_name"`
	Court     string `yaml:"court"`
	DateFiled string `yaml:"date_filed"`
	Docket    string `yaml:"docket,omitempty"`
	Citation  string `yaml:"citation,omitempty"`
	Summary   string `yaml:"summary,omitempty"`
	URL       string `yaml:"url"`
	Relevance string `yaml:"relevance"`
}

// LegalOutput represents the complete output YAML structure.
type LegalOutput struct {
	Query       string      `yaml:"query"`
	CollectedAt time.Time   `yaml:"collected_at"`
	Cases       []LegalCase `yaml:"cases"`
}

// saveResults saves the collected cases to a YAML file.
func (h *LegalHunter) saveResults(cfg HunterConfig, cases []LegalCase, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/legal"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-legal.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := LegalOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Cases:       cases,
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
