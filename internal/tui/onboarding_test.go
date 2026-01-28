package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type onboardingCancelMsg struct{}

func TestOnboardingQuitKeyCancels(t *testing.T) {
	orchestrator := NewOnboardingOrchestrator()
	called := false
	orchestrator.SetCallbacks(nil, func() tea.Cmd {
		called = true
		return func() tea.Msg { return onboardingCancelMsg{} }
	})

	_, cmd := orchestrator.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if !called {
		t.Fatal("expected onCancel to be called")
	}
	if cmd == nil {
		t.Fatal("expected a cancel command")
	}
	if _, ok := cmd().(onboardingCancelMsg); !ok {
		t.Fatalf("expected onboardingCancelMsg, got %T", cmd())
	}
}
