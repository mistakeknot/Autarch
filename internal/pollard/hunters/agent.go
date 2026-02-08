package hunters

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"

	"github.com/mistakeknot/autarch/pkg/thinking"
)

// AgentHunter uses the user's AI agent (Claude, Codex) to conduct research.
// This is the PRIMARY research mechanism - APIs are supplementary.
type AgentHunter struct {
	agentCommand string
	agentArgs    []string
}

// NewAgentHunter creates a hunter that uses the configured AI agent.
func NewAgentHunter() *AgentHunter {
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

	return &AgentHunter{
		agentCommand: cmd,
		agentArgs:    argList,
	}
}

// Name returns the hunter's identifier.
func (h *AgentHunter) Name() string {
	return "agent-research"
}

// Hunt generates a research brief and invokes the user's AI agent.
func (h *AgentHunter) Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error) {
	result := &HuntResult{
		HunterName: h.Name(),
		StartedAt:  time.Now(),
	}

	// Generate research brief from queries
	brief := h.generateResearchBrief(cfg)

	// Write brief to temp file
	briefPath := filepath.Join(cfg.ProjectPath, ".pollard", "temp", "research-brief.md")
	if err := os.MkdirAll(filepath.Dir(briefPath), 0755); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("create temp dir: %w", err))
		result.CompletedAt = time.Now()
		return result, nil
	}
	if err := os.WriteFile(briefPath, []byte(brief), 0644); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("write brief: %w", err))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Invoke agent with brief
	output, err := h.invokeAgent(ctx, brief, cfg.ProjectPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("agent invocation failed: %w", err))
		result.CompletedAt = time.Now()
		return result, nil
	}

	// Parse agent output into structured sources
	sources, parseErr := h.parseAgentOutput(output)
	if parseErr != nil {
		result.Errors = append(result.Errors, fmt.Errorf("parse output: %w", parseErr))
	}

	// Save results
	if len(sources) > 0 {
		outputFile, saveErr := h.saveResults(cfg, sources)
		if saveErr != nil {
			result.Errors = append(result.Errors, saveErr)
		} else {
			result.OutputFiles = append(result.OutputFiles, outputFile)
		}
	}

	result.SourcesCollected = len(sources)
	result.CompletedAt = time.Now()
	return result, nil
}

// generateResearchBrief creates a prompt for the AI agent.
func (h *AgentHunter) generateResearchBrief(cfg HunterConfig) string {
	var brief strings.Builder

	// Apply thinking shape preamble based on query type
	shape := h.resolveShape(cfg)
	if preamble := thinking.RenderPreamble(shape, h.shapePhaseName(shape)); preamble != "" {
		brief.WriteString(thinking.FormatPreamble(preamble))
	}

	brief.WriteString("# Research Brief\n\n")
	brief.WriteString("Conduct research on the following topics and return structured findings.\n\n")

	brief.WriteString("## Research Questions\n")
	for i, query := range cfg.Queries {
		brief.WriteString(fmt.Sprintf("%d. %s\n", i+1, query))
	}

	brief.WriteString("\n## Expected Output Format\n")
	brief.WriteString("Return findings as YAML with this structure:\n")
	brief.WriteString("```yaml\n")
	brief.WriteString("sources:\n")
	brief.WriteString("  - title: \"Source title\"\n")
	brief.WriteString("    url: \"https://...\"\n")
	brief.WriteString("    type: \"article|paper|repository|documentation\"\n")
	brief.WriteString("    summary: \"Brief summary of key insights\"\n")
	brief.WriteString("    relevance: \"high|medium|low\"\n")
	brief.WriteString("```\n\n")

	brief.WriteString("## Research Scope\n")
	brief.WriteString("- Focus on authoritative sources (academic papers, official docs, reputable sites)\n")
	brief.WriteString("- Prioritize recent information (last 2-3 years when relevant)\n")
	brief.WriteString("- Include both technical and domain-specific perspectives\n")
	brief.WriteString("- Use web search to find current information\n")
	brief.WriteString("- Be thorough but concise in summaries\n")

	return brief.String()
}

// invokeAgent runs the user's AI agent with the research brief.
func (h *AgentHunter) invokeAgent(ctx context.Context, brief, projectPath string) (string, error) {
	// Build command with brief as argument
	args := append(h.agentArgs, brief)

	cmd := exec.CommandContext(ctx, h.agentCommand, args...)
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		// Try to get stderr for debugging
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("agent command failed: %w, stderr: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("agent command failed: %w", err)
	}

	return string(output), nil
}

// AgentSource represents a source found by the agent.
type AgentSource struct {
	Title       string `yaml:"title"`
	URL         string `yaml:"url"`
	Type        string `yaml:"type"`
	Summary     string `yaml:"summary"`
	Relevance   string `yaml:"relevance"`
	CollectedAt string `yaml:"collected_at,omitempty"`
}

// parseAgentOutput extracts structured sources from agent response.
func (h *AgentHunter) parseAgentOutput(output string) ([]AgentSource, error) {
	// Find YAML block in output
	var yamlContent string

	// Try to find YAML block between triple backticks
	if idx := strings.Index(output, "```yaml"); idx != -1 {
		start := idx + 7
		if endIdx := strings.Index(output[start:], "```"); endIdx != -1 {
			yamlContent = output[start : start+endIdx]
		}
	}

	// If no code block, try to find sources: directly
	if yamlContent == "" {
		if idx := strings.Index(output, "sources:"); idx != -1 {
			yamlContent = output[idx:]
			// Try to find the end of the YAML
			if endIdx := strings.Index(yamlContent, "\n\n"); endIdx != -1 {
				yamlContent = yamlContent[:endIdx]
			}
		}
	}

	if yamlContent == "" {
		// Try to parse the entire output as YAML
		yamlContent = output
	}

	// Parse YAML
	var result struct {
		Sources []AgentSource `yaml:"sources"`
	}
	if err := yamlsafe.Decode([]byte(yamlContent), &result); err != nil {
		// If YAML parsing fails, try line-by-line extraction
		return h.extractSourcesFromText(output)
	}

	// Add collected_at timestamp
	now := time.Now().Format("2006-01-02")
	for i := range result.Sources {
		if result.Sources[i].CollectedAt == "" {
			result.Sources[i].CollectedAt = now
		}
	}

	return result.Sources, nil
}

// extractSourcesFromText tries to extract sources from unstructured text.
func (h *AgentHunter) extractSourcesFromText(text string) ([]AgentSource, error) {
	var sources []AgentSource

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Look for URLs
		if strings.Contains(line, "http://") || strings.Contains(line, "https://") {
			// Extract URL
			words := strings.Fields(line)
			for _, word := range words {
				if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
					// Clean URL
					url := strings.TrimSuffix(strings.TrimSuffix(word, ","), ")")
					sources = append(sources, AgentSource{
						Title:       "Found URL",
						URL:         url,
						Type:        "url",
						Summary:     line,
						Relevance:   "medium",
						CollectedAt: time.Now().Format("2006-01-02"),
					})
					break
				}
			}
		}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no sources found in agent output")
	}

	return sources, nil
}

// saveResults saves the collected sources to a YAML file.
func (h *AgentHunter) saveResults(cfg HunterConfig, sources []AgentSource) (string, error) {
	outputDir := filepath.Join(cfg.ProjectPath, ".pollard", "sources", "agent")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s-agent-research.yaml", time.Now().Format("2006-01-02"))
	fullPath := filepath.Join(outputDir, filename)

	output := struct {
		CollectedAt time.Time     `yaml:"collected_at"`
		Query       string        `yaml:"query"`
		Sources     []AgentSource `yaml:"sources"`
	}{
		CollectedAt: time.Now().UTC(),
		Query:       strings.Join(cfg.Queries, ", "),
		Sources:     sources,
	}

	data, err := yaml.Marshal(&output)
	if err != nil {
		return "", err
	}

	return fullPath, os.WriteFile(fullPath, data, 0644)
}

// SetAgent configures the agent command and arguments.
func (h *AgentHunter) SetAgent(command string, args []string) {
	h.agentCommand = command
	h.agentArgs = args
}

// resolveShape picks a thinking shape based on query characteristics.
func (h *AgentHunter) resolveShape(cfg HunterConfig) thinking.Shape {
	for _, q := range cfg.Queries {
		lower := strings.ToLower(q)
		if strings.Contains(lower, "competitor") || strings.Contains(lower, "alternative") || strings.Contains(lower, "compare") {
			return thinking.ShapeContrapositive
		}
	}
	return thinking.ShapeAbductive
}

// shapePhaseName maps a research shape to a phase name for template lookup.
func (h *AgentHunter) shapePhaseName(shape thinking.Shape) string {
	switch shape {
	case thinking.ShapeContrapositive:
		return "Problem" // reuse contrapositive template
	case thinking.ShapeAbductive:
		return "Critical User Journeys" // reuse abductive template
	default:
		return ""
	}
}
