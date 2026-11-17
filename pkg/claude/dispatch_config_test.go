package claude

import "testing"

func TestConfigFromArgsDispatchAgentOverride(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	cfg, _ := configFromArgs([]string{"-p", "hello"})
	if cfg.PreferredAgent != "codex" {
		t.Fatalf("dispatch picked %q, want codex", cfg.PreferredAgent)
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
