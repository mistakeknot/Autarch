// Package hunters provides research agent implementations for Pollard.
package hunters

import (
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

// EconomicsHunter fetches economic indicators from OECD and World Bank.
// These are free APIs with no authentication required.
type EconomicsHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
}

// NewEconomicsHunter creates a new economics research hunter.
func NewEconomicsHunter() *EconomicsHunter {
	return &EconomicsHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Polite rate limiting for public APIs
		rateLimiter: NewRateLimiter(2, time.Second, false),
	}
}

// Name returns the hunter's identifier.
func (h *EconomicsHunter) Name() string {
	return "economics"
}

// Hunt performs the research collection from OECD and World Bank.
func (h *EconomicsHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	var allIndicators []EconomicsIndicator
	var errors []error

	// Default indicators if none specified
	indicators := cfg.Queries
	if len(indicators) == 0 {
		indicators = []string{"GDP", "CPI", "UNEMP"}
	}

	// Default countries
	countries := []string{"USA", "GBR", "DEU", "JPN", "CHN"}

	// Fetch from World Bank (more accessible API)
	for _, indicator := range indicators {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.CompletedAt = time.Now()
			return result, ctx.Err()
		default:
		}

		// Map common indicator names to World Bank codes
		wbCode := mapToWorldBankCode(indicator)
		if wbCode == "" {
			continue
		}

		// Wait for rate limiter
		if err := h.rateLimiter.Wait(ctx); err != nil {
			errors = append(errors, fmt.Errorf("rate limit wait for indicator %q: %w", indicator, err))
			continue
		}

		data, err := h.fetchWorldBankIndicator(ctx, wbCode, countries)
		if err != nil {
			errors = append(errors, fmt.Errorf("fetch %q: %w", indicator, err))
			continue
		}

		allIndicators = append(allIndicators, data...)
	}

	// Save results
	if len(allIndicators) > 0 {
		outputFile, err := h.saveResults(cfg, allIndicators, indicators)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(allIndicators)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// mapToWorldBankCode converts common indicator names to World Bank codes.
func mapToWorldBankCode(indicator string) string {
	codes := map[string]string{
		"GDP":         "NY.GDP.MKTP.CD",       // GDP (current US$)
		"GDP_GROWTH":  "NY.GDP.MKTP.KD.ZG",    // GDP growth (annual %)
		"GDP_PER_CAP": "NY.GDP.PCAP.CD",       // GDP per capita (current US$)
		"CPI":         "FP.CPI.TOTL.ZG",       // Inflation, consumer prices (annual %)
		"UNEMP":       "SL.UEM.TOTL.ZS",       // Unemployment, total (% of labor force)
		"POP":         "SP.POP.TOTL",          // Population, total
		"TRADE":       "NE.TRD.GNFS.ZS",       // Trade (% of GDP)
		"FDI":         "BX.KLT.DINV.WD.GD.ZS", // FDI, net inflows (% of GDP)
		"DEBT":        "GC.DOD.TOTL.GD.ZS",    // Central government debt (% of GDP)
		"GINI":        "SI.POV.GINI",          // GINI index
		"LIFE_EXP":    "SP.DYN.LE00.IN",       // Life expectancy at birth
		"CO2":         "EN.ATM.CO2E.PC",       // CO2 emissions (metric tons per capita)
	}

	// Try exact match first
	if code, ok := codes[strings.ToUpper(indicator)]; ok {
		return code
	}

	// If it looks like a World Bank code already, use it directly
	if strings.Contains(indicator, ".") {
		return indicator
	}

	return ""
}

// fetchWorldBankIndicator fetches indicator data from World Bank API.
func (h *EconomicsHunter) fetchWorldBankIndicator(ctx context.Context, indicator string, countries []string) ([]EconomicsIndicator, error) {
	// Get most recent 5 years of data
	currentYear := time.Now().Year()
	dateRange := fmt.Sprintf("%d:%d", currentYear-5, currentYear)

	// World Bank API endpoint
	apiURL := fmt.Sprintf(
		"https://api.worldbank.org/v2/country/%s/indicator/%s?date=%s&format=json&per_page=500",
		strings.Join(countries, ";"),
		indicator,
		dateRange,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("World Bank API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseWorldBankResponse(body, indicator)
}

// parseWorldBankResponse parses the JSON response from World Bank API.
// World Bank returns an array where [0] is metadata and [1] is data.
func parseWorldBankResponse(data []byte, indicatorCode string) ([]EconomicsIndicator, error) {
	// World Bank returns [metadata, data]
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	if len(raw) < 2 {
		return nil, nil // No data
	}

	// Parse the data array
	var results []worldBankDataPoint
	if err := json.Unmarshal(raw[1], &results); err != nil {
		// The data might be null
		return nil, nil
	}

	indicators := make([]EconomicsIndicator, 0, len(results))
	for _, r := range results {
		if r.Value == nil {
			continue // Skip null values
		}

		indicators = append(indicators, EconomicsIndicator{
			Indicator: r.Indicator.Value,
			Country:   r.Country.Value,
			CountryID: r.Country.ID,
			Value:     *r.Value,
			Unit:      r.Unit,
			Period:    r.Date,
			Source:    "World Bank",
		})
	}

	return indicators, nil
}

// worldBankDataPoint represents a single data point from World Bank.
type worldBankDataPoint struct {
	Indicator struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"indicator"`
	Country struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"country"`
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
	Date  string   `json:"date"`
}

// EconomicsIndicator represents an economic indicator in the output YAML format.
type EconomicsIndicator struct {
	Indicator string  `yaml:"indicator"`
	Country   string  `yaml:"country"`
	CountryID string  `yaml:"country_id"`
	Value     float64 `yaml:"value"`
	Unit      string  `yaml:"unit,omitempty"`
	Period    string  `yaml:"period"`
	Source    string  `yaml:"source"`
}

// EconomicsOutput represents the complete output YAML structure.
type EconomicsOutput struct {
	Indicators []string             `yaml:"indicators"`
	CollectedAt time.Time           `yaml:"collected_at"`
	Data       []EconomicsIndicator `yaml:"data"`
}

// saveResults saves the collected indicators to a YAML file.
func (h *EconomicsHunter) saveResults(cfg HunterConfig, indicators []EconomicsIndicator, indicatorNames []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/economics"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-economics.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	output := EconomicsOutput{
		Indicators:  indicatorNames,
		CollectedAt: time.Now().UTC(),
		Data:        indicators,
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
