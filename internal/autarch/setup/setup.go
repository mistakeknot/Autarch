// Package setup handles Autarch first-run configuration.
//
// This includes:
// - Creating ~/.autarch/ directory structure
// - Installing agent state hooks for Claude Code and Codex CLI
// - Verifying required dependencies (tmux, etc.)
package setup

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed hooks/*
var hookFiles embed.FS

// Status represents the current setup state.
type Status struct {
	DataDirExists    bool
	HooksInstalled   bool
	ClaudeConfigured bool
	CodexConfigured  bool
	TmuxAvailable    bool
}

// Check returns the current setup status.
func Check() Status {
	home, _ := os.UserHomeDir()
	autarchDir := filepath.Join(home, ".autarch")
	hooksDir := filepath.Join(autarchDir, "hooks")

	status := Status{
		DataDirExists:  dirExists(autarchDir),
		HooksInstalled: fileExists(filepath.Join(hooksDir, "emit-state.sh")),
	}

	// Check Claude Code configuration
	claudeSettings := filepath.Join(home, ".claude", "settings.json")
	if data, err := os.ReadFile(claudeSettings); err == nil {
		status.ClaudeConfigured = strings.Contains(string(data), "emit-state.sh")
	}

	// Check Codex configuration
	codexConfig := filepath.Join(home, ".codex", "config.toml")
	if data, err := os.ReadFile(codexConfig); err == nil {
		status.CodexConfigured = strings.Contains(string(data), "codex-notify.sh")
	}

	// Check tmux availability
	_, err := exec.LookPath("tmux")
	status.TmuxAvailable = err == nil

	return status
}

// NeedsSetup returns true if any setup step is missing.
func NeedsSetup() bool {
	status := Check()
	return !status.DataDirExists || !status.HooksInstalled
}

// Run performs the full Autarch setup.
func Run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	autarchDir := filepath.Join(home, ".autarch")
	hooksDir := filepath.Join(autarchDir, "hooks")
	statesDir := filepath.Join(autarchDir, "agent-states")

	// Create directories
	dirs := []string{autarchDir, hooksDir, statesDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// Install hook scripts from embedded files
	if err := installHookScripts(hooksDir); err != nil {
		return fmt.Errorf("failed to install hook scripts: %w", err)
	}

	// Configure Claude Code (if settings.json exists or can be created)
	if err := configureClaudeCode(home, hooksDir); err != nil {
		// Non-fatal - Claude Code might not be installed
		fmt.Fprintf(os.Stderr, "Note: Could not configure Claude Code hooks: %v\n", err)
	}

	// Configure Codex CLI
	if err := configureCodexCLI(home, hooksDir); err != nil {
		// Non-fatal - Codex might not be installed
		fmt.Fprintf(os.Stderr, "Note: Could not configure Codex CLI hooks: %v\n", err)
	}

	// Install archviz post-commit hook in the repo's .git/hooks
	if err := installArchvizHook(); err != nil {
		fmt.Fprintf(os.Stderr, "Note: Could not install archviz post-commit hook: %v\n", err)
	}

	return nil
}

// installHookScripts copies embedded hook scripts to the hooks directory.
func installHookScripts(hooksDir string) error {
	scripts := []string{"emit-state.sh", "codex-notify.sh"}

	for _, script := range scripts {
		data, err := hookFiles.ReadFile("hooks/" + script)
		if err != nil {
			return fmt.Errorf("failed to read embedded %s: %w", script, err)
		}

		destPath := filepath.Join(hooksDir, script)
		if err := os.WriteFile(destPath, data, 0755); err != nil {
			return fmt.Errorf("failed to write %s: %w", script, err)
		}
	}

	return nil
}

// configureClaudeCode adds hooks to Claude Code settings.
func configureClaudeCode(home, hooksDir string) error {
	claudeDir := filepath.Join(home, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Read existing settings or start fresh
	var settings map[string]interface{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			settings = make(map[string]interface{})
		}
	} else {
		// Create .claude directory if needed
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			return err
		}
		settings = make(map[string]interface{})
	}

	// Check if hooks already configured
	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if _, hasSession := hooks["SessionStart"]; hasSession {
			return nil // Already configured
		}
	}

	// Add our hooks
	emitScript := filepath.Join(hooksDir, "emit-state.sh")
	settings["hooks"] = buildClaudeHooks(emitScript)

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// configureCodexCLI adds notify setting to Codex config.
func configureCodexCLI(home, hooksDir string) error {
	codexDir := filepath.Join(home, ".codex")
	configPath := filepath.Join(codexDir, "config.toml")

	notifyScript := filepath.Join(hooksDir, "codex-notify.sh")

	// Read existing config
	var content string
	if data, err := os.ReadFile(configPath); err == nil {
		content = string(data)
		// Check if already configured
		if strings.Contains(content, "notify") {
			return nil // Already configured
		}
	} else {
		// Create .codex directory if needed
		if err := os.MkdirAll(codexDir, 0755); err != nil {
			return err
		}
		content = "# Codex CLI configuration\n"
	}

	// Append notify setting
	content += fmt.Sprintf("\n# Autarch agent state hooks\nnotify = %q\n", notifyScript)

	return os.WriteFile(configPath, []byte(content), 0644)
}

// buildClaudeHooks creates the hooks configuration for Claude Code.
func buildClaudeHooks(emitScript string) map[string]interface{} {
	makeHook := func(state string) []interface{} {
		return []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": fmt.Sprintf("%s %s claude", emitScript, state),
						"timeout": 5,
					},
				},
			},
		}
	}

	makeMatcherHook := func(state string) []interface{} {
		return []interface{}{
			map[string]interface{}{
				"matcher": ".*",
				"hooks": []interface{}{
					map[string]interface{}{
						"type":    "command",
						"command": fmt.Sprintf("%s %s claude", emitScript, state),
						"timeout": 5,
					},
				},
			},
		}
	}

	return map[string]interface{}{
		"SessionStart":       makeHook("working"),
		"UserPromptSubmit":   makeHook("working"),
		"PreToolUse":         makeMatcherHook("executing_tool"),
		"PermissionRequest":  makeMatcherHook("blocked"),
		"PostToolUse":        makeMatcherHook("working"),
		"PostToolUseFailure": makeMatcherHook("error"),
		"Stop":               makeHook("waiting"),
		"SessionEnd":         makeHook("done"),
	}
}

// installArchvizHook appends the archviz post-commit hook to the repo's
// .git/hooks/post-commit if not already present.
func installArchvizHook() error {
	// Find repo root by walking up from cwd
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("not inside a git repo")
		}
		dir = parent
	}

	gitHooksDir := filepath.Join(dir, ".git", "hooks")
	if !dirExists(gitHooksDir) {
		if err := os.MkdirAll(gitHooksDir, 0755); err != nil {
			return err
		}
	}

	hookPath := filepath.Join(gitHooksDir, "post-commit")
	marker := "scripts/update-archviz.sh"

	// Check if already installed
	if data, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(data), marker) {
			return nil
		}
	}

	// Append to existing or create new
	f, err := os.OpenFile(hookPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		fmt.Fprintln(f, "#!/bin/sh")
	}
	fmt.Fprintf(f, "\n# Archviz: regenerate architecture visualizer on relevant changes\n")
	fmt.Fprintf(f, "\"$(git rev-parse --show-toplevel)/%s\"\n", marker)

	return nil
}

// Helper functions

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
