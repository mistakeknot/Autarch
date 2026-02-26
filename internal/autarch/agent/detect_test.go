package agent

import (
	"os/exec"
	"testing"
)

func TestDetectAgentByNamePrefersName(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex CLI not available, skipping")
	}
	agent, err := DetectAgentByName("codex", func(name string) (string, error) { return "/bin/codex", nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Type != TypeCodex {
		t.Fatalf("expected codex type, got %v", agent.Type)
	}
}
