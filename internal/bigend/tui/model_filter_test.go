package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestFilterParsesStatusTokens(t *testing.T) {
	state := parseFilter("!waiting codex")
	if !state.Statuses[icdata.StatusWaiting] {
		t.Fatalf("expected waiting status")
	}
	if len(state.Terms) != 1 || state.Terms[0] != "codex" {
		t.Fatalf("expected codex term")
	}
}

func TestFilterParsesUnknownStatusToken(t *testing.T) {
	state := parseFilter("!unknown codex")
	if !state.Statuses[icdata.StatusUnknown] {
		t.Fatalf("expected unknown status")
	}
	if len(state.Terms) != 1 || state.Terms[0] != "codex" {
		t.Fatalf("expected codex term")
	}
}

func TestSessionFilterAppliesStatusAndText(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-a"}, Status: icdata.StatusWaiting},
		SessionItem{Session: aggregator.TmuxSession{Name: "codex-b"}, Status: icdata.StatusActive},
		SessionItem{Session: aggregator.TmuxSession{Name: "claude"}, Status: icdata.StatusWaiting},
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
	statusByAgent := map[string]icdata.UnifiedStatus{
		"Copper": icdata.StatusWaiting,
		"Rose":   icdata.StatusActive,
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

func TestAgentFilterMatchesUnknownStatus(t *testing.T) {
	items := []list.Item{
		AgentItem{Agent: aggregator.Agent{Name: "Copper"}},
		AgentItem{Agent: aggregator.Agent{Name: "Rose"}},
	}
	statusByAgent := map[string]icdata.UnifiedStatus{
		"Rose": icdata.StatusActive,
	}
	state := parseFilter("!unknown")
	filtered := filterAgentItems(items, state, statusByAgent)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 item, got %d", len(filtered))
	}
	got := filtered[0].(AgentItem)
	if got.Agent.Name != "Copper" {
		t.Fatalf("unexpected agent: %s", got.Agent.Name)
	}
}
