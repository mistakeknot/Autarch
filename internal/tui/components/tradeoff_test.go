package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/pollard/research"
)

func TestTradeoffNavDownSelectsFirst(t *testing.T) {
	tradeoff := NewTradeoff()
	tradeoff.SetOptions([]research.TradeoffOption{{Label: "Alpha"}, {Label: "Beta"}})

	if tradeoff.selected != -1 {
		t.Fatalf("expected initial selection -1, got %d", tradeoff.selected)
	}

	tradeoff, _ = tradeoff.Update(tea.KeyMsg{Type: tea.KeyDown})

	if tradeoff.selected != 0 {
		t.Fatalf("expected selection 0 after nav down, got %d", tradeoff.selected)
	}
}

func TestTradeoffEnterAdoptsSelectedOption(t *testing.T) {
	tradeoff := NewTradeoff()
	tradeoff.SetOptions([]research.TradeoffOption{
		{Label: "Alpha", InsightID: "a"},
		{Label: "Beta", InsightID: "b"},
	})
	tradeoff.Select(1)

	_, cmd := tradeoff.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command for selected option")
	}

	msg := cmd()
	selected, ok := msg.(TradeoffSelectedMsg)
	if !ok {
		t.Fatalf("expected TradeoffSelectedMsg, got %T", msg)
	}

	if selected.Index != 1 {
		t.Fatalf("expected index 1, got %d", selected.Index)
	}
	if selected.Option.Label != "Beta" {
		t.Fatalf("expected label Beta, got %q", selected.Option.Label)
	}
}
