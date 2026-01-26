package contracts

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestGenerateFromSpec_CRUDPatterns(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test API",
		Requirements: []string{
			"Create new user accounts",
			"Get user profile by ID",
			"Update user settings",
			"Delete inactive users",
			"List all users",
		},
	}

	set := gen.GenerateFromSpec(spec)

	if len(set.Contracts) == 0 {
		t.Error("expected contracts to be generated")
	}

	// Check for expected methods
	methods := make(map[string]bool)
	for _, c := range set.Contracts {
		methods[c.Method] = true
	}

	expectedMethods := []string{"POST", "GET", "PUT", "DELETE"}
	for _, m := range expectedMethods {
		if !methods[m] {
			t.Errorf("expected %s method contract", m)
		}
	}
}

func TestGenerateFromSpec_APIPattern(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test API",
		Requirements: []string{
			"API to authenticate users",
			"Endpoint for refreshing tokens",
		},
	}

	set := gen.GenerateFromSpec(spec)

	if len(set.Contracts) < 2 {
		t.Errorf("expected at least 2 contracts, got %d", len(set.Contracts))
	}
}

func TestGenerateFromSpec_CUJSteps(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test API",
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID: "CUJ-001",
				Steps: []string{
					"User submits registration form",
					"User uploads profile photo",
					"User shares post to social media",
				},
			},
		},
	}

	set := gen.GenerateFromSpec(spec)

	// Should detect submit, upload, share actions
	if len(set.Contracts) < 1 {
		t.Errorf("expected contracts from CUJ steps, got %d", len(set.Contracts))
	}
}

func TestExtractResource(t *testing.T) {
	tests := []struct {
		text     string
		verb     string
		expected string
	}{
		{"create new user", "create", "user"},
		{"get the profile", "get", "profile"},
		{"delete an account", "delete", "account"},
		{"list all orders", "list", "orders"},
		{"update user settings", "update", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := extractResource(tt.text, tt.verb)
			if result != tt.expected {
				t.Errorf("extractResource(%q, %q) = %q, want %q", tt.text, tt.verb, result, tt.expected)
			}
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user", "users"},
		{"users", "users"},
		{"category", "categories"},
		{"post", "posts"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := pluralize(tt.input)
			if result != tt.expected {
				t.Errorf("pluralize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatOpenAPI(t *testing.T) {
	set := &ContractSet{
		SpecID:  "SPEC-001",
		Title:   "Test API",
		Version: "1.0.0",
		Contracts: []APIContract{
			{
				Endpoint:     "/api/users",
				Method:       "GET",
				Summary:      "List all users",
				RequiresAuth: true,
				Tags:         []string{"users"},
				Errors:       defaultErrors(),
			},
			{
				Endpoint:     "/api/users",
				Method:       "POST",
				Summary:      "Create a user",
				RequiresAuth: true,
				Errors:       defaultErrors(),
			},
		},
	}

	output := FormatOpenAPI(set)

	if !strings.Contains(output, "openapi: \"3.0.0\"") {
		t.Error("should contain OpenAPI version")
	}
	if !strings.Contains(output, "Test API") {
		t.Error("should contain API title")
	}
	if !strings.Contains(output, "/api/users") {
		t.Error("should contain endpoint path")
	}
	if !strings.Contains(output, "get:") {
		t.Error("should contain GET method")
	}
	if !strings.Contains(output, "post:") {
		t.Error("should contain POST method")
	}
	if !strings.Contains(output, "bearerAuth") {
		t.Error("should contain security scheme")
	}
}

func TestDeduplicateContracts(t *testing.T) {
	contracts := []APIContract{
		{Endpoint: "/api/users", Method: "GET"},
		{Endpoint: "/api/users", Method: "GET"}, // Duplicate
		{Endpoint: "/api/users", Method: "POST"},
	}

	result := deduplicateContracts(contracts)

	if len(result) != 2 {
		t.Errorf("expected 2 unique contracts, got %d", len(result))
	}
}

func TestCreateCRUDContract(t *testing.T) {
	gen := NewGenerator()

	// Test GET with list
	listContract := gen.createCRUDContract("GET", "user", "list all users")
	if strings.Contains(listContract.Endpoint, "{id}") {
		t.Error("list endpoint should not have {id}")
	}

	// Test GET without list (single resource)
	getContract := gen.createCRUDContract("GET", "user", "get user by id")
	if !strings.Contains(getContract.Endpoint, "{id}") {
		t.Error("single GET endpoint should have {id}")
	}

	// Test PUT (always has ID)
	putContract := gen.createCRUDContract("PUT", "user", "update user")
	if !strings.Contains(putContract.Endpoint, "{id}") {
		t.Error("PUT endpoint should have {id}")
	}
	if putContract.Request == nil {
		t.Error("PUT should have request schema")
	}

	// Test DELETE (always has ID)
	deleteContract := gen.createCRUDContract("DELETE", "user", "delete user")
	if !strings.Contains(deleteContract.Endpoint, "{id}") {
		t.Error("DELETE endpoint should have {id}")
	}
}

func TestGenerateFromPRD(t *testing.T) {
	gen := NewGenerator()

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "MVP Release",
		Version: "mvp",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "User Management",
				Requirements: []string{"Create new users", "List all users"},
			},
			{
				ID:    "FEAT-002",
				Title: "Posts",
				CriticalUserJourneys: []specs.CriticalUserJourney{
					{
						ID:    "CUJ-001",
						Steps: []string{"User submits new post"},
					},
				},
			},
		},
	}

	set := gen.GenerateFromPRD(prd)

	if set.Version != "mvp" {
		t.Errorf("Version = %s, want mvp", set.Version)
	}

	// Check that contracts are tagged with feature IDs
	foundTaggedContract := false
	for _, c := range set.Contracts {
		if len(c.Tags) > 0 {
			foundTaggedContract = true
			break
		}
	}
	if !foundTaggedContract {
		t.Error("expected contracts to be tagged with feature IDs")
	}
}
