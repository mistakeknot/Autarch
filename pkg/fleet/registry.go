package fleet

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AgentSpec describes an agent's capabilities, runtime, and cost profile
// as declared in fleet-registry.yaml.
type AgentSpec struct {
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	Category        string   `json:"category"`
	Description     string   `json:"description"`
	Capabilities    []string `json:"capabilities"`
	Roles           []string `json:"roles"`
	Runtime         Runtime  `json:"runtime"`
	Models          Models   `json:"models"`
	Tools           []string `json:"tools"`
	Tags            []string `json:"tags"`
	ColdStartTokens int      `json:"cold_start_tokens,omitempty"`
}

// Runtime describes how an agent executes.
type Runtime struct {
	Mode         string `yaml:"mode" json:"mode"`                   // cli, subagent, daemon
	SubagentType string `yaml:"subagent_type" json:"subagent_type"` // for subagent mode
	Binary       string `yaml:"binary" json:"binary"`               // for cli mode
}

// Models describes preferred and supported models.
type Models struct {
	Preferred string   `yaml:"preferred" json:"preferred"`
	Supported []string `yaml:"supported" json:"supported"`
}

// registryFile is the top-level structure of fleet-registry.yaml.
type registryFile struct {
	Version              string                       `yaml:"version"`
	CapabilityVocabulary []string                     `yaml:"capability_vocabulary"`
	Agents               map[string]registryAgentYAML `yaml:"agents"`
}

// registryAgentYAML maps the YAML structure of a single agent entry.
type registryAgentYAML struct {
	Source          string   `yaml:"source"`
	Category        string   `yaml:"category"`
	Description     string   `yaml:"description"`
	Capabilities    []string `yaml:"capabilities"`
	Roles           []string `yaml:"roles"`
	Runtime         Runtime  `yaml:"runtime"`
	Models          Models   `yaml:"models"`
	Tools           []string `yaml:"tools"`
	ColdStartTokens int      `yaml:"cold_start_tokens"`
	Tags            []string `yaml:"tags"`
}

// LoadRegistry parses fleet-registry.yaml and returns all agent specs.
func LoadRegistry(path string) ([]AgentSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var reg registryFile
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry %s: %w", path, err)
	}

	specs := make([]AgentSpec, 0, len(reg.Agents))
	for name, agent := range reg.Agents {
		specs = append(specs, AgentSpec{
			Name:            name,
			Source:          agent.Source,
			Category:        agent.Category,
			Description:     agent.Description,
			Capabilities:    agent.Capabilities,
			Roles:           agent.Roles,
			Runtime:         agent.Runtime,
			Models:          agent.Models,
			Tools:           agent.Tools,
			Tags:            agent.Tags,
			ColdStartTokens: agent.ColdStartTokens,
		})
	}

	return specs, nil
}

// LoadRegistryOrEmpty loads the registry, returning an empty slice on error.
func LoadRegistryOrEmpty(path string) []AgentSpec {
	specs, err := LoadRegistry(path)
	if err != nil {
		return nil
	}
	return specs
}

// FilterByRuntime returns agents matching a given runtime mode.
func FilterByRuntime(specs []AgentSpec, mode string) []AgentSpec {
	var filtered []AgentSpec
	for _, s := range specs {
		if s.Runtime.Mode == mode {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// FindByName looks up an agent by name.
func FindByName(specs []AgentSpec, name string) (AgentSpec, bool) {
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return AgentSpec{}, false
}
