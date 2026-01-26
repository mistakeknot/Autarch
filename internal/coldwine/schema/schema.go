// Package schema provides data model validation and schema review capabilities.
// The "auditor" subagent audits proposed data structures for correctness, efficiency,
// normalization, and identifies PII fields that need special handling.
package schema

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// NormalizationForm represents database normalization levels
type NormalizationForm string

const (
	Form1NF   NormalizationForm = "1NF" // First Normal Form
	Form2NF   NormalizationForm = "2NF" // Second Normal Form
	Form3NF   NormalizationForm = "3NF" // Third Normal Form
	FormBCNF  NormalizationForm = "BCNF" // Boyce-Codd Normal Form
	FormDenorm NormalizationForm = "denormalized" // Intentionally denormalized
)

// ConcernSeverity indicates the severity of a schema concern
type ConcernSeverity string

const (
	SeverityCritical ConcernSeverity = "critical"
	SeverityHigh     ConcernSeverity = "high"
	SeverityMedium   ConcernSeverity = "medium"
	SeverityLow      ConcernSeverity = "low"
)

// PIIType represents types of personally identifiable information
type PIIType string

const (
	PIIEmail       PIIType = "email"
	PIIPhone       PIIType = "phone"
	PIIAddress     PIIType = "address"
	PIIName        PIIType = "name"
	PIISSN         PIIType = "ssn"
	PIIFinancial   PIIType = "financial"
	PIIMedical     PIIType = "medical"
	PIIBiometric   PIIType = "biometric"
)

// TableDefinition represents a database table structure
type TableDefinition struct {
	Name        string             `yaml:"name" json:"name"`
	Description string             `yaml:"description,omitempty" json:"description,omitempty"`
	Columns     []ColumnDefinition `yaml:"columns" json:"columns"`
	PrimaryKey  []string           `yaml:"primary_key" json:"primary_key"`
	ForeignKeys []ForeignKey       `yaml:"foreign_keys,omitempty" json:"foreign_keys,omitempty"`
}

// ColumnDefinition represents a column in a table
type ColumnDefinition struct {
	Name        string   `yaml:"name" json:"name"`
	Type        string   `yaml:"type" json:"type"`
	Nullable    bool     `yaml:"nullable" json:"nullable"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	PIITypes    []PIIType `yaml:"pii_types,omitempty" json:"pii_types,omitempty"`
}

// ForeignKey represents a foreign key relationship
type ForeignKey struct {
	Column     string `yaml:"column" json:"column"`
	References string `yaml:"references" json:"references"` // table.column
}

// IndexSuggestion suggests an index to improve query performance
type IndexSuggestion struct {
	TableName   string   `yaml:"table_name" json:"table_name"`
	Columns     []string `yaml:"columns" json:"columns"`
	IndexType   string   `yaml:"index_type" json:"index_type"` // btree, hash, gin, gist
	Rationale   string   `yaml:"rationale" json:"rationale"`
	Priority    ConcernSeverity `yaml:"priority" json:"priority"`
}

// SchemaConcern represents an issue found during schema review
type SchemaConcern struct {
	Type        string          `yaml:"type" json:"type"` // n+1_risk, denorm_without_rationale, missing_index, etc.
	Severity    ConcernSeverity `yaml:"severity" json:"severity"`
	TableName   string          `yaml:"table_name,omitempty" json:"table_name,omitempty"`
	ColumnName  string          `yaml:"column_name,omitempty" json:"column_name,omitempty"`
	Description string          `yaml:"description" json:"description"`
	Suggestion  string          `yaml:"suggestion" json:"suggestion"`
}

// MigrationStep represents a database migration step
type MigrationStep struct {
	Order       int    `yaml:"order" json:"order"`
	Description string `yaml:"description" json:"description"`
	SQL         string `yaml:"sql,omitempty" json:"sql,omitempty"` // Optional: actual DDL
	Reversible  bool   `yaml:"reversible" json:"reversible"`
	RollbackSQL string `yaml:"rollback_sql,omitempty" json:"rollback_sql,omitempty"`
}

// SchemaReview contains the complete schema audit results
type SchemaReview struct {
	SpecID             string              `yaml:"spec_id" json:"spec_id"`
	Tables             []TableDefinition   `yaml:"tables" json:"tables"`
	Indexes            []IndexSuggestion   `yaml:"indexes" json:"indexes"`
	Concerns           []SchemaConcern     `yaml:"concerns" json:"concerns"`
	PIIFields          []string            `yaml:"pii_fields" json:"pii_fields"`
	NormalizationLevel NormalizationForm   `yaml:"normalization_level" json:"normalization_level"`
	Migrations         []MigrationStep     `yaml:"migrations" json:"migrations"`
	Complexity         string              `yaml:"complexity" json:"complexity"` // simple, moderate, complex
}

// Auditor performs schema reviews on specs
type Auditor struct{}

// NewAuditor creates a new schema auditor
func NewAuditor() *Auditor {
	return &Auditor{}
}

// ReviewSpec analyzes a spec for data model patterns
func (a *Auditor) ReviewSpec(spec *specs.Spec) *SchemaReview {
	review := &SchemaReview{
		SpecID: spec.ID,
	}

	// Aggregate text for analysis
	allText := a.aggregateText(spec)

	// Extract implied tables
	review.Tables = a.extractTables(allText, spec.Requirements)

	// Identify PII fields
	review.PIIFields = a.identifyPIIFields(review.Tables)

	// Suggest indexes
	review.Indexes = a.suggestIndexes(review.Tables, allText)

	// Identify concerns
	review.Concerns = a.identifyConcerns(review.Tables, allText)

	// Assess normalization
	review.NormalizationLevel = a.assessNormalization(review.Tables)

	// Generate migration plan
	review.Migrations = a.generateMigrations(review.Tables)

	// Assess complexity
	review.Complexity = a.assessComplexity(review)

	return review
}

// ReviewPRD analyzes a PRD for data model patterns
func (a *Auditor) ReviewPRD(prd *specs.PRD) *SchemaReview {
	review := &SchemaReview{
		SpecID: prd.ID,
	}

	// Aggregate text from all features
	var allText []string
	var reqs []string
	for _, feature := range prd.Features {
		allText = append(allText, feature.Title, feature.Summary)
		allText = append(allText, feature.Requirements...)
		reqs = append(reqs, feature.Requirements...)
	}
	text := strings.ToLower(strings.Join(allText, " "))

	review.Tables = a.extractTables(text, reqs)
	review.PIIFields = a.identifyPIIFields(review.Tables)
	review.Indexes = a.suggestIndexes(review.Tables, text)
	review.Concerns = a.identifyConcerns(review.Tables, text)
	review.NormalizationLevel = a.assessNormalization(review.Tables)
	review.Migrations = a.generateMigrations(review.Tables)
	review.Complexity = a.assessComplexity(review)

	return review
}

// aggregateText combines all spec text for analysis
func (a *Auditor) aggregateText(spec *specs.Spec) string {
	var parts []string
	parts = append(parts, spec.Title, spec.Summary)
	parts = append(parts, spec.Requirements...)
	for _, cuj := range spec.CriticalUserJourneys {
		parts = append(parts, cuj.Title)
		parts = append(parts, cuj.Steps...)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// extractTables identifies implied database tables from requirements
func (a *Auditor) extractTables(text string, requirements []string) []TableDefinition {
	var tables []TableDefinition
	seen := make(map[string]bool)

	// Common entity patterns that imply tables
	entityPatterns := []struct {
		pattern *regexp.Regexp
		entity  string
	}{
		{regexp.MustCompile(`(?i)(user|users|account|accounts)`), "users"},
		{regexp.MustCompile(`(?i)(post|posts|article|articles)`), "posts"},
		{regexp.MustCompile(`(?i)(comment|comments)`), "comments"},
		{regexp.MustCompile(`(?i)(order|orders|purchase)`), "orders"},
		{regexp.MustCompile(`(?i)(product|products|item|items)`), "products"},
		{regexp.MustCompile(`(?i)(category|categories)`), "categories"},
		{regexp.MustCompile(`(?i)(tag|tags)`), "tags"},
		{regexp.MustCompile(`(?i)(payment|payments|transaction)`), "payments"},
		{regexp.MustCompile(`(?i)(session|sessions)`), "sessions"},
		{regexp.MustCompile(`(?i)(notification|notifications)`), "notifications"},
		{regexp.MustCompile(`(?i)(message|messages)`), "messages"},
		{regexp.MustCompile(`(?i)(file|files|upload|attachment)`), "files"},
		{regexp.MustCompile(`(?i)(setting|settings|preference)`), "settings"},
		{regexp.MustCompile(`(?i)(role|roles|permission)`), "roles"},
	}

	for _, ep := range entityPatterns {
		if ep.pattern.MatchString(text) && !seen[ep.entity] {
			seen[ep.entity] = true
			table := a.createTableDefinition(ep.entity, text)
			tables = append(tables, table)
		}
	}

	// Look for "store X" or "persist X" patterns
	storePattern := regexp.MustCompile(`(?i)(store|persist|save|track)\s+(\w+)`)
	matches := storePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 2 {
			entity := strings.ToLower(match[2])
			if !seen[entity] && len(entity) > 2 {
				seen[entity] = true
				table := a.createTableDefinition(pluralize(entity), text)
				tables = append(tables, table)
			}
		}
	}

	return tables
}

// createTableDefinition creates a basic table definition with common columns
func (a *Auditor) createTableDefinition(name string, text string) TableDefinition {
	table := TableDefinition{
		Name:       name,
		PrimaryKey: []string{"id"},
		Columns: []ColumnDefinition{
			{Name: "id", Type: "uuid", Nullable: false, Description: "Primary key"},
			{Name: "created_at", Type: "timestamp", Nullable: false, Description: "Creation timestamp"},
			{Name: "updated_at", Type: "timestamp", Nullable: false, Description: "Last update timestamp"},
		},
	}

	// Add common columns based on table name
	switch name {
	case "users":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "email", Type: "varchar(255)", Nullable: false, PIITypes: []PIIType{PIIEmail}},
			ColumnDefinition{Name: "password_hash", Type: "varchar(255)", Nullable: false},
			ColumnDefinition{Name: "name", Type: "varchar(255)", Nullable: true, PIITypes: []PIIType{PIIName}},
		)
	case "posts", "articles":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "user_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "title", Type: "varchar(255)", Nullable: false},
			ColumnDefinition{Name: "content", Type: "text", Nullable: true},
			ColumnDefinition{Name: "published_at", Type: "timestamp", Nullable: true},
		)
		table.ForeignKeys = []ForeignKey{{Column: "user_id", References: "users.id"}}
	case "comments":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "user_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "post_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "content", Type: "text", Nullable: false},
		)
		table.ForeignKeys = []ForeignKey{
			{Column: "user_id", References: "users.id"},
			{Column: "post_id", References: "posts.id"},
		}
	case "orders":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "user_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "status", Type: "varchar(50)", Nullable: false},
			ColumnDefinition{Name: "total_amount", Type: "decimal(10,2)", Nullable: false, PIITypes: []PIIType{PIIFinancial}},
		)
		table.ForeignKeys = []ForeignKey{{Column: "user_id", References: "users.id"}}
	case "products":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "name", Type: "varchar(255)", Nullable: false},
			ColumnDefinition{Name: "description", Type: "text", Nullable: true},
			ColumnDefinition{Name: "price", Type: "decimal(10,2)", Nullable: false},
			ColumnDefinition{Name: "stock", Type: "integer", Nullable: false, Description: "Available inventory"},
		)
	case "payments":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "order_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "amount", Type: "decimal(10,2)", Nullable: false, PIITypes: []PIIType{PIIFinancial}},
			ColumnDefinition{Name: "status", Type: "varchar(50)", Nullable: false},
			ColumnDefinition{Name: "provider", Type: "varchar(100)", Nullable: false},
		)
		table.ForeignKeys = []ForeignKey{{Column: "order_id", References: "orders.id"}}
	case "sessions":
		table.Columns = append(table.Columns,
			ColumnDefinition{Name: "user_id", Type: "uuid", Nullable: false},
			ColumnDefinition{Name: "token_hash", Type: "varchar(255)", Nullable: false},
			ColumnDefinition{Name: "expires_at", Type: "timestamp", Nullable: false},
			ColumnDefinition{Name: "ip_address", Type: "varchar(45)", Nullable: true, PIITypes: []PIIType{PIIAddress}},
		)
		table.ForeignKeys = []ForeignKey{{Column: "user_id", References: "users.id"}}
	}

	return table
}

// identifyPIIFields extracts all PII field paths from tables
func (a *Auditor) identifyPIIFields(tables []TableDefinition) []string {
	var piiFields []string
	for _, table := range tables {
		for _, col := range table.Columns {
			if len(col.PIITypes) > 0 {
				piiFields = append(piiFields, fmt.Sprintf("%s.%s", table.Name, col.Name))
			}
		}
	}
	return piiFields
}

// suggestIndexes recommends indexes based on likely query patterns
func (a *Auditor) suggestIndexes(tables []TableDefinition, text string) []IndexSuggestion {
	var indexes []IndexSuggestion

	for _, table := range tables {
		// Foreign keys should be indexed
		for _, fk := range table.ForeignKeys {
			indexes = append(indexes, IndexSuggestion{
				TableName: table.Name,
				Columns:   []string{fk.Column},
				IndexType: "btree",
				Rationale: "Foreign key - improves JOIN performance",
				Priority:  SeverityHigh,
			})
		}

		// Common query patterns
		for _, col := range table.Columns {
			// Status columns are often filtered
			if col.Name == "status" {
				indexes = append(indexes, IndexSuggestion{
					TableName: table.Name,
					Columns:   []string{col.Name},
					IndexType: "btree",
					Rationale: "Status filtering is common in queries",
					Priority:  SeverityMedium,
				})
			}
			// Email lookups for users
			if col.Name == "email" {
				indexes = append(indexes, IndexSuggestion{
					TableName: table.Name,
					Columns:   []string{col.Name},
					IndexType: "btree",
					Rationale: "Email lookup for authentication",
					Priority:  SeverityHigh,
				})
			}
			// Created_at for ordering
			if col.Name == "created_at" && table.Name != "users" {
				indexes = append(indexes, IndexSuggestion{
					TableName: table.Name,
					Columns:   []string{col.Name},
					IndexType: "btree",
					Rationale: "Chronological ordering is common",
					Priority:  SeverityLow,
				})
			}
		}
	}

	// Check for search patterns that might need full-text indexes
	searchPatterns := regexp.MustCompile(`(?i)(search|find|filter|query)`)
	if searchPatterns.MatchString(text) {
		for _, table := range tables {
			for _, col := range table.Columns {
				if col.Type == "text" || (strings.HasPrefix(col.Type, "varchar") && col.Name != "email") {
					if col.Name == "title" || col.Name == "content" || col.Name == "description" || col.Name == "name" {
						indexes = append(indexes, IndexSuggestion{
							TableName: table.Name,
							Columns:   []string{col.Name},
							IndexType: "gin",
							Rationale: "Full-text search capability detected",
							Priority:  SeverityMedium,
						})
					}
				}
			}
		}
	}

	return indexes
}

// identifyConcerns finds potential issues in the schema
func (a *Auditor) identifyConcerns(tables []TableDefinition, text string) []SchemaConcern {
	var concerns []SchemaConcern

	// Check for N+1 query risks
	for _, table := range tables {
		if len(table.ForeignKeys) > 0 {
			concerns = append(concerns, SchemaConcern{
				Type:        "n+1_risk",
				Severity:    SeverityMedium,
				TableName:   table.Name,
				Description: fmt.Sprintf("Table %s has foreign keys - ensure queries use JOINs or batch loading", table.Name),
				Suggestion:  "Use eager loading or batch queries to avoid N+1 query patterns",
			})
		}
	}

	// Check for missing soft delete
	for _, table := range tables {
		hasDeletedAt := false
		for _, col := range table.Columns {
			if col.Name == "deleted_at" {
				hasDeletedAt = true
				break
			}
		}
		if !hasDeletedAt && (table.Name == "users" || table.Name == "orders") {
			concerns = append(concerns, SchemaConcern{
				Type:        "missing_soft_delete",
				Severity:    SeverityMedium,
				TableName:   table.Name,
				Description: fmt.Sprintf("Table %s may need soft delete for audit/recovery", table.Name),
				Suggestion:  "Add deleted_at timestamp column for soft deletes",
			})
		}
	}

	// Check for potential denormalization needs
	listPatterns := regexp.MustCompile(`(?i)(list|feed|timeline|dashboard)`)
	if listPatterns.MatchString(text) {
		concerns = append(concerns, SchemaConcern{
			Type:        "denorm_consideration",
			Severity:    SeverityLow,
			Description: "List/feed patterns detected - consider denormalization for read performance",
			Suggestion:  "Evaluate if denormalized views or materialized views would improve read performance",
		})
	}

	// Check for PII without encryption consideration
	for _, table := range tables {
		for _, col := range table.Columns {
			if len(col.PIITypes) > 0 {
				concerns = append(concerns, SchemaConcern{
					Type:        "pii_encryption",
					Severity:    SeverityHigh,
					TableName:   table.Name,
					ColumnName:  col.Name,
					Description: fmt.Sprintf("PII field %s.%s may require encryption at rest", table.Name, col.Name),
					Suggestion:  "Implement column-level encryption or use encrypted storage",
				})
			}
		}
	}

	// Check for scale patterns
	scalePatterns := regexp.MustCompile(`(?i)(million|scale|high.?volume|enterprise)`)
	if scalePatterns.MatchString(text) {
		concerns = append(concerns, SchemaConcern{
			Type:        "scale_consideration",
			Severity:    SeverityHigh,
			Description: "Scale requirements detected - consider partitioning and sharding strategies",
			Suggestion:  "Evaluate table partitioning, read replicas, and horizontal sharding options",
		})
	}

	return concerns
}

// assessNormalization determines the appropriate normalization level
func (a *Auditor) assessNormalization(tables []TableDefinition) NormalizationForm {
	if len(tables) == 0 {
		return Form3NF // Default
	}

	// Check for proper foreign key relationships
	hasProperRelationships := true
	for _, table := range tables {
		if len(table.ForeignKeys) == 0 && table.Name != "users" && table.Name != "products" {
			// Tables that typically should have relationships
			if table.Name == "comments" || table.Name == "orders" || table.Name == "payments" {
				hasProperRelationships = false
			}
		}
	}

	if hasProperRelationships {
		return Form3NF
	}
	return Form2NF
}

// generateMigrations creates migration steps for the tables
func (a *Auditor) generateMigrations(tables []TableDefinition) []MigrationStep {
	var migrations []MigrationStep

	// Create tables first (in dependency order)
	order := 1

	// Sort tables: those without foreign keys first
	var noDeps, withDeps []TableDefinition
	for _, t := range tables {
		if len(t.ForeignKeys) == 0 {
			noDeps = append(noDeps, t)
		} else {
			withDeps = append(withDeps, t)
		}
	}
	sortedTables := append(noDeps, withDeps...)

	for _, table := range sortedTables {
		migrations = append(migrations, MigrationStep{
			Order:       order,
			Description: fmt.Sprintf("Create %s table", table.Name),
			Reversible:  true,
		})
		order++
	}

	// Add indexes
	migrations = append(migrations, MigrationStep{
		Order:       order,
		Description: "Create indexes for foreign keys and common query patterns",
		Reversible:  true,
	})

	return migrations
}

// assessComplexity determines overall schema complexity
func (a *Auditor) assessComplexity(review *SchemaReview) string {
	tableCount := len(review.Tables)
	concernCount := 0
	for _, c := range review.Concerns {
		if c.Severity == SeverityHigh || c.Severity == SeverityCritical {
			concernCount++
		}
	}

	if tableCount > 10 || concernCount > 5 {
		return "complex"
	}
	if tableCount > 5 || concernCount > 2 {
		return "moderate"
	}
	return "simple"
}

// FormatSchemaReview formats the review as markdown
func FormatSchemaReview(review *SchemaReview) string {
	var sb strings.Builder

	sb.WriteString("# Schema Review Report\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n", review.SpecID))
	sb.WriteString(fmt.Sprintf("**Complexity:** %s\n", review.Complexity))
	sb.WriteString(fmt.Sprintf("**Normalization:** %s\n\n", review.NormalizationLevel))

	// Tables
	if len(review.Tables) > 0 {
		sb.WriteString("## Identified Tables\n\n")
		for _, table := range review.Tables {
			sb.WriteString(fmt.Sprintf("### %s\n\n", table.Name))
			sb.WriteString("| Column | Type | Nullable | PII |\n")
			sb.WriteString("|--------|------|----------|-----|\n")
			for _, col := range table.Columns {
				pii := ""
				if len(col.PIITypes) > 0 {
					var types []string
					for _, t := range col.PIITypes {
						types = append(types, string(t))
					}
					pii = strings.Join(types, ", ")
				}
				sb.WriteString(fmt.Sprintf("| %s | %s | %v | %s |\n", col.Name, col.Type, col.Nullable, pii))
			}
			if len(table.ForeignKeys) > 0 {
				sb.WriteString("\n**Foreign Keys:**\n")
				for _, fk := range table.ForeignKeys {
					sb.WriteString(fmt.Sprintf("- %s → %s\n", fk.Column, fk.References))
				}
			}
			sb.WriteString("\n")
		}
	}

	// PII Summary
	if len(review.PIIFields) > 0 {
		sb.WriteString("## PII Fields\n\n")
		sb.WriteString("⚠️ The following fields contain personally identifiable information:\n\n")
		for _, field := range review.PIIFields {
			sb.WriteString(fmt.Sprintf("- `%s`\n", field))
		}
		sb.WriteString("\n")
	}

	// Index Suggestions
	if len(review.Indexes) > 0 {
		sb.WriteString("## Suggested Indexes\n\n")
		for _, idx := range review.Indexes {
			icon := severityIcon(idx.Priority)
			sb.WriteString(fmt.Sprintf("%s **%s** (%s) on `%s`\n", icon, strings.Join(idx.Columns, ", "), idx.IndexType, idx.TableName))
			sb.WriteString(fmt.Sprintf("   - %s\n", idx.Rationale))
		}
		sb.WriteString("\n")
	}

	// Concerns
	if len(review.Concerns) > 0 {
		sb.WriteString("## Concerns\n\n")
		for _, concern := range review.Concerns {
			icon := severityIcon(concern.Severity)
			sb.WriteString(fmt.Sprintf("### %s %s\n\n", icon, concern.Type))
			if concern.TableName != "" {
				sb.WriteString(fmt.Sprintf("**Table:** %s", concern.TableName))
				if concern.ColumnName != "" {
					sb.WriteString(fmt.Sprintf(".%s", concern.ColumnName))
				}
				sb.WriteString("\n\n")
			}
			sb.WriteString(fmt.Sprintf("%s\n\n", concern.Description))
			sb.WriteString(fmt.Sprintf("**Suggestion:** %s\n\n", concern.Suggestion))
		}
	}

	// Migration Steps
	if len(review.Migrations) > 0 {
		sb.WriteString("## Migration Plan\n\n")
		for _, mig := range review.Migrations {
			sb.WriteString(fmt.Sprintf("%d. %s\n", mig.Order, mig.Description))
		}
	}

	return sb.String()
}

// BuildAuditorBrief creates an agent brief for schema review
func BuildAuditorBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Auditor Agent Brief: Schema Review\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Analyze the spec for data model patterns and provide schema recommendations.\n\n")

	sb.WriteString("## Spec to Analyze\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", spec.Summary))

	if len(spec.Requirements) > 0 {
		sb.WriteString("### Requirements\n")
		for _, req := range spec.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Analysis Areas\n\n")
	sb.WriteString("- **Tables:** Identify implied database tables\n")
	sb.WriteString("- **Normalization:** Assess appropriate normalization level\n")
	sb.WriteString("- **PII:** Flag fields containing personal information\n")
	sb.WriteString("- **Indexes:** Suggest indexes for query performance\n")
	sb.WriteString("- **N+1 Risks:** Identify potential N+1 query patterns\n")
	sb.WriteString("- **Migrations:** Plan database migration steps\n")

	return sb.String()
}

// --- Helper functions ---

func severityIcon(s ConcernSeverity) string {
	switch s {
	case SeverityCritical:
		return "🔴"
	case SeverityHigh:
		return "🟠"
	case SeverityMedium:
		return "🟡"
	case SeverityLow:
		return "🟢"
	default:
		return "⚪"
	}
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
