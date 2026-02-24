package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPalette_NonBroadcastSkipsPhases(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Normal", Description: "non-broadcast", Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter on a non-broadcast command should hide and execute directly
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.Visible() {
		t.Error("palette should hide after entering non-broadcast command")
	}
	if p.phase != PhaseCommand {
		t.Errorf("phase = %v, want PhaseCommand", p.phase)
	}
}

func TestPalette_BroadcastEnterGoesToTarget(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Description: "broadcast", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter on a broadcast command should transition to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !p.Visible() {
		t.Error("palette should remain visible in target phase")
	}
	if p.phase != PhaseTarget {
		t.Errorf("phase = %v, want PhaseTarget", p.phase)
	}
}

func TestPalette_TargetPhaseKeySelectsTarget(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Enter to get to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press "2" to select Claude
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if p.phase != PhaseConfirm {
		t.Errorf("phase = %v, want PhaseConfirm", p.phase)
	}
	if p.target != TargetClaude {
		t.Errorf("target = %v, want TargetClaude", p.target)
	}
}

func TestPalette_ConfirmPhaseEnterExecutes(t *testing.T) {
	executed := false
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd {
			executed = true
			return nil
		}},
	})
	p.Show()

	// Command -> Target -> Confirm -> Execute
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}) // -> Confirm (All)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // -> Execute

	if !executed {
		t.Error("action should have been executed")
	}
	if p.Visible() {
		t.Error("palette should hide after execution")
	}
	if p.phase != PhaseCommand {
		t.Errorf("phase should reset to PhaseCommand, got %v", p.phase)
	}
}

func TestPalette_EscGoesBackOnePhase(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()

	// Command -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.phase != PhaseTarget {
		t.Fatalf("expected PhaseTarget, got %v", p.phase)
	}

	// Target -> Confirm
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if p.phase != PhaseConfirm {
		t.Fatalf("expected PhaseConfirm, got %v", p.phase)
	}

	// Confirm -> Target (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.phase != PhaseTarget {
		t.Errorf("expected PhaseTarget after esc from confirm, got %v", p.phase)
	}

	// Target -> Command (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.phase != PhaseCommand {
		t.Errorf("expected PhaseCommand after esc from target, got %v", p.phase)
	}
	if !p.Visible() {
		t.Error("palette should still be visible in command phase")
	}

	// Command -> Close (Esc)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.Visible() {
		t.Error("palette should be hidden after esc from command")
	}
}

func TestPalette_CtrlCClosesFromAnyPhase(t *testing.T) {
	for _, startPhase := range []Phase{PhaseCommand, PhaseTarget, PhaseConfirm} {
		t.Run(startPhase.String(), func(t *testing.T) {
			p := NewPalette()
			p.SetCommands([]Command{
				{Name: "Send", Broadcast: true, Action: func() tea.Cmd { return nil }},
			})
			p.Show()

			// Navigate to the target phase
			if startPhase >= PhaseTarget {
				p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if startPhase >= PhaseConfirm {
				p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
			}

			p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if p.Visible() {
				t.Error("ctrl+c should close palette from any phase")
			}
			if p.phase != PhaseCommand {
				t.Errorf("phase should reset to PhaseCommand, got %v", p.phase)
			}
		})
	}
}

func TestPalette_ShowResetsPhasesToCommand(t *testing.T) {
	p := NewPalette()
	p.phase = PhaseConfirm
	p.target = TargetClaude
	p.Show()

	if p.phase != PhaseCommand {
		t.Errorf("Show should reset phase to PhaseCommand, got %v", p.phase)
	}
}

// --- View tests (Task 4) ---

func TestPalette_ViewTargetPhaseShowsCounts(t *testing.T) {
	p := NewPalette()
	p.SetSize(80, 30)
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 0}

	// Navigate to target phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := p.View()
	if !strings.Contains(view, "All agents (3)") {
		t.Errorf("target view should show 'All agents (3)', got:\n%s", view)
	}
	if !strings.Contains(view, "Claude (2)") {
		t.Errorf("target view should show 'Claude (2)', got:\n%s", view)
	}
	if !strings.Contains(view, "Codex (1)") {
		t.Errorf("target view should show 'Codex (1)', got:\n%s", view)
	}
	if !strings.Contains(view, "Gemini (0)") {
		t.Errorf("target view should show 'Gemini (0)', got:\n%s", view)
	}
}

func TestPalette_ViewConfirmPhaseShowsDetails(t *testing.T) {
	p := NewPalette()
	p.SetSize(80, 30)
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.Show()
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 0}

	// Navigate to confirm phase
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})                     // -> Target
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}) // -> Confirm (Claude)

	view := p.View()
	if !strings.Contains(view, "Send Prompt") {
		t.Errorf("confirm view should show command name 'Send Prompt', got:\n%s", view)
	}
	if !strings.Contains(view, "Claude") {
		t.Errorf("confirm view should show target 'Claude', got:\n%s", view)
	}
	if !strings.Contains(view, "enter") {
		t.Errorf("confirm view should show 'enter' key hint, got:\n%s", view)
	}
}

// --- Async pane count tests (Task 5) ---

func TestPalette_TargetPhaseReturnsFetchCmd(t *testing.T) {
	p := NewPalette()
	p.SetCommands([]Command{
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd { return nil }},
	})
	p.SetPaneCountFetcher(func() tea.Msg {
		return PaneCountMsg{Counts: PaneCounts{Claude: 1}}
	})
	p.Show()

	// Entering target phase should return a command (for fetching pane counts)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("entering target phase should return a tea.Cmd for fetching pane counts")
	}
}

func TestPalette_PaneCountMsgUpdatesCounts(t *testing.T) {
	p := NewPalette()
	p.phase = PhaseTarget
	p.visible = true

	p, _ = p.Update(PaneCountMsg{Counts: PaneCounts{Claude: 3, Codex: 1, Gemini: 2}})

	if p.paneCounts.Claude != 3 {
		t.Errorf("Claude count = %d, want 3", p.paneCounts.Claude)
	}
	if p.paneCounts.Codex != 1 {
		t.Errorf("Codex count = %d, want 1", p.paneCounts.Codex)
	}
	if p.paneCounts.Gemini != 2 {
		t.Errorf("Gemini count = %d, want 2", p.paneCounts.Gemini)
	}
}

// --- Integration test (Task 7) ---

func TestPalette_FullBroadcastFlow(t *testing.T) {
	executed := false

	p := NewPalette()
	p.SetSize(80, 30)
	p.paneCounts = PaneCounts{Claude: 2, Codex: 1, Gemini: 3}
	p.SetPaneCountFetcher(func() tea.Msg {
		return PaneCountMsg{Counts: PaneCounts{Claude: 2, Codex: 1, Gemini: 3}}
	})

	p.SetCommands([]Command{
		{Name: "Normal Cmd", Action: func() tea.Cmd { return nil }},
		{Name: "Send Prompt", Broadcast: true, Action: func() tea.Cmd {
			// Do NOT read p.target or p.paneCounts here — this runs as tea.Cmd
			// on a goroutine, which would race with Update() writing these fields.
			executed = true
			return nil
		}},
	})

	// Open palette
	p.Show()

	// Type to filter to broadcast command
	for _, r := range "Send" {
		p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify "Send Prompt" is top match
	sel := p.Selected()
	if sel == nil || sel.Name != "Send Prompt" {
		t.Fatal("expected 'Send Prompt' to be selected after fuzzy search")
	}

	// Enter → Target phase
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.phase != PhaseTarget {
		t.Fatalf("expected PhaseTarget, got %v", p.phase)
	}
	if cmd == nil {
		t.Error("expected fetch cmd from target phase entry")
	}

	// Verify target view shows counts
	view := p.View()
	if !strings.Contains(view, "All agents (6)") {
		t.Errorf("target view should show total count 6, got:\n%s", view)
	}

	// Select "3" → Codex → Confirm
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if p.phase != PhaseConfirm {
		t.Fatalf("expected PhaseConfirm, got %v", p.phase)
	}
	if p.target != TargetCodex {
		t.Fatalf("expected TargetCodex, got %v", p.target)
	}

	// Verify confirm view
	view = p.View()
	if !strings.Contains(view, "Send Prompt") {
		t.Errorf("confirm view should show 'Send Prompt'")
	}
	if !strings.Contains(view, "Codex (1)") {
		t.Errorf("confirm view should show 'Codex (1)'")
	}

	// Enter → Execute
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.Visible() {
		t.Error("palette should hide after execution")
	}
	if !executed {
		t.Error("action should have been executed")
	}
}
