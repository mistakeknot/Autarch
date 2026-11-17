package claude

import (
	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"os"
	"testing"
)

func TestConfigFromArgsDispatchAgentOverride(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	cfg, _ := configFromArgs([]string{"-p", "hello"})
	if cfg.PreferredAgent != "claude" {
		t.Fatalf("dispatch picked %q, want claude", cfg.PreferredAgent)
	}
}

func TestConfigFromArgsDefaults(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	cfg, prompt := configFromArgs([]string{"-p", "hello"})
	if cfg.PreferredAgent != "claude" || cfg.Timeout != 0 || cfg.Verbose || cfg.Print || cfg.Sandbox != "" || prompt != "hello" {
		t.Fatalf("legacy defaults changed: %+v", cfg)
	}
}

func TestChatIgnoresProjectPreferencesAndPreservesExplicitArguments(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".gurgeh", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gurgeh/agents.toml", []byte("[dispatch]\npreferred_agent = 'codex'\npreferred_backend = 'api'\ntimeout = '1s'\nmodel = 'project-model'\nsession_id = 'project-session'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	cfg, _ := configFromArgs([]string{"--model", "explicit-model", "--resume", "claude-session", "--system-prompt", "system", "-c", "-p", "hello"})
	if cfg.PreferredAgent != "claude" || cfg.PreferredBackend != agenttargets.BackendSubscriptionCLI || cfg.Timeout != 0 || cfg.Model != "explicit-model" || cfg.SessionID != "claude-session" {
		t.Fatalf("chat config changed: %+v", cfg)
	}
}
