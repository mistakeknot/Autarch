// Package readiness provides implementation readiness checklists for specs.
// This module generates operational checklists covering feature flags, monitoring,
// rollback plans, migrations, and documentation requirements.
package readiness

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// ChecklistStatus indicates whether a checklist item is addressed
type ChecklistStatus string

const (
	StatusPending   ChecklistStatus = "pending"
	StatusAddressed ChecklistStatus = "addressed"
	StatusNA        ChecklistStatus = "n/a"
)

// Priority indicates checklist item priority
type Priority string

const (
	PriorityRequired  Priority = "required"  // Must be done before release
	PriorityRecommended Priority = "recommended" // Strongly suggested
	PriorityOptional   Priority = "optional"  // Nice to have
)

// FeatureFlagConfig defines feature flag requirements
type FeatureFlagConfig struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	DefaultOff  bool   `yaml:"default_off" json:"default_off"`
	Gradual     bool   `yaml:"gradual" json:"gradual"` // Supports percentage rollout
}

// MonitoringConfig defines monitoring requirements
type MonitoringConfig struct {
	Type        string `yaml:"type" json:"type"` // metric, log, alert
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Threshold   string `yaml:"threshold,omitempty" json:"threshold,omitempty"` // For alerts
}

// LoggingStrategy defines logging requirements
type LoggingStrategy struct {
	Level       string   `yaml:"level" json:"level"` // debug, info, warn, error
	Events      []string `yaml:"events" json:"events"`
	Structured  bool     `yaml:"structured" json:"structured"`
	Redactions  []string `yaml:"redactions,omitempty" json:"redactions,omitempty"` // Fields to redact
}

// RollbackPlan defines rollback procedures
type RollbackPlan struct {
	Strategy    string   `yaml:"strategy" json:"strategy"` // feature_flag, code_revert, data_restore
	Steps       []string `yaml:"steps" json:"steps"`
	Timeframe   string   `yaml:"timeframe" json:"timeframe"` // e.g., "within 5 minutes"
	DataBackup  bool     `yaml:"data_backup" json:"data_backup"`
}

// MigrationStep defines a migration requirement
type MigrationStep struct {
	Order        int    `yaml:"order" json:"order"`
	Description  string `yaml:"description" json:"description"`
	PreDeploy    bool   `yaml:"pre_deploy" json:"pre_deploy"`   // Run before code deploy
	PostDeploy   bool   `yaml:"post_deploy" json:"post_deploy"` // Run after code deploy
	Reversible   bool   `yaml:"reversible" json:"reversible"`
	RiskLevel    string `yaml:"risk_level" json:"risk_level"` // low, medium, high
}

// EnvVarRequirement defines required environment variables
type EnvVarRequirement struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required" json:"required"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Sensitive   bool   `yaml:"sensitive" json:"sensitive"` // Should be in secrets manager
}

// DocRequirement defines documentation requirements
type DocRequirement struct {
	Type        string `yaml:"type" json:"type"` // api, user, admin, runbook
	Description string `yaml:"description" json:"description"`
	Priority    Priority `yaml:"priority" json:"priority"`
}

// ChecklistItem represents a single checklist item
type ChecklistItem struct {
	ID          string          `yaml:"id" json:"id"`
	Category    string          `yaml:"category" json:"category"`
	Description string          `yaml:"description" json:"description"`
	Priority    Priority        `yaml:"priority" json:"priority"`
	Status      ChecklistStatus `yaml:"status" json:"status"`
	Notes       string          `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// ReadinessChecklist contains all readiness requirements
type ReadinessChecklist struct {
	SpecID        string              `yaml:"spec_id" json:"spec_id"`
	FeatureFlag   *FeatureFlagConfig  `yaml:"feature_flag,omitempty" json:"feature_flag,omitempty"`
	Monitoring    []MonitoringConfig  `yaml:"monitoring" json:"monitoring"`
	Logging       LoggingStrategy     `yaml:"logging" json:"logging"`
	Rollback      RollbackPlan        `yaml:"rollback" json:"rollback"`
	Migrations    []MigrationStep     `yaml:"migrations" json:"migrations"`
	EnvVars       []EnvVarRequirement `yaml:"env_vars" json:"env_vars"`
	Documentation []DocRequirement    `yaml:"documentation" json:"documentation"`
	Checklist     []ChecklistItem     `yaml:"checklist" json:"checklist"`
}

// Generator creates readiness checklists from specs
type Generator struct{}

// NewGenerator creates a new readiness generator
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateFromSpec creates a readiness checklist for a spec
func (g *Generator) GenerateFromSpec(spec *specs.Spec) *ReadinessChecklist {
	checklist := &ReadinessChecklist{
		SpecID: spec.ID,
	}

	// Aggregate text for analysis
	allText := g.aggregateText(spec)

	// Feature flag (recommended for all features)
	checklist.FeatureFlag = g.generateFeatureFlag(spec)

	// Monitoring
	checklist.Monitoring = g.generateMonitoring(allText, spec)

	// Logging
	checklist.Logging = g.generateLogging(allText)

	// Rollback
	checklist.Rollback = g.generateRollback(allText)

	// Migrations
	checklist.Migrations = g.generateMigrations(allText)

	// Environment variables
	checklist.EnvVars = g.generateEnvVars(allText)

	// Documentation
	checklist.Documentation = g.generateDocumentation(allText)

	// Build checklist items
	checklist.Checklist = g.buildChecklistItems(checklist, allText)

	return checklist
}

// GenerateFromPRD creates a readiness checklist for a PRD
func (g *Generator) GenerateFromPRD(prd *specs.PRD) *ReadinessChecklist {
	// Create synthetic spec
	syntheticSpec := &specs.Spec{
		ID:    prd.ID,
		Title: prd.Title,
	}
	for _, feature := range prd.Features {
		syntheticSpec.Requirements = append(syntheticSpec.Requirements, feature.Requirements...)
	}
	return g.GenerateFromSpec(syntheticSpec)
}

// aggregateText combines all spec text for analysis
func (g *Generator) aggregateText(spec *specs.Spec) string {
	var parts []string
	parts = append(parts, spec.Title, spec.Summary)
	parts = append(parts, spec.Requirements...)
	for _, cuj := range spec.CriticalUserJourneys {
		parts = append(parts, cuj.Title)
		parts = append(parts, cuj.Steps...)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// generateFeatureFlag creates feature flag config
func (g *Generator) generateFeatureFlag(spec *specs.Spec) *FeatureFlagConfig {
	name := strings.ToLower(strings.ReplaceAll(spec.Title, " ", "_"))
	name = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(name, "")

	return &FeatureFlagConfig{
		Name:        fmt.Sprintf("feature_%s", name),
		Description: fmt.Sprintf("Feature flag for %s", spec.Title),
		DefaultOff:  true,
		Gradual:     true,
	}
}

// generateMonitoring creates monitoring config
func (g *Generator) generateMonitoring(text string, spec *specs.Spec) []MonitoringConfig {
	var monitoring []MonitoringConfig

	// Basic success/error metrics
	monitoring = append(monitoring, MonitoringConfig{
		Type:        "metric",
		Name:        fmt.Sprintf("%s_requests_total", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
		Description: "Total requests for this feature",
	})
	monitoring = append(monitoring, MonitoringConfig{
		Type:        "metric",
		Name:        fmt.Sprintf("%s_errors_total", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
		Description: "Total errors for this feature",
	})

	// Latency metric
	monitoring = append(monitoring, MonitoringConfig{
		Type:        "metric",
		Name:        fmt.Sprintf("%s_latency_seconds", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
		Description: "Request latency histogram",
	})

	// Error rate alert
	monitoring = append(monitoring, MonitoringConfig{
		Type:        "alert",
		Name:        fmt.Sprintf("%s_high_error_rate", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
		Description: "Alert when error rate exceeds threshold",
		Threshold:   "> 1% over 5 minutes",
	})

	// Latency alert
	monitoring = append(monitoring, MonitoringConfig{
		Type:        "alert",
		Name:        fmt.Sprintf("%s_high_latency", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
		Description: "Alert when p95 latency exceeds threshold",
		Threshold:   "p95 > 2s over 5 minutes",
	})

	// Payment-specific monitoring
	if strings.Contains(text, "payment") || strings.Contains(text, "transaction") {
		monitoring = append(monitoring, MonitoringConfig{
			Type:        "alert",
			Name:        fmt.Sprintf("%s_payment_failures", strings.ToLower(strings.ReplaceAll(spec.ID, "-", "_"))),
			Description: "Alert on payment failures",
			Threshold:   "> 0 in 1 minute",
		})
	}

	return monitoring
}

// generateLogging creates logging strategy
func (g *Generator) generateLogging(text string) LoggingStrategy {
	logging := LoggingStrategy{
		Level:      "info",
		Structured: true,
		Events: []string{
			"feature_accessed",
			"action_started",
			"action_completed",
			"error_occurred",
		},
	}

	// Add redactions for sensitive data
	if strings.Contains(text, "password") || strings.Contains(text, "auth") {
		logging.Redactions = append(logging.Redactions, "password", "token", "secret")
	}
	if strings.Contains(text, "email") || strings.Contains(text, "user") {
		logging.Redactions = append(logging.Redactions, "email")
	}
	if strings.Contains(text, "payment") || strings.Contains(text, "card") {
		logging.Redactions = append(logging.Redactions, "card_number", "cvv", "billing_address")
	}

	return logging
}

// generateRollback creates rollback plan
func (g *Generator) generateRollback(text string) RollbackPlan {
	plan := RollbackPlan{
		Strategy:  "feature_flag",
		Timeframe: "within 5 minutes",
		Steps: []string{
			"1. Disable feature flag in configuration",
			"2. Monitor error rates for recovery",
			"3. Notify stakeholders of rollback",
		},
	}

	// Data-sensitive features need backup
	if strings.Contains(text, "migrate") || strings.Contains(text, "delete") ||
		strings.Contains(text, "update") || strings.Contains(text, "modify") {
		plan.DataBackup = true
		plan.Steps = append([]string{"0. Ensure database backup exists"}, plan.Steps...)
	}

	// If no feature flag possible, use code revert
	if strings.Contains(text, "critical") || strings.Contains(text, "core") {
		plan.Strategy = "code_revert"
		plan.Steps = []string{
			"1. Revert to previous deployment",
			"2. Run database rollback if needed",
			"3. Verify system stability",
			"4. Notify stakeholders",
		}
	}

	return plan
}

// generateMigrations creates migration requirements
func (g *Generator) generateMigrations(text string) []MigrationStep {
	var migrations []MigrationStep

	// Check for database changes
	if strings.Contains(text, "store") || strings.Contains(text, "persist") ||
		strings.Contains(text, "database") || strings.Contains(text, "table") {
		migrations = append(migrations, MigrationStep{
			Order:       1,
			Description: "Create new database tables/columns",
			PreDeploy:   true,
			Reversible:  true,
			RiskLevel:   "low",
		})
	}

	// Check for data transformation
	if strings.Contains(text, "migrate") || strings.Contains(text, "transform") ||
		strings.Contains(text, "convert") {
		migrations = append(migrations, MigrationStep{
			Order:       2,
			Description: "Migrate existing data to new format",
			PostDeploy:  true,
			Reversible:  false,
			RiskLevel:   "high",
		})
	}

	// Check for cache invalidation
	if strings.Contains(text, "cache") {
		migrations = append(migrations, MigrationStep{
			Order:       3,
			Description: "Invalidate related caches",
			PostDeploy:  true,
			Reversible:  true,
			RiskLevel:   "low",
		})
	}

	return migrations
}

// generateEnvVars creates environment variable requirements
func (g *Generator) generateEnvVars(text string) []EnvVarRequirement {
	var envVars []EnvVarRequirement

	// Feature flag env var
	envVars = append(envVars, EnvVarRequirement{
		Name:        "FEATURE_FLAGS_ENABLED",
		Description: "Enable feature flag system",
		Required:    true,
		Default:     "true",
		Sensitive:   false,
	})

	// Third-party integrations
	if strings.Contains(text, "api") || strings.Contains(text, "integrate") {
		envVars = append(envVars, EnvVarRequirement{
			Name:        "EXTERNAL_API_URL",
			Description: "External API endpoint URL",
			Required:    true,
			Sensitive:   false,
		})
		envVars = append(envVars, EnvVarRequirement{
			Name:        "EXTERNAL_API_KEY",
			Description: "External API authentication key",
			Required:    true,
			Sensitive:   true,
		})
	}

	// Payment integrations
	if strings.Contains(text, "payment") || strings.Contains(text, "stripe") {
		envVars = append(envVars, EnvVarRequirement{
			Name:        "PAYMENT_PROVIDER_KEY",
			Description: "Payment provider API key",
			Required:    true,
			Sensitive:   true,
		})
		envVars = append(envVars, EnvVarRequirement{
			Name:        "PAYMENT_WEBHOOK_SECRET",
			Description: "Webhook signature verification secret",
			Required:    true,
			Sensitive:   true,
		})
	}

	// Email integrations
	if strings.Contains(text, "email") || strings.Contains(text, "notification") {
		envVars = append(envVars, EnvVarRequirement{
			Name:        "EMAIL_SERVICE_API_KEY",
			Description: "Email service API key",
			Required:    true,
			Sensitive:   true,
		})
	}

	return envVars
}

// generateDocumentation creates documentation requirements
func (g *Generator) generateDocumentation(text string) []DocRequirement {
	docs := []DocRequirement{
		{
			Type:        "runbook",
			Description: "Operational runbook for this feature",
			Priority:    PriorityRequired,
		},
	}

	// API docs if API-related
	if strings.Contains(text, "api") || strings.Contains(text, "endpoint") {
		docs = append(docs, DocRequirement{
			Type:        "api",
			Description: "API documentation for new endpoints",
			Priority:    PriorityRequired,
		})
	}

	// User docs if user-facing
	if strings.Contains(text, "user") || strings.Contains(text, "interface") ||
		strings.Contains(text, "form") || strings.Contains(text, "page") {
		docs = append(docs, DocRequirement{
			Type:        "user",
			Description: "User-facing documentation/help content",
			Priority:    PriorityRecommended,
		})
	}

	// Admin docs if admin-related
	if strings.Contains(text, "admin") || strings.Contains(text, "manage") ||
		strings.Contains(text, "configure") {
		docs = append(docs, DocRequirement{
			Type:        "admin",
			Description: "Admin documentation for configuration",
			Priority:    PriorityRecommended,
		})
	}

	return docs
}

// buildChecklistItems creates the comprehensive checklist
func (g *Generator) buildChecklistItems(checklist *ReadinessChecklist, text string) []ChecklistItem {
	var items []ChecklistItem
	itemID := 1

	// Feature flag
	if checklist.FeatureFlag != nil {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "feature_flag",
			Description: "Create and configure feature flag",
			Priority:    PriorityRequired,
			Status:      StatusPending,
		})
		itemID++
	}

	// Monitoring
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "monitoring",
		Description: "Set up metrics collection",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "monitoring",
		Description: "Configure alerts for error rates and latency",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++

	// Logging
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "logging",
		Description: "Implement structured logging with appropriate events",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++
	if len(checklist.Logging.Redactions) > 0 {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "logging",
			Description: fmt.Sprintf("Configure log redactions for: %s", strings.Join(checklist.Logging.Redactions, ", ")),
			Priority:    PriorityRequired,
			Status:      StatusPending,
		})
		itemID++
	}

	// Rollback
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "rollback",
		Description: "Document and test rollback procedure",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++
	if checklist.Rollback.DataBackup {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "rollback",
			Description: "Ensure database backup exists before deployment",
			Priority:    PriorityRequired,
			Status:      StatusPending,
		})
		itemID++
	}

	// Migrations
	for _, mig := range checklist.Migrations {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "migration",
			Description: mig.Description,
			Priority:    PriorityRequired,
			Status:      StatusPending,
			Notes:       fmt.Sprintf("Risk level: %s, Reversible: %v", mig.RiskLevel, mig.Reversible),
		})
		itemID++
	}

	// Environment variables
	for _, env := range checklist.EnvVars {
		if env.Required {
			priority := PriorityRequired
			if env.Sensitive {
				items = append(items, ChecklistItem{
					ID:          fmt.Sprintf("CHK-%03d", itemID),
					Category:    "env_vars",
					Description: fmt.Sprintf("Configure %s in secrets manager", env.Name),
					Priority:    priority,
					Status:      StatusPending,
				})
			} else {
				items = append(items, ChecklistItem{
					ID:          fmt.Sprintf("CHK-%03d", itemID),
					Category:    "env_vars",
					Description: fmt.Sprintf("Set %s environment variable", env.Name),
					Priority:    priority,
					Status:      StatusPending,
				})
			}
			itemID++
		}
	}

	// Documentation
	for _, doc := range checklist.Documentation {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "documentation",
			Description: fmt.Sprintf("Create %s: %s", doc.Type, doc.Description),
			Priority:    doc.Priority,
			Status:      StatusPending,
		})
		itemID++
	}

	// Testing
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "testing",
		Description: "Write and pass unit tests",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "testing",
		Description: "Write and pass integration tests",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++

	// Code review
	items = append(items, ChecklistItem{
		ID:          fmt.Sprintf("CHK-%03d", itemID),
		Category:    "review",
		Description: "Complete code review",
		Priority:    PriorityRequired,
		Status:      StatusPending,
	})
	itemID++

	// Security review if sensitive
	if strings.Contains(text, "auth") || strings.Contains(text, "payment") ||
		strings.Contains(text, "password") || strings.Contains(text, "sensitive") {
		items = append(items, ChecklistItem{
			ID:          fmt.Sprintf("CHK-%03d", itemID),
			Category:    "review",
			Description: "Complete security review",
			Priority:    PriorityRequired,
			Status:      StatusPending,
		})
		itemID++
	}

	return items
}

// FormatReadinessChecklist formats the checklist as markdown
func FormatReadinessChecklist(checklist *ReadinessChecklist) string {
	var sb strings.Builder

	sb.WriteString("# Implementation Readiness Checklist\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n\n", checklist.SpecID))

	// Feature Flag
	if checklist.FeatureFlag != nil {
		sb.WriteString("## Feature Flag\n\n")
		sb.WriteString(fmt.Sprintf("- **Name:** `%s`\n", checklist.FeatureFlag.Name))
		sb.WriteString(fmt.Sprintf("- **Default Off:** %v\n", checklist.FeatureFlag.DefaultOff))
		sb.WriteString(fmt.Sprintf("- **Gradual Rollout:** %v\n\n", checklist.FeatureFlag.Gradual))
	}

	// Monitoring
	sb.WriteString("## Monitoring\n\n")
	for _, m := range checklist.Monitoring {
		sb.WriteString(fmt.Sprintf("- **[%s]** `%s`: %s", m.Type, m.Name, m.Description))
		if m.Threshold != "" {
			sb.WriteString(fmt.Sprintf(" (threshold: %s)", m.Threshold))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Logging
	sb.WriteString("## Logging\n\n")
	sb.WriteString(fmt.Sprintf("- **Level:** %s\n", checklist.Logging.Level))
	sb.WriteString(fmt.Sprintf("- **Structured:** %v\n", checklist.Logging.Structured))
	sb.WriteString("- **Events:**\n")
	for _, e := range checklist.Logging.Events {
		sb.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	if len(checklist.Logging.Redactions) > 0 {
		sb.WriteString("- **Redactions:**\n")
		for _, r := range checklist.Logging.Redactions {
			sb.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}
	sb.WriteString("\n")

	// Rollback
	sb.WriteString("## Rollback Plan\n\n")
	sb.WriteString(fmt.Sprintf("- **Strategy:** %s\n", checklist.Rollback.Strategy))
	sb.WriteString(fmt.Sprintf("- **Timeframe:** %s\n", checklist.Rollback.Timeframe))
	sb.WriteString(fmt.Sprintf("- **Data Backup Required:** %v\n", checklist.Rollback.DataBackup))
	sb.WriteString("- **Steps:**\n")
	for _, step := range checklist.Rollback.Steps {
		sb.WriteString(fmt.Sprintf("  %s\n", step))
	}
	sb.WriteString("\n")

	// Environment Variables
	if len(checklist.EnvVars) > 0 {
		sb.WriteString("## Environment Variables\n\n")
		sb.WriteString("| Name | Description | Required | Sensitive |\n")
		sb.WriteString("|------|-------------|----------|----------|\n")
		for _, env := range checklist.EnvVars {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %v | %v |\n", env.Name, env.Description, env.Required, env.Sensitive))
		}
		sb.WriteString("\n")
	}

	// Checklist
	sb.WriteString("## Checklist\n\n")
	currentCategory := ""
	for _, item := range checklist.Checklist {
		if item.Category != currentCategory {
			sb.WriteString(fmt.Sprintf("\n### %s\n\n", strings.Title(strings.ReplaceAll(item.Category, "_", " "))))
			currentCategory = item.Category
		}
		status := "[ ]"
		if item.Status == StatusAddressed {
			status = "[x]"
		} else if item.Status == StatusNA {
			status = "[~]"
		}
		priority := ""
		if item.Priority == PriorityRequired {
			priority = " **(required)**"
		}
		sb.WriteString(fmt.Sprintf("- %s %s: %s%s\n", status, item.ID, item.Description, priority))
		if item.Notes != "" {
			sb.WriteString(fmt.Sprintf("  - _%s_\n", item.Notes))
		}
	}

	return sb.String()
}

// BuildReadinessBrief creates an agent brief for readiness analysis
func BuildReadinessBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Readiness Agent Brief: Implementation Checklist\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Generate implementation readiness checklist for the spec.\n\n")

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

	sb.WriteString("## Checklist Areas\n\n")
	sb.WriteString("- **Feature Flags:** Gradual rollout configuration\n")
	sb.WriteString("- **Monitoring:** Metrics and alerts\n")
	sb.WriteString("- **Logging:** Events and redactions\n")
	sb.WriteString("- **Rollback:** Recovery procedures\n")
	sb.WriteString("- **Migrations:** Database changes\n")
	sb.WriteString("- **Environment:** Required variables\n")
	sb.WriteString("- **Documentation:** Required docs\n")

	return sb.String()
}
