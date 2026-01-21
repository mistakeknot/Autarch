package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type GeneralConfig struct {
	MaxAgents int `toml:"max_agents"`
}

type TUIConfig struct {
	ConfirmApprove bool `toml:"confirm_approve"`
}

type ReviewConfig struct {
	TargetBranch string `toml:"target_branch"`
}

type CodingAgentConfig struct {
	HealthCheckInterval int  `toml:"health_check_interval"`
	RestartOnFailure    bool `toml:"restart_on_failure"`
}

type LLMSummaryConfig struct {
	Command        string `toml:"command"`
	TimeoutSeconds int    `toml:"timeout_seconds"`
}

type Config struct {
	General    GeneralConfig     `toml:"general"`
	TUI        TUIConfig         `toml:"tui"`
	Review     ReviewConfig      `toml:"review"`
	Coding     CodingAgentConfig `toml:"coding_agent"`
	LLMSummary LLMSummaryConfig  `toml:"llm_summary"`
}

func defaultConfig() Config {
	return Config{
		General:    GeneralConfig{MaxAgents: 4},
		TUI:        TUIConfig{ConfirmApprove: true},
		Review:     ReviewConfig{TargetBranch: ""},
		Coding:     CodingAgentConfig{HealthCheckInterval: 0, RestartOnFailure: false},
		LLMSummary: LLMSummaryConfig{Command: "", TimeoutSeconds: 0},
	}
}

func LoadFromProject(projectDir string) (Config, error) {
	cfg := defaultConfig()
	path := filepath.Join(projectDir, ".tandemonium", "config.toml")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
