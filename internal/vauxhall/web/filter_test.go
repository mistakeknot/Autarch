package web

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

func TestParseFilterStatusTokens(t *testing.T) {
	state := parseFilter("!waiting codex")
	if !state.Statuses[tmux.StatusWaiting] {
		t.Fatalf("expected waiting status")
	}
	if len(state.Terms) != 1 || state.Terms[0] != "codex" {
		t.Fatalf("expected codex term")
	}
}

func TestFilterSessionsAppliesStatusAndText(t *testing.T) {
	sessions := []aggregator.TmuxSession{
		{Name: "codex-a", AgentType: "codex"},
		{Name: "codex-b", AgentType: "codex"},
		{Name: "claude"},
	}
	statusBySession := map[string]tmux.Status{
		"codex-a": tmux.StatusWaiting,
		"codex-b": tmux.StatusRunning,
		"claude":  tmux.StatusWaiting,
	}
	state := parseFilter("!waiting codex")
	filtered := filterSessions(sessions, state, statusBySession)
	if len(filtered) != 1 || filtered[0].Name != "codex-a" {
		t.Fatalf("unexpected filter result")
	}
}

func TestFilterAgentsMatchesUnknownStatus(t *testing.T) {
	agents := []aggregator.Agent{
		{Name: "Copper"},
		{Name: "Rose", SessionName: "rose"},
	}
	statusBySession := map[string]tmux.Status{
		"rose": tmux.StatusRunning,
	}
	state := parseFilter("!unknown")
	filtered := filterAgents(agents, state, statusBySession)
	if len(filtered) != 1 || filtered[0].Name != "Copper" {
		t.Fatalf("expected unknown agent")
	}
}
