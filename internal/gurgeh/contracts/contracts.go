// Package contracts provides API contract generation from spec requirements.
// The "scribe" subagent drafts OpenAPI contracts from requirements, defining
// endpoint schemas, request/response types, and error codes.
package contracts

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// APIContract represents an API endpoint contract
type APIContract struct {
	Endpoint    string            `yaml:"endpoint" json:"endpoint"`       // POST /api/posts/{id}/share
	Method      string            `yaml:"method" json:"method"`           // POST
	Summary     string            `yaml:"summary" json:"summary"`
	Request     *SchemaDefinition `yaml:"request,omitempty" json:"request,omitempty"`
	Response    *SchemaDefinition `yaml:"response,omitempty" json:"response,omitempty"`
	Errors      []ErrorDefinition `yaml:"errors,omitempty" json:"errors,omitempty"`
	RateLimit   *RateLimitConfig  `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	RequiresAuth bool             `yaml:"requires_auth" json:"requires_auth"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// SchemaDefinition defines a request/response schema
type SchemaDefinition struct {
	Type       string                       `yaml:"type" json:"type"`                                 // object, array, string, etc.
	Properties map[string]PropertyDefinition `yaml:"properties,omitempty" json:"properties,omitempty"`
	Required   []string                     `yaml:"required,omitempty" json:"required,omitempty"`
	Items      *SchemaDefinition            `yaml:"items,omitempty" json:"items,omitempty"` // For arrays
	Example    interface{}                  `yaml:"example,omitempty" json:"example,omitempty"`
}

// PropertyDefinition defines a schema property
type PropertyDefinition struct {
	Type        string `yaml:"type" json:"type"`
	Format      string `yaml:"format,omitempty" json:"format,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Example     string `yaml:"example,omitempty" json:"example,omitempty"`
}

// ErrorDefinition defines an error response
type ErrorDefinition struct {
	Code        int    `yaml:"code" json:"code"`                 // 401, 429, 500
	Name        string `yaml:"name" json:"name"`                 // Unauthorized
	Description string `yaml:"description" json:"description"`
}

// RateLimitConfig defines rate limiting
type RateLimitConfig struct {
	RequestsPerMinute int    `yaml:"requests_per_minute" json:"requests_per_minute"`
	BurstLimit        int    `yaml:"burst_limit,omitempty" json:"burst_limit,omitempty"`
	Scope             string `yaml:"scope,omitempty" json:"scope,omitempty"` // user, ip, global
}

// ContractSet represents all contracts for a spec
type ContractSet struct {
	SpecID      string        `yaml:"spec_id" json:"spec_id"`
	Title       string        `yaml:"title" json:"title"`
	Version     string        `yaml:"version" json:"version"`
	BaseURL     string        `yaml:"base_url" json:"base_url"`
	Contracts   []APIContract `yaml:"contracts" json:"contracts"`
	GeneratedAt time.Time     `yaml:"generated_at" json:"generated_at"`
}

// Generator generates API contracts from specs
type Generator struct {
	defaultAuth      bool
	defaultRateLimit *RateLimitConfig
}

// NewGenerator creates a new contract generator
func NewGenerator() *Generator {
	return &Generator{
		defaultAuth: true,
		defaultRateLimit: &RateLimitConfig{
			RequestsPerMinute: 60,
			Scope:             "user",
		},
	}
}

// GenerateFromSpec extracts API contracts from spec requirements
func (g *Generator) GenerateFromSpec(spec *specs.Spec) *ContractSet {
	set := &ContractSet{
		SpecID:      spec.ID,
		Title:       spec.Title,
		Version:     "1.0.0",
		BaseURL:     "/api",
		GeneratedAt: time.Now(),
	}

	// Analyze requirements for API patterns
	for _, req := range spec.Requirements {
		contracts := g.extractContractsFromRequirement(req)
		set.Contracts = append(set.Contracts, contracts...)
	}

	// Analyze CUJ steps for API patterns
	for _, cuj := range spec.CriticalUserJourneys {
		for _, step := range cuj.Steps {
			contracts := g.extractContractsFromStep(step)
			set.Contracts = append(set.Contracts, contracts...)
		}
	}

	// Deduplicate
	set.Contracts = deduplicateContracts(set.Contracts)

	return set
}

// GenerateFromPRD extracts API contracts from a PRD
func (g *Generator) GenerateFromPRD(prd *specs.PRD) *ContractSet {
	set := &ContractSet{
		SpecID:      prd.ID,
		Title:       prd.Title,
		Version:     prd.Version,
		BaseURL:     "/api",
		GeneratedAt: time.Now(),
	}

	for _, feature := range prd.Features {
		// Extract from requirements
		for _, req := range feature.Requirements {
			contracts := g.extractContractsFromRequirement(req)
			for i := range contracts {
				contracts[i].Tags = append(contracts[i].Tags, feature.ID)
			}
			set.Contracts = append(set.Contracts, contracts...)
		}

		// Extract from CUJs
		for _, cuj := range feature.CriticalUserJourneys {
			for _, step := range cuj.Steps {
				contracts := g.extractContractsFromStep(step)
				for i := range contracts {
					contracts[i].Tags = append(contracts[i].Tags, feature.ID)
				}
				set.Contracts = append(set.Contracts, contracts...)
			}
		}
	}

	set.Contracts = deduplicateContracts(set.Contracts)
	return set
}

// extractContractsFromRequirement analyzes a requirement for API patterns
func (g *Generator) extractContractsFromRequirement(req string) []APIContract {
	var contracts []APIContract
	lower := strings.ToLower(req)

	// Pattern: "API to/for X"
	apiPattern := regexp.MustCompile(`(?i)api\s+(to|for)\s+(\w+)`)
	if matches := apiPattern.FindStringSubmatch(req); len(matches) > 2 {
		action := matches[2]
		contracts = append(contracts, g.createContract(action, req))
	}

	// Pattern: "endpoint for X"
	endpointPattern := regexp.MustCompile(`(?i)endpoint\s+(for|to)\s+(\w+)`)
	if matches := endpointPattern.FindStringSubmatch(req); len(matches) > 2 {
		action := matches[2]
		contracts = append(contracts, g.createContract(action, req))
	}

	// CRUD patterns
	crudPatterns := map[string]string{
		"create":  "POST",
		"add":     "POST",
		"insert":  "POST",
		"read":    "GET",
		"get":     "GET",
		"fetch":   "GET",
		"list":    "GET",
		"retrieve": "GET",
		"update":  "PUT",
		"modify":  "PUT",
		"edit":    "PUT",
		"delete":  "DELETE",
		"remove":  "DELETE",
	}

	for verb, method := range crudPatterns {
		if strings.Contains(lower, verb) {
			// Extract the resource
			resource := extractResource(req, verb)
			if resource != "" {
				contract := g.createCRUDContract(method, resource, req)
				contracts = append(contracts, contract)
			}
		}
	}

	return contracts
}

// extractContractsFromStep analyzes a CUJ step for API patterns
func (g *Generator) extractContractsFromStep(step string) []APIContract {
	var contracts []APIContract
	lower := strings.ToLower(step)

	// Look for action verbs that imply API calls
	actionVerbs := []string{"submit", "save", "send", "upload", "download", "share", "post", "publish"}

	for _, verb := range actionVerbs {
		if strings.Contains(lower, verb) {
			resource := extractResource(step, verb)
			if resource != "" {
				contract := g.createContract(verb+"-"+resource, step)
				contracts = append(contracts, contract)
			}
		}
	}

	return contracts
}

// createContract creates a basic API contract
func (g *Generator) createContract(action, source string) APIContract {
	// Normalize action to endpoint
	action = strings.ToLower(strings.ReplaceAll(action, " ", "-"))
	endpoint := fmt.Sprintf("/api/%s", action)

	return APIContract{
		Endpoint:     endpoint,
		Method:       "POST", // Default to POST for actions
		Summary:      truncateString(source, 100),
		RequiresAuth: g.defaultAuth,
		RateLimit:    g.defaultRateLimit,
		Errors:       defaultErrors(),
	}
}

// createCRUDContract creates a CRUD-style API contract
func (g *Generator) createCRUDContract(method, resource, source string) APIContract {
	resource = pluralize(strings.ToLower(resource))
	endpoint := fmt.Sprintf("/api/%s", resource)

	// Add ID parameter for single-resource operations
	if method == "GET" && !strings.Contains(strings.ToLower(source), "list") {
		endpoint += "/{id}"
	} else if method == "PUT" || method == "DELETE" {
		endpoint += "/{id}"
	}

	contract := APIContract{
		Endpoint:     endpoint,
		Method:       method,
		Summary:      truncateString(source, 100),
		RequiresAuth: g.defaultAuth,
		RateLimit:    g.defaultRateLimit,
		Errors:       defaultErrors(),
	}

	// Add request schema for write operations
	if method == "POST" || method == "PUT" {
		contract.Request = &SchemaDefinition{
			Type:       "object",
			Properties: map[string]PropertyDefinition{},
		}
	}

	// Add response schema
	if method == "GET" {
		contract.Response = &SchemaDefinition{
			Type: "object",
		}
	}

	return contract
}

// FormatOpenAPI formats contracts as OpenAPI 3.0 YAML
func FormatOpenAPI(set *ContractSet) string {
	var sb strings.Builder

	sb.WriteString("openapi: \"3.0.0\"\n")
	sb.WriteString("info:\n")
	sb.WriteString(fmt.Sprintf("  title: \"%s API\"\n", set.Title))
	sb.WriteString(fmt.Sprintf("  version: \"%s\"\n", set.Version))
	sb.WriteString("paths:\n")

	// Group by endpoint
	byEndpoint := make(map[string][]APIContract)
	for _, c := range set.Contracts {
		byEndpoint[c.Endpoint] = append(byEndpoint[c.Endpoint], c)
	}

	for endpoint, contracts := range byEndpoint {
		sb.WriteString(fmt.Sprintf("  %s:\n", endpoint))
		for _, c := range contracts {
			sb.WriteString(fmt.Sprintf("    %s:\n", strings.ToLower(c.Method)))
			sb.WriteString(fmt.Sprintf("      summary: \"%s\"\n", escapeYAML(c.Summary)))
			if len(c.Tags) > 0 {
				sb.WriteString("      tags:\n")
				for _, tag := range c.Tags {
					sb.WriteString(fmt.Sprintf("        - %s\n", tag))
				}
			}
			if c.RequiresAuth {
				sb.WriteString("      security:\n")
				sb.WriteString("        - bearerAuth: []\n")
			}
			sb.WriteString("      responses:\n")
			sb.WriteString("        \"200\":\n")
			sb.WriteString("          description: Success\n")
			for _, err := range c.Errors {
				sb.WriteString(fmt.Sprintf("        \"%d\":\n", err.Code))
				sb.WriteString(fmt.Sprintf("          description: %s\n", err.Description))
			}
		}
	}

	// Security schemes
	sb.WriteString("components:\n")
	sb.WriteString("  securitySchemes:\n")
	sb.WriteString("    bearerAuth:\n")
	sb.WriteString("      type: http\n")
	sb.WriteString("      scheme: bearer\n")

	return sb.String()
}

// --- Helper functions ---

func extractResource(text, verb string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, verb)
	if idx < 0 {
		return ""
	}

	// Get the word after the verb
	after := strings.TrimSpace(text[idx+len(verb):])
	words := strings.Fields(after)

	// Skip articles and common filler words
	skip := map[string]bool{"a": true, "an": true, "the": true, "to": true, "for": true, "new": true, "all": true, "some": true, "any": true}
	for _, word := range words {
		lower := strings.ToLower(word)
		if !skip[lower] && len(word) > 2 {
			// Clean up the word
			word = regexp.MustCompile(`[^a-zA-Z]`).ReplaceAllString(word, "")
			return word
		}
	}

	return ""
}

func pluralize(word string) string {
	if strings.HasSuffix(word, "s") {
		return word
	}
	if strings.HasSuffix(word, "y") {
		return word[:len(word)-1] + "ies"
	}
	return word + "s"
}

func defaultErrors() []ErrorDefinition {
	return []ErrorDefinition{
		{Code: 400, Name: "Bad Request", Description: "Invalid request parameters"},
		{Code: 401, Name: "Unauthorized", Description: "Authentication required"},
		{Code: 403, Name: "Forbidden", Description: "Permission denied"},
		{Code: 404, Name: "Not Found", Description: "Resource not found"},
		{Code: 500, Name: "Internal Error", Description: "Internal server error"},
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func deduplicateContracts(contracts []APIContract) []APIContract {
	seen := make(map[string]bool)
	var result []APIContract

	for _, c := range contracts {
		key := c.Method + ":" + c.Endpoint
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
	}

	return result
}
