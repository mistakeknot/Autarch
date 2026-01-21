package agents

import (
	"strings"
	"testing"
)

func TestResolveProfileMissing(t *testing.T) {
	cfg := map[string]Profile{}
	if _, err := Resolve(cfg, "codex"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildCommandSetsEnvForSubagent(t *testing.T) {
	p := Profile{Command: "echo", Args: []string{"hello"}}
	cmd := buildCommand(p, "brief.md", []string{"PRAUDE_SUBAGENT=1"})
	if len(cmd.Env) == 0 {
		t.Fatalf("expected env set")
	}
	found := false
	for _, entry := range cmd.Env {
		if entry == "PRAUDE_SUBAGENT=1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected PRAUDE_SUBAGENT in env")
	}
	if got := strings.Join(cmd.Args, " "); !strings.Contains(got, "brief.md") {
		t.Fatalf("expected brief path in args: %s", got)
	}
}
