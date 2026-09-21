// Package proposal provides research agenda proposals from project context.
package proposal

import "time"

// ProjectContext holds extracted project information from documentation.
type ProjectContext struct {
	ProjectName  string            `yaml:"project_name" json:"project_name"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	Technologies []string          `yaml:"technologies,omitempty" json:"technologies,omitempty"`
	Domain       string            `yaml:"domain,omitempty" json:"domain,omitempty"`
	Files        map[string]string `yaml:"files" json:"files"`                 // filename -> content (truncated)
	DetectedType string            `yaml:"detected_type" json:"detected_type"` // web, cli, library, api, monorepo
}

// ResearchAgenda represents a proposed research direction.
type ResearchAgenda struct {
	ID               string   `yaml:"id" json:"id"`
	Title            string   `yaml:"title" json:"title"`
	Description      string   `yaml:"description" json:"description"`
	Questions        []string `yaml:"questions" json:"questions"`
	SuggestedHunters []string `yaml:"suggested_hunters" json:"suggested_hunters"`
	EstimatedScope   string   `yaml:"estimated_scope" json:"estimated_scope"` // narrow, medium, broad
	Priority         string   `yaml:"priority" json:"priority"`               // high, medium, low
}

// ProposalResult holds the agent's research agenda proposals.
type ProposalResult struct {
	ProjectContext ProjectContext   `yaml:"project_context" json:"project_context"`
	Agendas        []ResearchAgenda `yaml:"agendas" json:"agendas"`
	GeneratedAt    time.Time        `yaml:"generated_at" json:"generated_at"`
	AgentUsed      string           `yaml:"agent_used" json:"agent_used"`
}

// ProposalConfig controls proposal generation behavior.
type ProposalConfig struct {
	MaxAgendas   int      `yaml:"max_agendas" json:"max_agendas"`
	IncludeSrc   bool     `yaml:"include_src" json:"include_src"`
	OutputFormat string   `yaml:"output_format" json:"output_format"`         // yaml, json, markdown
	Hunters      []string `yaml:"hunters,omitempty" json:"hunters,omitempty"` // available hunters
}

// DefaultConfig returns the default proposal configuration.
func DefaultConfig() ProposalConfig {
	return ProposalConfig{
		MaxAgendas:   5,
		IncludeSrc:   false,
		OutputFormat: "yaml",
		Hunters: []string{
			"github-scout",
			"hackernews",
			"arxiv",
			"openalex",
			"pubmed",
			"competitor-tracker",
		},
	}
}
