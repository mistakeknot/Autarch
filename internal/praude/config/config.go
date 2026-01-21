package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
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
	return cfg, nil
}
