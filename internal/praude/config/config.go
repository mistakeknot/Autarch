package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/mistakeknot/vauxpraudemonium/pkg/agenttargets"
)

type Config struct {
	ValidationMode string                   `toml:"validation_mode"`
	Agents         map[string]AgentProfile `toml:"agents"`
}

type AgentProfile struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

const DefaultConfigToml = `# Praude configuration

validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
# args = ["--profile", "pm", "--prompt-file", "{brief}"]

[agents.claude]
command = "claude"
args = []
# args = ["--profile", "pm", "--prompt-file", "{brief}"]

[agents.opencode]
command = "opencode"
args = []

[agents.droid]
command = "droid"
args = []
`

func LoadFromRoot(root string) (Config, error) {
	path := filepath.Join(root, ".praude", "config.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if merged, err := loadSharedAgents(root); err == nil && len(merged) > 0 {
		cfg.Agents = merged
	}
	return cfg, nil
}

func loadSharedAgents(projectRoot string) (map[string]AgentProfile, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, nil
	}
	globalPath := filepath.Join(configDir, "vauxpraudemonium", "agents.toml")
	globalReg, projectReg, err := agenttargets.Load(globalPath, projectRoot)
	if err != nil {
		return nil, err
	}
	merged := agenttargets.Merge(globalReg, projectReg)
	if len(merged.Targets) == 0 {
		return nil, nil
	}
	out := make(map[string]AgentProfile, len(merged.Targets))
	for name, target := range merged.Targets {
		if target.Command == "" {
			continue
		}
		out[name] = AgentProfile{Command: target.Command, Args: target.Args}
	}
	return out, nil
}
