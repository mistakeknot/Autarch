package tui

import (
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
)

var launchAgent = agents.Launch
var launchSubagent = agents.LaunchSubagent

func agentProfiles(cfg config.Config) map[string]agents.Profile {
	out := make(map[string]agents.Profile)
	for name, profile := range cfg.Agents {
		out[name] = agents.Profile{Command: profile.Command, Args: profile.Args}
	}
	return out
}

func defaultAgentName(cfg config.Config) string {
	if _, ok := cfg.Agents["codex"]; ok {
		return "codex"
	}
	for name := range cfg.Agents {
		return name
	}
	return "codex"
}

func isClaudeProfile(name string, profile agents.Profile) bool {
	if strings.EqualFold(name, "claude") {
		return true
	}
	return strings.Contains(strings.ToLower(profile.Command), "claude")
}
