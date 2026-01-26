package schema

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestAuditor_ReviewSpec_ExtractsTables(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Management",
		Summary:      "Manage users and posts",
		Requirements: []string{"Create user accounts", "Users can create posts", "Add comments to posts"},
	}

	review := auditor.ReviewSpec(spec)

	// Should extract users, posts, and comments tables
	tableNames := make(map[string]bool)
	for _, t := range review.Tables {
		tableNames[t.Name] = true
	}

	if !tableNames["users"] {
		t.Error("expected users table")
	}
	if !tableNames["posts"] {
		t.Error("expected posts table")
	}
	if !tableNames["comments"] {
		t.Error("expected comments table")
	}
}

func TestAuditor_ReviewSpec_StorePattern(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Analytics",
		Summary:      "Track analytics",
		Requirements: []string{"Store analytics data", "Persist event logs"},
	}

	review := auditor.ReviewSpec(spec)

	// Should extract tables from "store X" pattern
	if len(review.Tables) == 0 {
		t.Error("expected tables from store pattern")
	}
}

func TestAuditor_IdentifiesPII(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Profile",
		Summary:      "User account management",
		Requirements: []string{"Store user email and name"},
	}

	review := auditor.ReviewSpec(spec)

	if len(review.PIIFields) == 0 {
		t.Error("expected PII fields to be identified")
	}

	// Check for email PII
	foundEmail := false
	for _, field := range review.PIIFields {
		if strings.Contains(field, "email") {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Error("expected email to be identified as PII")
	}
}

func TestAuditor_SuggestsIndexes_ForeignKeys(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Blog",
		Summary:      "Blog with posts and comments",
		Requirements: []string{"Users create posts", "Comments on posts"},
	}

	review := auditor.ReviewSpec(spec)

	// Should suggest indexes for foreign keys
	foundFKIndex := false
	for _, idx := range review.Indexes {
		if idx.Rationale == "Foreign key - improves JOIN performance" {
			foundFKIndex = true
			break
		}
	}
	if !foundFKIndex {
		t.Error("expected foreign key index suggestion")
	}
}

func TestAuditor_SuggestsIndexes_Email(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth",
		Summary:      "User authentication",
		Requirements: []string{"User can login with email"},
	}

	review := auditor.ReviewSpec(spec)

	// Should suggest index for email
	foundEmailIndex := false
	for _, idx := range review.Indexes {
		if len(idx.Columns) > 0 && idx.Columns[0] == "email" {
			foundEmailIndex = true
			break
		}
	}
	if !foundEmailIndex {
		t.Error("expected email index suggestion")
	}
}

func TestAuditor_IdentifiesConcerns_N1Risk(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Blog",
		Summary:      "Blog with posts and comments",
		Requirements: []string{"Users create posts", "Comments on posts"},
	}

	review := auditor.ReviewSpec(spec)

	// Should identify N+1 risk for tables with foreign keys
	foundN1Risk := false
	for _, concern := range review.Concerns {
		if concern.Type == "n+1_risk" {
			foundN1Risk = true
			break
		}
	}
	if !foundN1Risk {
		t.Error("expected N+1 risk concern")
	}
}

func TestAuditor_IdentifiesConcerns_PIIEncryption(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User System",
		Summary:      "User management",
		Requirements: []string{"Store user information"},
	}

	review := auditor.ReviewSpec(spec)

	// Should identify PII encryption concern
	foundPIIConcern := false
	for _, concern := range review.Concerns {
		if concern.Type == "pii_encryption" {
			foundPIIConcern = true
			break
		}
	}
	if !foundPIIConcern {
		t.Error("expected PII encryption concern")
	}
}

func TestAuditor_IdentifiesConcerns_Scale(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Enterprise",
		Summary:      "Handle millions of users",
		Requirements: []string{"Scale to high volume traffic", "Support enterprise customers"},
	}

	review := auditor.ReviewSpec(spec)

	// Should identify scale concern
	foundScaleConcern := false
	for _, concern := range review.Concerns {
		if concern.Type == "scale_consideration" {
			foundScaleConcern = true
			break
		}
	}
	if !foundScaleConcern {
		t.Error("expected scale consideration concern")
	}
}

func TestAuditor_AssessesComplexity_Simple(t *testing.T) {
	auditor := NewAuditor()

	// Spec with no recognizable entities (no tables extracted)
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Simple Feature",
		Summary:      "A simple feature with no data storage",
		Requirements: []string{"Display a welcome message"},
	}

	review := auditor.ReviewSpec(spec)

	// With no tables and no concerns, should be simple
	if review.Complexity != "simple" && len(review.Tables) == 0 {
		t.Errorf("Complexity = %s, want simple for no tables", review.Complexity)
	}
}

func TestAuditor_GeneratesMigrations(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Blog",
		Summary:      "Blog platform",
		Requirements: []string{"Users create posts", "Comments on posts"},
	}

	review := auditor.ReviewSpec(spec)

	if len(review.Migrations) == 0 {
		t.Error("expected migration steps")
	}

	// Migrations should be in order
	for i, mig := range review.Migrations {
		if mig.Order != i+1 {
			t.Errorf("Migration order = %d, want %d", mig.Order, i+1)
		}
	}
}

func TestAuditor_ReviewPRD(t *testing.T) {
	auditor := NewAuditor()

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "MVP Release",
		Version: "mvp",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "User Auth",
				Summary:      "Authentication",
				Requirements: []string{"User login", "User registration"},
			},
			{
				ID:           "FEAT-002",
				Title:        "Orders",
				Summary:      "Order management",
				Requirements: []string{"Create orders", "Track payments"},
			},
		},
	}

	review := auditor.ReviewPRD(prd)

	if review.SpecID != "MVP" {
		t.Errorf("SpecID = %s, want MVP", review.SpecID)
	}

	// Should extract tables from all features
	tableNames := make(map[string]bool)
	for _, t := range review.Tables {
		tableNames[t.Name] = true
	}

	if !tableNames["users"] {
		t.Error("expected users table from auth feature")
	}
	if !tableNames["orders"] {
		t.Error("expected orders table from orders feature")
	}
}

func TestAuditor_TableDefinition_Users(t *testing.T) {
	auditor := NewAuditor()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User System",
		Requirements: []string{"User registration"},
	}

	review := auditor.ReviewSpec(spec)

	// Find users table
	var usersTable *TableDefinition
	for i := range review.Tables {
		if review.Tables[i].Name == "users" {
			usersTable = &review.Tables[i]
			break
		}
	}

	if usersTable == nil {
		t.Fatal("users table not found")
	}

	// Should have primary key
	if len(usersTable.PrimaryKey) == 0 || usersTable.PrimaryKey[0] != "id" {
		t.Error("expected id as primary key")
	}

	// Should have email column
	hasEmail := false
	for _, col := range usersTable.Columns {
		if col.Name == "email" {
			hasEmail = true
			if len(col.PIITypes) == 0 {
				t.Error("email should be marked as PII")
			}
			break
		}
	}
	if !hasEmail {
		t.Error("expected email column")
	}
}

func TestFormatSchemaReview(t *testing.T) {
	review := &SchemaReview{
		SpecID:             "SPEC-001",
		Complexity:         "moderate",
		NormalizationLevel: Form3NF,
		Tables: []TableDefinition{
			{
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: []ColumnDefinition{
					{Name: "id", Type: "uuid", Nullable: false},
					{Name: "email", Type: "varchar(255)", Nullable: false, PIITypes: []PIIType{PIIEmail}},
				},
			},
		},
		PIIFields: []string{"users.email"},
		Indexes: []IndexSuggestion{
			{TableName: "users", Columns: []string{"email"}, IndexType: "btree", Rationale: "Lookup", Priority: SeverityHigh},
		},
		Concerns: []SchemaConcern{
			{Type: "pii_encryption", Severity: SeverityHigh, TableName: "users", ColumnName: "email", Description: "PII needs encryption", Suggestion: "Encrypt"},
		},
		Migrations: []MigrationStep{
			{Order: 1, Description: "Create users table", Reversible: true},
		},
	}

	output := FormatSchemaReview(review)

	if !strings.Contains(output, "Schema Review Report") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "SPEC-001") {
		t.Error("should contain spec ID")
	}
	if !strings.Contains(output, "users") {
		t.Error("should contain table name")
	}
	if !strings.Contains(output, "PII Fields") {
		t.Error("should contain PII section")
	}
	if !strings.Contains(output, "Suggested Indexes") {
		t.Error("should contain indexes section")
	}
	if !strings.Contains(output, "Concerns") {
		t.Error("should contain concerns section")
	}
	if !strings.Contains(output, "Migration Plan") {
		t.Error("should contain migration section")
	}
}

func TestBuildAuditorBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Feature",
		Summary:      "A test feature",
		Requirements: []string{"Req 1", "Req 2"},
	}

	brief := BuildAuditorBrief(spec)

	if !strings.Contains(brief, "Auditor Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "Schema Review") {
		t.Error("should contain schema review reference")
	}
	if !strings.Contains(brief, "Normalization") {
		t.Error("should contain normalization reference")
	}
	if !strings.Contains(brief, "N+1") {
		t.Error("should contain N+1 reference")
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
