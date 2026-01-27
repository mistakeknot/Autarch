// Package compound provides knowledge capture and retrieval for institutional learning.
// It follows the Compound Engineering pattern of documenting solved problems
// in a structured, searchable format.
package compound

import (
	"time"
)

// Solution represents a documented problem resolution.
// Each solution is stored as a markdown file with YAML frontmatter.
type Solution struct {
	// Module identifies which Autarch tool or area this solution belongs to.
	// Valid values: gurgeh, coldwine, pollard, bigend, integration
	Module string `yaml:"module"`

	// Date when the solution was documented (YYYY-MM-DD format).
	Date string `yaml:"date"`

	// ProblemType categorizes the kind of issue.
	// Examples: validation_error, integration_issue, performance, config, ui_bug
	ProblemType string `yaml:"problem_type"`

	// Component identifies the specific component affected.
	Component string `yaml:"component"`

	// Symptoms are observable behaviors that indicate this problem.
	Symptoms []string `yaml:"symptoms"`

	// RootCause explains why the problem occurred.
	RootCause string `yaml:"root_cause"`

	// Severity indicates impact level: low, medium, high, critical
	Severity string `yaml:"severity"`

	// Tags enable flexible searching and categorization.
	Tags []string `yaml:"tags"`
}

// SearchOptions configures how to filter solutions during search.
type SearchOptions struct {
	// Module filters to solutions from a specific tool.
	Module string

	// Tags filters to solutions containing any of these tags.
	Tags []string

	// Severity filters to solutions with this severity or higher.
	Severity string

	// ProblemType filters to a specific problem category.
	ProblemType string

	// Since filters to solutions documented after this date.
	Since time.Time

	// Query performs full-text search across symptoms and root cause.
	Query string
}

// SearchResult contains a matched solution with its file path.
type SearchResult struct {
	// Path is the relative path to the solution file.
	Path string

	// Solution contains the parsed frontmatter.
	Solution Solution

	// Title is extracted from the markdown H1 header.
	Title string

	// Score indicates match relevance (higher is better).
	Score float64
}

// ValidModules lists acceptable module values.
var ValidModules = []string{
	"gurgeh",
	"coldwine",
	"pollard",
	"bigend",
	"integration",
}

// ValidSeverities lists acceptable severity levels in ascending order.
var ValidSeverities = []string{
	"low",
	"medium",
	"high",
	"critical",
}

// ValidProblemTypes lists acceptable problem type values.
var ValidProblemTypes = []string{
	"validation_error",
	"integration_issue",
	"performance",
	"config",
	"ui_bug",
	"data_corruption",
	"concurrency",
}
