package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tmux"
)

func TestFilterParsesStatusTokens(t *testing.T) {
	state := parseFilter("!waiting codex")
	if !state.Statuses[tmux.StatusWaiting] {
		t.Fatalf("expected waiting status")
	}
	if len(state.Terms) != 1 || state.Terms[0] != "codex" {
		t.Fatalf("expected codex term")
	}
}

func TestSessionFilterAppliesStatusAndText(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-a"}, Status: tmux.StatusWaiting},
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-b"}, Status: tmux.StatusRunning},
		SessionItem{Session: aggregator.TmuxSession{Name: "claude"}, Status: tmux.StatusWaiting},
	}
	state := parseFilter("!waiting codex")
	filtered := filterSessionItems(items, state)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item, got %d", len(filtered))
	}
	got := filtered[0].(SessionItem)
	if got.Session.Name != "codex-a" {
		t.Fatalf("unexpected session: %s", got.Session.Name)
	}
}

func TestAgentFilterUsesLinkedSessionStatus(t *testing.T) {
	items := []list.Item{
		AgentItem{Agent: aggregator.Agent{Name: "Copper"}},
		AgentItem{Agent: aggregator.Agent{Name: "Rose"}},
	}
	statusByAgent := map[string]tmux.Status{
		"Copper": tmux.StatusWaiting,
		"Rose":   tmux.StatusRunning,
	}
	state := parseFilter("!waiting")
	filtered := filterAgentItems(items, state, statusByAgent)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item, got %d", len(filtered))
	}
	got := filtered[0].(AgentItem)
	if got.Agent.Name != "Copper" {
		t.Fatalf("unexpected agent: %s", got.Agent.Name)
	}
}
