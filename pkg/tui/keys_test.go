package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewCommonKeys(t *testing.T) {
	keys := NewCommonKeys()

	tests := []struct {
		name    string
		binding key.Binding
		inputs  []string
	}{
		{"Quit q", keys.Quit, []string{"q"}},
		{"Quit ctrl+c", keys.Quit, []string{"ctrl+c"}},
		{"Help", keys.Help, []string{"?"}},
		{"Search", keys.Search, []string{"/"}},
		{"Back", keys.Back, []string{"esc"}},
		{"NavUp k", keys.NavUp, []string{"k"}},
		{"NavUp arrow", keys.NavUp, []string{"up"}},
		{"NavDown j", keys.NavDown, []string{"j"}},
		{"NavDown arrow", keys.NavDown, []string{"down"}},
		{"Refresh", keys.Refresh, []string{"r"}},
		{"TabCycle tab", keys.TabCycle, []string{"tab"}},
		{"TabCycle shift+tab", keys.TabCycle, []string{"shift+tab"}},
		{"Select", keys.Select, []string{"enter"}},
		{"Toggle", keys.Toggle, []string{" "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, inp := range tt.inputs {
				msg := tea.KeyMsg{Type: tea.KeyRunes}
				// Map special keys to their tea.KeyType
				switch inp {
				case "ctrl+c":
					msg = tea.KeyMsg{Type: tea.KeyCtrlC}
				case "esc":
					msg = tea.KeyMsg{Type: tea.KeyEscape}
				case "up":
					msg = tea.KeyMsg{Type: tea.KeyUp}
				case "down":
					msg = tea.KeyMsg{Type: tea.KeyDown}
				case "tab":
					msg = tea.KeyMsg{Type: tea.KeyTab}
				case "shift+tab":
					msg = tea.KeyMsg{Type: tea.KeyShiftTab}
				case "enter":
					msg = tea.KeyMsg{Type: tea.KeyEnter}
				case " ":
					msg = tea.KeyMsg{Type: tea.KeySpace}
				default:
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(inp)}
				}
				if !key.Matches(msg, tt.binding) {
					t.Errorf("expected key %q to match binding %s", inp, tt.name)
				}
			}
		})
	}
}

func TestHandleCommon_Quit(t *testing.T) {
	keys := NewCommonKeys()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	cmd := HandleCommon(msg, keys)
	if cmd == nil {
		t.Fatal("expected tea.Quit command for 'q'")
	}
	// Execute the cmd; tea.Quit returns a QuitMsg
	result := cmd()
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", result)
	}
}

func TestHandleCommon_Help(t *testing.T) {
	keys := NewCommonKeys()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}
	cmd := HandleCommon(msg, keys)
	if cmd == nil {
		t.Fatal("expected command for '?'")
	}
	result := cmd()
	if _, ok := result.(ToggleHelpMsg); !ok {
		t.Fatalf("expected ToggleHelpMsg, got %T", result)
	}
}

func TestHandleCommon_Unhandled(t *testing.T) {
	keys := NewCommonKeys()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	cmd := HandleCommon(msg, keys)
	if cmd != nil {
		t.Fatal("expected nil for unhandled key")
	}
}

func TestHelpOverlay_Toggle(t *testing.T) {
	h := NewHelpOverlay()
	if h.Visible {
		t.Fatal("expected hidden by default")
	}
	h.Toggle()
	if !h.Visible {
		t.Fatal("expected visible after toggle")
	}
	h.Toggle()
	if h.Visible {
		t.Fatal("expected hidden after second toggle")
	}
}

func TestHelpOverlay_RenderHidden(t *testing.T) {
	h := NewHelpOverlay()
	keys := NewCommonKeys()
	result := h.Render(keys, nil, 80)
	if result != "" {
		t.Fatal("expected empty string when hidden")
	}
}

func TestHelpOverlay_RenderVisible(t *testing.T) {
	h := NewHelpOverlay()
	h.Toggle()
	keys := NewCommonKeys()
	extras := []HelpBinding{{Key: "d", Description: "delete"}}
	result := h.Render(keys, extras, 80)
	if result == "" {
		t.Fatal("expected non-empty render when visible")
	}
	// Should contain key descriptions
	if !containsStr(result, "quit") {
		t.Error("expected 'quit' in rendered output")
	}
	if !containsStr(result, "delete") {
		t.Error("expected 'delete' in rendered output")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && strings.Contains(s, sub)
}
