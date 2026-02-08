package hunters

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

// HunterCreator uses the AI agent to design new hunters for specific domains.
type HunterCreator struct {
	agentCommand string
	agentArgs    []string
}

// NewHunterCreator creates a creator using the configured agent.
func NewHunterCreator() *HunterCreator {
	cmd := os.Getenv("POLLARD_AGENT_COMMAND")
	if cmd == "" {
		cmd = "claude"
	}
	return &HunterCreator{
		agentCommand: cmd,
		agentArgs:    []string{"--print"},
	}
}

// CreateHunterForDomain asks the agent to design a hunter for a specific domain.
func (c *HunterCreator) CreateHunterForDomain(ctx context.Context, projectPath, domain, contextInfo string) (*CustomHunterSpec, error) {
	prompt := fmt.Sprintf(`Design a research hunter for the "%s" domain.

Context: %s

Your task is to identify a free, publicly accessible API that can be used to gather research data for this domain.

Requirements:
1. The API must be free (no paid subscription required)
2. The API must not require authentication (or use minimal auth like email)
3. The API should return structured data (JSON preferred)

Return a YAML configuration for a custom hunter:

If you find a suitable API:
`+"```yaml"+`
name: %s-hunter
description: "Hunter for %s domain"
api_endpoint: "https://..."
method: GET
headers:
  User-Agent: "Pollard Research Hunter"
query_param: "q"
results_path: "results"
mappings:
  title: "title"
  url: "url"
  description: "description"
  date: "date"
`+"```"+`

If no suitable free API exists:
`+"```yaml"+`
name: %s-hunter
no_api: true
recommendation: "Use agent-research with web search for this domain. The agent can search for [specific resources] and analyze the results."
`+"```"+`

Only output the YAML block, nothing else.
`, domain, contextInfo, domain, domain, domain)

	// Invoke agent
	cmd := exec.CommandContext(ctx, c.agentCommand, append(c.agentArgs, prompt)...)
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("agent failed: %w, stderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("agent failed: %w", err)
	}

	// Parse response
	spec, err := c.parseSpecFromOutput(string(output))
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	// Save spec if valid
	if spec.APIEndpoint != "" || spec.NoAPI {
		specPath := filepath.Join(projectPath, ".pollard", "hunters", "custom", spec.Name+".yaml")
		if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
			return nil, fmt.Errorf("create custom hunters dir: %w", err)
		}
		data, _ := yaml.Marshal(&spec)
		if err := os.WriteFile(specPath, data, 0644); err != nil {
			return nil, fmt.Errorf("write spec: %w", err)
		}
	}

	return spec, nil
}

// parseSpecFromOutput extracts the YAML spec from agent output.
func (c *HunterCreator) parseSpecFromOutput(output string) (*CustomHunterSpec, error) {
	// Try to find YAML block between triple backticks
	var yamlContent string

	if idx := strings.Index(output, "```yaml"); idx != -1 {
		start := idx + 7
		if endIdx := strings.Index(output[start:], "```"); endIdx != -1 {
			yamlContent = output[start : start+endIdx]
		}
	}

	// If no code block, try parsing the entire output
	if yamlContent == "" {
		if strings.Contains(output, "name:") {
			yamlContent = output
		}
	}

	if yamlContent == "" {
		return nil, fmt.Errorf("no YAML spec found in output")
	}

	var spec CustomHunterSpec
	if err := yamlsafe.Decode([]byte(yamlContent), &spec); err != nil {
		return nil, err
	}

	// Validate required fields
	if spec.Name == "" {
		return nil, fmt.Errorf("spec missing name field")
	}

	return &spec, nil
}

// SuggestHunterForQuery analyzes a query and suggests whether a custom hunter would help.
func (c *HunterCreator) SuggestHunterForQuery(query string) (string, bool) {
	queryLower := strings.ToLower(query)

	// Domain patterns that might benefit from custom hunters
	patterns := map[string][]string{
		"recipe-hunter":         {"recipe", "cooking", "food prep", "ingredients for"},
		"weather-hunter":        {"weather", "forecast", "temperature", "precipitation"},
		"sports-hunter":         {"sports", "game score", "team", "league", "player stats"},
		"real-estate-hunter":    {"property", "housing", "real estate", "listing", "rental"},
		"job-hunter":            {"job opening", "career", "hiring", "employment"},
		"product-hunter":        {"product review", "price comparison", "shopping"},
		"travel-hunter":         {"flights", "hotels", "travel", "vacation", "booking"},
		"cryptocurrency-hunter": {"crypto", "bitcoin", "ethereum", "blockchain"},
	}

	for hunter, keywords := range patterns {
		matchCount := 0
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				matchCount++
			}
		}
		if matchCount >= 2 || (matchCount >= 1 && len(keywords) <= 3) {
			return hunter, true
		}
	}

	return "", false
}

// ListCustomHunters returns the names of all custom hunters in the project.
func ListCustomHunters(projectPath string) ([]string, error) {
	customDir := filepath.Join(projectPath, ".pollard", "hunters", "custom")
	if _, err := os.Stat(customDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(customDir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		names = append(names, name)
	}

	return names, nil
}

// GetCustomHunterSpec loads a custom hunter spec by name.
func GetCustomHunterSpec(projectPath, name string) (*CustomHunterSpec, error) {
	specPath := filepath.Join(projectPath, ".pollard", "hunters", "custom", name+".yaml")
	var spec CustomHunterSpec
	if _, err := yamlsafe.UnmarshalFile(specPath, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// DeleteCustomHunter removes a custom hunter spec.
func DeleteCustomHunter(projectPath, name string) error {
	specPath := filepath.Join(projectPath, ".pollard", "hunters", "custom", name+".yaml")
	return os.Remove(specPath)
}
