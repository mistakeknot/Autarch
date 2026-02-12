package agenttargets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectInstructions_NewFile(t *testing.T) {
	dir := t.TempDir()
	err := InjectInstructions(dir, "claude", "# Task Context\nYou are validating spec PRD-001.")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, markerBegin) {
		t.Error("missing BEGIN marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("missing END marker")
	}
	if !strings.Contains(content, "PRD-001") {
		t.Error("missing injected content")
	}
}

func TestInjectInstructions_ExistingFileNoMarkers(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("# Existing Content\n\nSome rules.\n"), 0644)

	err := InjectInstructions(dir, "claude", "# Autarch Task")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	content := string(data)

	// Should have both original and injected.
	if !strings.Contains(content, "Existing Content") {
		t.Error("original content lost")
	}
	if !strings.Contains(content, "# Autarch Task") {
		t.Error("injected content missing")
	}
	if !strings.Contains(content, markerBegin) {
		t.Error("BEGIN marker missing")
	}
}

func TestInjectInstructions_Idempotent(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("# Base\n"), 0644)

	// Inject twice with different content — second should replace first.
	InjectInstructions(dir, "claude", "version 1")
	InjectInstructions(dir, "claude", "version 2")

	data, _ := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	content := string(data)

	if strings.Contains(content, "version 1") {
		t.Error("first injection not replaced")
	}
	if !strings.Contains(content, "version 2") {
		t.Error("second injection missing")
	}
	// Should have exactly one BEGIN marker.
	if strings.Count(content, markerBegin) != 1 {
		t.Errorf("expected 1 BEGIN marker, got %d", strings.Count(content, markerBegin))
	}
}

func TestInjectInstructions_CodexTarget(t *testing.T) {
	dir := t.TempDir()
	err := InjectInstructions(dir, "codex", "# Codex Instructions")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Codex Instructions") {
		t.Error("codex injection missing")
	}
}

func TestInjectInstructions_UnknownAgent(t *testing.T) {
	dir := t.TempDir()
	err := InjectInstructions(dir, "gemini", "# Gemini Instructions")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestRemoveInjectedInstructions(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("# Base\n"), 0644)

	InjectInstructions(dir, "claude", "temporary context")

	// Verify it's there.
	data, _ := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if !strings.Contains(string(data), "temporary context") {
		t.Fatal("injection failed")
	}

	// Remove it.
	err := RemoveInjectedInstructions(dir, "claude")
	if err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	content := string(data)
	if strings.Contains(content, "temporary context") {
		t.Error("injected content not removed")
	}
	if strings.Contains(content, markerBegin) {
		t.Error("BEGIN marker not removed")
	}
	if !strings.Contains(content, "# Base") {
		t.Error("original content lost during removal")
	}
}

func TestRemoveInjectedInstructions_NoFile(t *testing.T) {
	dir := t.TempDir()
	// Should not error when file doesn't exist.
	err := RemoveInjectedInstructions(dir, "claude")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemoveInjectedInstructions_NoMarkers(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("# Clean File\n"), 0644)

	err := RemoveInjectedInstructions(dir, "claude")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	if string(data) != "# Clean File\n" {
		t.Error("file modified when no markers present")
	}
}

func TestSupportsSystemPromptFlag(t *testing.T) {
	tests := []struct {
		agent string
		want  bool
	}{
		{"claude", true},
		{"codex", false},
		{"gemini", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := SupportsSystemPromptFlag(tt.agent); got != tt.want {
			t.Errorf("SupportsSystemPromptFlag(%q) = %v, want %v", tt.agent, got, tt.want)
		}
	}
}

func TestBuildSystemPromptArgs(t *testing.T) {
	args := BuildSystemPromptArgs("claude", "You are reviewing spec PRD-001.")
	if len(args) != 2 || args[0] != "--append-system-prompt" {
		t.Errorf("unexpected args: %v", args)
	}

	// Empty instructions → nil
	args = BuildSystemPromptArgs("claude", "")
	if args != nil {
		t.Error("expected nil for empty instructions")
	}

	// Codex doesn't support it → nil
	args = BuildSystemPromptArgs("codex", "some instructions")
	if args != nil {
		t.Error("expected nil for codex")
	}
}

func TestReplaceMarkerBlock_ContentPreservation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		block   string
		checks  []string // substrings that must be present
		absent  []string // substrings that must not be present
	}{
		{
			name:    "append to empty file",
			content: "",
			block:   markerBegin + "\ntest\n" + markerEnd,
			checks:  []string{markerBegin, "test", markerEnd},
		},
		{
			name:    "append to content",
			content: "# Header\n\nBody text.\n",
			block:   markerBegin + "\ninjected\n" + markerEnd,
			checks:  []string{"# Header", "Body text.", "injected"},
		},
		{
			name:    "replace existing block",
			content: "# Header\n\n" + markerBegin + "\nold\n" + markerEnd + "\n\n# Footer\n",
			block:   markerBegin + "\nnew\n" + markerEnd,
			checks:  []string{"# Header", "new", "# Footer"},
			absent:  []string{"old"},
		},
		{
			name:    "remove block",
			content: "# Header\n\n" + markerBegin + "\nold\n" + markerEnd + "\n\n# Footer\n",
			block:   "",
			checks:  []string{"# Header", "# Footer"},
			absent:  []string{markerBegin, "old"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceMarkerBlock(tt.content, tt.block)
			for _, s := range tt.checks {
				if !strings.Contains(result, s) {
					t.Errorf("result missing %q\nresult: %q", s, result)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(result, s) {
					t.Errorf("result should not contain %q\nresult: %q", s, result)
				}
			}
		})
	}
}
