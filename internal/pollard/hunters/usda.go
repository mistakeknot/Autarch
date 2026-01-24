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

// USDAHunter searches food and nutrition data from USDA FoodData Central.
// FoodData Central contains 1.4M+ foods with nutrients and allergen information.
type USDAHunter struct {
	client      *http.Client
	rateLimiter *RateLimiter
	apiKey      string
}

// NewUSDAHunter creates a new USDA nutrition research hunter.
func NewUSDAHunter() *USDAHunter {
	apiKey := os.Getenv("USDA_API_KEY")
	return &USDAHunter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		// USDA rate limit: 12k requests/hour
		rateLimiter: NewRateLimiter(10, time.Second, apiKey != ""),
		apiKey:      apiKey,
	}
}

// Name returns the hunter's identifier.
func (h *USDAHunter) Name() string {
	return "usda-nutrition"
}

// Hunt performs the research collection from USDA FoodData Central.
func (h *USDAHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	if h.apiKey == "" {
		result.Errors = append(result.Errors, fmt.Errorf("USDA_API_KEY environment variable not set"))
		result.CompletedAt = time.Now()
		return result, nil
	}

	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 50
	}

	var allFoods []USDAFood
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

		foods, err := h.searchUSDA(ctx, query, maxResults)
		if err != nil {
			errors = append(errors, fmt.Errorf("search %q: %w", query, err))
			continue
		}

		allFoods = append(allFoods, foods...)
	}

	// Deduplicate foods by FDC ID
	seen := make(map[int]bool)
	uniqueFoods := make([]USDAFood, 0, len(allFoods))
	for _, f := range allFoods {
		if !seen[f.FDCID] {
			seen[f.FDCID] = true
			uniqueFoods = append(uniqueFoods, f)
		}
	}

	// Save results
	if len(uniqueFoods) > 0 {
		outputFile, err := h.saveResults(cfg, uniqueFoods, cfg.Queries)
		if err != nil {
			errors = append(errors, fmt.Errorf("save results: %w", err))
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(uniqueFoods)
	result.Errors = errors
	result.CompletedAt = time.Now()

	return result, nil
}

// searchUSDA queries the USDA FoodData Central API.
func (h *USDAHunter) searchUSDA(ctx context.Context, query string, maxResults int) ([]USDAFood, error) {
	apiURL := fmt.Sprintf(
		"https://api.nal.usda.gov/fdc/v1/foods/search?api_key=%s&query=%s&pageSize=%d",
		h.apiKey,
		url.QueryEscape(query),
		min(maxResults, 200), // API max is 200
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
		return nil, fmt.Errorf("USDA API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseUSDAResponse(body, query)
}

// usdaResponse represents the API response structure.
type usdaResponse struct {
	Foods []usdaFood `json:"foods"`
}

type usdaFood struct {
	FDCID        int            `json:"fdcId"`
	Description  string         `json:"description"`
	DataType     string         `json:"dataType"`
	BrandOwner   string         `json:"brandOwner,omitempty"`
	BrandName    string         `json:"brandName,omitempty"`
	Ingredients  string         `json:"ingredients,omitempty"`
	FoodNutrients []usdaNutrient `json:"foodNutrients,omitempty"`
}

type usdaNutrient struct {
	NutrientName   string  `json:"nutrientName"`
	NutrientNumber string  `json:"nutrientNumber,omitempty"`
	Value          float64 `json:"value"`
	UnitName       string  `json:"unitName"`
}

// parseUSDAResponse parses the JSON response from USDA.
func parseUSDAResponse(data []byte, originalQuery string) ([]USDAFood, error) {
	var response usdaResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	foods := make([]USDAFood, 0, len(response.Foods))
	for _, f := range response.Foods {
		// Extract key nutrients (focus on common allergen-related and nutritional info)
		var nutrients []USDANutrient
		importantNutrients := map[string]bool{
			"Protein":       true,
			"Total lipid (fat)": true,
			"Carbohydrate, by difference": true,
			"Energy":        true,
			"Fiber, total dietary": true,
			"Sugars, total including NLEA": true,
			"Calcium, Ca":   true,
			"Iron, Fe":      true,
			"Sodium, Na":    true,
		}

		for _, n := range f.FoodNutrients {
			if importantNutrients[n.NutrientName] {
				nutrients = append(nutrients, USDANutrient{
					Name:   n.NutrientName,
					Amount: n.Value,
					Unit:   n.UnitName,
				})
			}
		}

		// Extract allergens from ingredients
		allergens := extractAllergens(f.Ingredients)

		// Use brand name if available
		brandOwner := f.BrandOwner
		if brandOwner == "" {
			brandOwner = f.BrandName
		}

		food := USDAFood{
			FDCID:       f.FDCID,
			Description: f.Description,
			DataType:    f.DataType,
			BrandOwner:  brandOwner,
			Nutrients:   nutrients,
			Allergens:   allergens,
			Ingredients: f.Ingredients,
			URL:         fmt.Sprintf("https://fdc.nal.usda.gov/fdc-app.html#/food-details/%d/nutrients", f.FDCID),
		}
		foods = append(foods, food)
	}

	return foods, nil
}

// extractAllergens identifies common allergens from ingredients text.
func extractAllergens(ingredients string) []string {
	if ingredients == "" {
		return nil
	}

	ingredientsLower := strings.ToLower(ingredients)

	// Common allergens (Big 9 in the US)
	allergenPatterns := map[string][]string{
		"milk":      {"milk", "dairy", "lactose", "casein", "whey", "cream", "butter", "cheese"},
		"eggs":      {"egg", "eggs", "albumin", "mayonnaise"},
		"fish":      {"fish", "cod", "salmon", "tuna", "anchov"},
		"shellfish": {"shellfish", "shrimp", "crab", "lobster", "clam", "oyster", "mussel", "scallop"},
		"tree nuts": {"almond", "cashew", "walnut", "pecan", "pistachio", "macadamia", "hazelnut", "brazil nut"},
		"peanuts":   {"peanut", "peanuts"},
		"wheat":     {"wheat", "flour", "gluten", "semolina", "durum", "bread crumb"},
		"soybeans":  {"soy", "soya", "soybean", "tofu", "edamame", "tempeh"},
		"sesame":    {"sesame", "tahini"},
	}

	var found []string
	for allergen, patterns := range allergenPatterns {
		for _, pattern := range patterns {
			if strings.Contains(ingredientsLower, pattern) {
				found = append(found, allergen)
				break
			}
		}
	}

	return found
}

// USDANutrient represents a nutrient value.
type USDANutrient struct {
	Name   string  `yaml:"name"`
	Amount float64 `yaml:"amount"`
	Unit   string  `yaml:"unit"`
}

// USDAFood represents a food item in the output YAML format.
type USDAFood struct {
	FDCID       int            `yaml:"fdc_id"`
	Description string         `yaml:"description"`
	DataType    string         `yaml:"data_type"`
	BrandOwner  string         `yaml:"brand_owner,omitempty"`
	Nutrients   []USDANutrient `yaml:"nutrients,omitempty"`
	Allergens   []string       `yaml:"allergens,omitempty"`
	Ingredients string         `yaml:"ingredients,omitempty"`
	URL         string         `yaml:"url"`
}

// USDAOutput represents the complete output YAML structure.
type USDAOutput struct {
	Query       string     `yaml:"query"`
	CollectedAt time.Time  `yaml:"collected_at"`
	Foods       []USDAFood `yaml:"foods"`
}

// saveResults saves the collected foods to a YAML file.
func (h *USDAHunter) saveResults(cfg HunterConfig, foods []USDAFood, queries []string) (string, error) {
	// Determine output directory
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "sources/nutrition"
	}

	// Ensure the directory exists
	fullOutputDir := filepath.Join(cfg.ProjectPath, ".pollard", outputDir)
	if err := os.MkdirAll(fullOutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Generate filename with date
	filename := fmt.Sprintf("%s-usda.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(fullOutputDir, filename)

	// Create output structure
	queryStr := strings.Join(queries, ", ")
	output := USDAOutput{
		Query:       queryStr,
		CollectedAt: time.Now().UTC(),
		Foods:       foods,
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
