package tmux

import (
	"errors"
	"testing"
)

// fakeRunnerPanes follows project convention (see client_actions_test.go)
type fakeRunnerPanes struct {
	stdout string
	stderr string
	err    error
}

func (f *fakeRunnerPanes) Run(name string, args ...string) (string, string, error) {
	return f.stdout, f.stderr, f.err
}

func TestGetAgentPanes_ParsesOutput(t *testing.T) {
	// Tab-delimited (matching RefreshCache pattern) -- safe for pane titles with colons
	runner := &fakeRunnerPanes{
		stdout: "%0\tclaude-agent\tdev\n%1\tcodex-agent\tdev\n%2\tgemini-agent\tdev\n%3\tuser-shell\tdev\n",
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(panes) != 4 {
		t.Fatalf("got %d panes, want 4", len(panes))
	}

	expected := []struct {
		id        string
		agentType AgentType
	}{
		{"%0", AgentClaude},
		{"%1", AgentCodex},
		{"%2", AgentGemini},
		{"%3", AgentUser},
	}
	for i, e := range expected {
		if panes[i].ID != e.id {
			t.Errorf("panes[%d].ID = %q, want %q", i, panes[i].ID, e.id)
		}
		if panes[i].AgentType != e.agentType {
			t.Errorf("panes[%d].AgentType = %q, want %q", i, panes[i].AgentType, e.agentType)
		}
	}
}

func TestGetAgentPanes_FiltersBySession(t *testing.T) {
	runner := &fakeRunnerPanes{
		stdout: "%0\tclaude-agent\tdev\n%1\tcodex-agent\tother\n%2\tgemini-agent\tdev\n",
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2 (filtered to 'dev' session)", len(panes))
	}
}

func TestGetAgentPanes_EmptyOnNoServer(t *testing.T) {
	runner := &fakeRunnerPanes{
		stderr: "no server running on /tmp/tmux-1000/default",
		err:    errors.New("exit status 1"),
	}
	client := NewClientWithRunner(runner)

	panes, err := client.GetAgentPanes("dev")
	if err != nil {
		t.Fatalf("expected nil error for no-server, got: %v", err)
	}
	if len(panes) != 0 {
		t.Errorf("expected empty panes, got %d", len(panes))
	}
}

func TestDetectAgentType(t *testing.T) {
	tests := []struct {
		title string
		want  AgentType
	}{
		{"claude-agent", AgentClaude},
		{"Claude Code", AgentClaude},
		{"codex-agent", AgentCodex},
		{"Codex CLI", AgentCodex},
		{"gemini-agent", AgentGemini},
		{"Gemini Pro", AgentGemini},
		{"user-shell", AgentUser},
		{"bash", AgentUser},
		{"zsh", AgentUser},
		{"something-else", AgentUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := detectAgentType(tt.title)
			if got != tt.want {
				t.Errorf("detectAgentType(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
