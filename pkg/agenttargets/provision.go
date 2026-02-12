package agenttargets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	markerBegin = "<!-- AUTARCH:BEGIN -->"
	markerEnd   = "<!-- AUTARCH:END -->"
)

// ProvisionTarget describes which agent tool's config file to inject into.
type ProvisionTarget struct {
	// RelPath is the file path relative to the work directory.
	// e.g., ".claude/CLAUDE.md" or ".codex/AGENTS.md"
	RelPath string
	// Agent is "claude" or "codex".
	Agent string
}

// KnownProvisionTargets returns the standard provision targets for each agent.
func KnownProvisionTargets() []ProvisionTarget {
	return []ProvisionTarget{
		{RelPath: ".claude/CLAUDE.md", Agent: "claude"},
		{RelPath: ".codex/AGENTS.md", Agent: "codex"},
	}
}

// ProvisionTargetForAgent returns the provision target for a specific agent.
// Returns the target and true if found, zero value and false otherwise.
func ProvisionTargetForAgent(agent string) (ProvisionTarget, bool) {
	for _, pt := range KnownProvisionTargets() {
		if pt.Agent == agent {
			return pt, true
		}
	}
	return ProvisionTarget{}, false
}

// InjectInstructions writes instructions into the appropriate config file
// for the given agent, using idempotent AUTARCH:BEGIN/END markers.
// If the file already has an AUTARCH block, it is replaced. If the file
// doesn't exist, it is created (along with parent directories).
// The instructions string should be the raw content to inject (no markers needed).
func InjectInstructions(workDir, agent, instructions string) error {
	target, ok := ProvisionTargetForAgent(agent)
	if !ok {
		return fmt.Errorf("unknown agent %q for instruction injection", agent)
	}

	filePath := filepath.Join(workDir, target.RelPath)
	return injectIntoFile(filePath, instructions)
}

// InjectInstructionsToFile writes instructions into a specific file path
// using idempotent AUTARCH:BEGIN/END markers.
func InjectInstructionsToFile(filePath, instructions string) error {
	return injectIntoFile(filePath, instructions)
}

// RemoveInjectedInstructions removes the AUTARCH:BEGIN/END block from
// the agent's config file if present. Returns nil if file doesn't exist.
func RemoveInjectedInstructions(workDir, agent string) error {
	target, ok := ProvisionTargetForAgent(agent)
	if !ok {
		return fmt.Errorf("unknown agent %q for instruction removal", agent)
	}

	filePath := filepath.Join(workDir, target.RelPath)
	return removeFromFile(filePath)
}

// SupportsSystemPromptFlag reports whether the given agent CLI supports
// --append-system-prompt for inline instruction injection.
func SupportsSystemPromptFlag(agent string) bool {
	return agent == "claude"
}

// BuildSystemPromptArgs returns CLI args for inline instruction injection.
// Returns nil if the agent doesn't support it or instructions are empty.
func BuildSystemPromptArgs(agent, instructions string) []string {
	if instructions == "" || !SupportsSystemPromptFlag(agent) {
		return nil
	}
	return []string{"--append-system-prompt", instructions}
}

func injectIntoFile(filePath, instructions string) error {
	block := markerBegin + "\n" + instructions + "\n" + markerEnd

	// Ensure parent directory exists.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	existing, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// New file — just write the block.
			return os.WriteFile(filePath, []byte(block+"\n"), 0644)
		}
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	content := string(existing)
	replaced := replaceMarkerBlock(content, block)
	return os.WriteFile(filePath, []byte(replaced), 0644)
}

func removeFromFile(filePath string) error {
	existing, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	content := string(existing)
	if !strings.Contains(content, markerBegin) {
		return nil
	}

	cleaned := replaceMarkerBlock(content, "")
	return os.WriteFile(filePath, []byte(cleaned), 0644)
}

// replaceMarkerBlock replaces or appends the AUTARCH marker block in content.
// If replacement is empty, the block is removed entirely.
func replaceMarkerBlock(content, replacement string) string {
	beginIdx := strings.Index(content, markerBegin)
	if beginIdx == -1 {
		// No existing block — append.
		if replacement == "" {
			return content
		}
		trimmed := strings.TrimRight(content, "\n")
		if trimmed == "" {
			return replacement + "\n"
		}
		return trimmed + "\n\n" + replacement + "\n"
	}

	endIdx := strings.Index(content[beginIdx:], markerEnd)
	if endIdx == -1 {
		// Malformed — BEGIN without END. Replace from BEGIN to end of file.
		before := content[:beginIdx]
		if replacement == "" {
			return strings.TrimRight(before, "\n") + "\n"
		}
		return strings.TrimRight(before, "\n") + "\n\n" + replacement + "\n"
	}

	// Full block found. Replace it.
	blockEnd := beginIdx + endIdx + len(markerEnd)

	// Consume trailing newline if present.
	if blockEnd < len(content) && content[blockEnd] == '\n' {
		blockEnd++
	}

	before := content[:beginIdx]
	after := content[blockEnd:]

	if replacement == "" {
		// Remove — trim extra blank lines between before and after.
		result := strings.TrimRight(before, "\n")
		after = strings.TrimLeft(after, "\n")
		if result == "" {
			return after
		}
		if after == "" {
			return result + "\n"
		}
		return result + "\n\n" + after
	}

	before = strings.TrimRight(before, "\n")
	after = strings.TrimLeft(after, "\n")
	if before == "" {
		if after == "" {
			return replacement + "\n"
		}
		return replacement + "\n\n" + after
	}
	if after == "" {
		return before + "\n\n" + replacement + "\n"
	}
	return before + "\n\n" + replacement + "\n\n" + after
}
