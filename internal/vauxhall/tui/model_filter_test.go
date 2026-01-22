package tui

import (
	"testing"

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
