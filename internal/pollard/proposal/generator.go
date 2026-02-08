package proposal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

// AgendaGenerator uses the user's AI agent to propose research agendas.
type AgendaGenerator struct {
	agentCommand string
	agentArgs    []string
	config       ProposalConfig
}

// NewAgendaGenerator creates a generator using environment configuration.
func NewAgendaGenerator() *AgendaGenerator {
	// Use environment variable to configure agent
	cmd := os.Getenv("POLLARD_AGENT_COMMAND")
	if cmd == "" {
		cmd = "claude" // Default to Claude
	}

	args := os.Getenv("POLLARD_AGENT_ARGS")
	var argList []string
	if args != "" {
		argList = strings.Fields(args)
	} else {
		argList = []string{"--print"} // Default: print mode for structured output
	}

	return &AgendaGenerator{
		agentCommand: cmd,
		agentArgs:    argList,
		config:       DefaultConfig(),
	}
}

// NewAgendaGeneratorWithConfig creates a generator with specific config.
func NewAgendaGeneratorWithConfig(cfg ProposalConfig) *AgendaGenerator {
	g := NewAgendaGenerator()
	g.config = cfg
	return g
}

// Generate invokes the AI agent to propose research agendas.
func (g *AgendaGenerator) Generate(ctx context.Context, pc *ProjectContext) (*ProposalResult, error) {
	prompt := g.buildPrompt(pc)

	output, err := g.invokeAgent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("agent invocation failed: %w", err)
	}

	agendas, err := g.parseAgentOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent output: %w", err)
	}

	// Limit to max agendas
	if len(agendas) > g.config.MaxAgendas {
		agendas = agendas[:g.config.MaxAgendas]
	}

	return &ProposalResult{
		ProjectContext: *pc,
		Agendas:        agendas,
		GeneratedAt:    time.Now(),
		AgentUsed:      g.agentCommand,
	}, nil
}

// buildPrompt creates the prompt for the AI agent.
func (g *AgendaGenerator) buildPrompt(pc *ProjectContext) string {
	var prompt strings.Builder

	prompt.WriteString("# Research Agenda Proposal Request\n\n")
	prompt.WriteString("Analyze this project and propose research agendas.\n\n")

	// Project context
	prompt.WriteString("## Project Context\n\n")
	prompt.WriteString(fmt.Sprintf("**Project Name:** %s\n", pc.ProjectName))
	if len(pc.Technologies) > 0 {
		prompt.WriteString(fmt.Sprintf("**Technologies:** %s\n", strings.Join(pc.Technologies, ", ")))
	}
	if pc.DetectedType != "" && pc.DetectedType != "unknown" {
		prompt.WriteString(fmt.Sprintf("**Type:** %s\n", pc.DetectedType))
	}
	if pc.Domain != "" {
		prompt.WriteString(fmt.Sprintf("**Domain:** %s\n", pc.Domain))
	}
	if pc.Description != "" {
		prompt.WriteString(fmt.Sprintf("**Description:** %s\n", pc.Description))
	}
	prompt.WriteString("\n")

	// Include documentation files
	for filename, content := range pc.Files {
		prompt.WriteString(fmt.Sprintf("### %s\n", filename))
		prompt.WriteString("```\n")
		prompt.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			prompt.WriteString("\n")
		}
		prompt.WriteString("```\n\n")
	}

	// Available hunters
	prompt.WriteString("## Available Hunters\n\n")
	hunterDescriptions := map[string]string{
		"github-scout":       "Open source repositories, implementations, libraries",
		"hackernews":         "Trending tech discussions, community opinions",
		"arxiv":              "AI/CS research papers, academic publications",
		"openalex":           "Multi-domain academic research across all fields",
		"pubmed":             "Medical and life sciences research",
		"competitor-tracker": "Competitor monitoring, changelog tracking",
		"usda-nutrition":     "Food and nutrition data (requires API key)",
		"legal":              "Court opinions and legal cases (requires API key)",
		"economics":          "Economic indicators and statistics",
		"wiki":               "Wikipedia and Wikidata knowledge",
	}
	for _, h := range g.config.Hunters {
		desc := hunterDescriptions[h]
		if desc == "" {
			desc = "Custom research hunter"
		}
		prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", h, desc))
	}
	prompt.WriteString("\n")

	// Output format
	prompt.WriteString("## Output Format\n\n")
	prompt.WriteString(fmt.Sprintf("Propose %d research agendas. ", g.config.MaxAgendas))
	prompt.WriteString("Respond with ONLY valid YAML:\n\n")
	prompt.WriteString("```yaml\n")
	prompt.WriteString("agendas:\n")
	prompt.WriteString("  - id: \"agenda-1\"\n")
	prompt.WriteString("    title: \"Concise agenda title\"\n")
	prompt.WriteString("    description: \"What this research would uncover and why it matters\"\n")
	prompt.WriteString("    questions:\n")
	prompt.WriteString("      - \"Specific research question 1\"\n")
	prompt.WriteString("      - \"Specific research question 2\"\n")
	prompt.WriteString("    suggested_hunters:\n")
	prompt.WriteString("      - \"hunter-name\"\n")
	prompt.WriteString("    estimated_scope: \"narrow|medium|broad\"\n")
	prompt.WriteString("    priority: \"high|medium|low\"\n")
	prompt.WriteString("```\n\n")

	// Guidelines
	prompt.WriteString("## Guidelines\n\n")
	prompt.WriteString("- Focus on research that would inform development decisions\n")
	prompt.WriteString("- Prioritize agendas that match available hunters\n")
	prompt.WriteString("- Make questions specific enough to generate useful search queries\n")
	prompt.WriteString("- Consider both technical implementation and market/user research\n")
	prompt.WriteString("- Order agendas by strategic importance\n")

	return prompt.String()
}

// invokeAgent runs the user's AI agent with the prompt.
func (g *AgendaGenerator) invokeAgent(ctx context.Context, prompt string) (string, error) {
	args := append(g.agentArgs, prompt)

	cmd := exec.CommandContext(ctx, g.agentCommand, args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("agent command failed: %w, stderr: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("agent command failed: %w", err)
	}

	return string(output), nil
}

// parseAgentOutput extracts agendas from the agent's response.
func (g *AgendaGenerator) parseAgentOutput(output string) ([]ResearchAgenda, error) {
	// Try to find YAML block
	yamlContent := extractYAML(output)
	if yamlContent == "" {
		yamlContent = output // Try parsing entire output
	}

	var result struct {
		Agendas []ResearchAgenda `yaml:"agendas"`
	}

	if err := yamlsafe.Decode([]byte(yamlContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate and fix agendas
	for i := range result.Agendas {
		if result.Agendas[i].ID == "" {
			result.Agendas[i].ID = fmt.Sprintf("agenda-%d", i+1)
		}
		if result.Agendas[i].EstimatedScope == "" {
			result.Agendas[i].EstimatedScope = "medium"
		}
		if result.Agendas[i].Priority == "" {
			result.Agendas[i].Priority = "medium"
		}
	}

	return result.Agendas, nil
}

// extractYAML finds YAML content in agent output.
func extractYAML(output string) string {
	// Try to find YAML block between triple backticks
	if idx := strings.Index(output, "```yaml"); idx != -1 {
		start := idx + 7
		if endIdx := strings.Index(output[start:], "```"); endIdx != -1 {
			return strings.TrimSpace(output[start : start+endIdx])
		}
	}

	// Try plain code block
	if idx := strings.Index(output, "```"); idx != -1 {
		start := idx + 3
		// Skip optional language identifier
		if nlIdx := strings.Index(output[start:], "\n"); nlIdx != -1 {
			start += nlIdx + 1
		}
		if endIdx := strings.Index(output[start:], "```"); endIdx != -1 {
			return strings.TrimSpace(output[start : start+endIdx])
		}
	}

	// Try to find agendas: directly
	if idx := strings.Index(output, "agendas:"); idx != -1 {
		return strings.TrimSpace(output[idx:])
	}

	return ""
}

// SaveResult saves the proposal result to the proposals directory.
func SaveResult(projectPath string, result *ProposalResult) error {
	proposalsDir := filepath.Join(projectPath, ".pollard", "proposals")
	if err := os.MkdirAll(proposalsDir, 0755); err != nil {
		return fmt.Errorf("failed to create proposals dir: %w", err)
	}

	// Save as current.yaml
	currentPath := filepath.Join(proposalsDir, "current.yaml")
	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(currentPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write result: %w", err)
	}

	// Also save timestamped archive
	archivePath := filepath.Join(proposalsDir, fmt.Sprintf("%s-proposals.yaml",
		result.GeneratedAt.Format("2006-01-02-150405")))
	if err := os.WriteFile(archivePath, data, 0644); err != nil {
		// Archive failure is not critical
		return nil
	}

	return nil
}

// LoadResult loads the current proposal result.
func LoadResult(projectPath string) (*ProposalResult, error) {
	currentPath := filepath.Join(projectPath, ".pollard", "proposals", "current.yaml")
	data, err := os.ReadFile(currentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read proposals: %w", err)
	}

	var result ProposalResult
	if err := yamlsafe.Decode(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse proposals: %w", err)
	}

	return &result, nil
}

// SetAgent configures the agent command and arguments.
func (g *AgendaGenerator) SetAgent(command string, args []string) {
	g.agentCommand = command
	g.agentArgs = args
}
