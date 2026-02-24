package agenttargets

import (
	"testing"
)

func TestBuildCodexArgs_Defaults(t *testing.T) {
	cfg := DefaultDispatchConfig()
	args := buildCodexArgs(cfg, "hello", "/tmp/out.md")
	assertContains(t, args, "exec")
	assertContains(t, args, "-s")
	assertContains(t, args, "workspace-write")
	assertContains(t, args, "-o")
	assertContains(t, args, "/tmp/out.md")
	assertContains(t, args, "hello")
}

func TestBuildCodexArgs_CustomSandbox(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.Sandbox = "read-only"
	args := buildCodexArgs(cfg, "test", "/tmp/out.md")
	assertContains(t, args, "read-only")
}

func TestBuildCodexArgs_WithModel(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.Model = "gpt-5.3-codex"
	args := buildCodexArgs(cfg, "test", "/tmp/out.md")
	assertContains(t, args, "--model")
	assertContains(t, args, "gpt-5.3-codex")
}

func TestBuildCodexArgs_ExtraArgs(t *testing.T) {
	cfg := DefaultDispatchConfig()
	cfg.ExtraArgs = []string{"--add-dir", "/extra"}
	args := buildCodexArgs(cfg, "test", "/tmp/out.md")
	assertContains(t, args, "--add-dir")
	assertContains(t, args, "/extra")
}

func TestBuildCodexArgs_DefaultSandboxWhenEmpty(t *testing.T) {
	cfg := DispatchConfig{} // No sandbox set
	args := buildCodexArgs(cfg, "test", "/tmp/out.md")
	// Should default to workspace-write
	assertContains(t, args, "workspace-write")
}

func TestBuildCodexArgs_PromptIsLastArg(t *testing.T) {
	cfg := DefaultDispatchConfig()
	args := buildCodexArgs(cfg, "do something", "/tmp/out.md")
	last := args[len(args)-1]
	if last != "do something" {
		t.Errorf("last arg = %q, want %q", last, "do something")
	}
}
