package tui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCommonKeysBackMatchesEscOnly(t *testing.T) {
	keys := NewCommonKeys()
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	if !key.Matches(esc, keys.Back) {
		t.Fatalf("expected Back to match esc")
	}
	h := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	if key.Matches(h, keys.Back) {
		t.Fatalf("expected Back to not match h")
	}
}

func TestCommonKeysIncludesTopBottomNextPrevAndSections(t *testing.T) {
	keys := NewCommonKeys()
	typ := reflect.TypeOf(keys)
	for _, field := range []string{"Top", "Bottom", "Next", "Prev", "Sections"} {
		if _, ok := typ.FieldByName(field); !ok {
			t.Fatalf("expected CommonKeys to include %s", field)
		}
	}

	v := reflect.ValueOf(keys)
	top := v.FieldByName("Top").Interface().(key.Binding)
	bottom := v.FieldByName("Bottom").Interface().(key.Binding)
	next := v.FieldByName("Next").Interface().(key.Binding)
	prev := v.FieldByName("Prev").Interface().(key.Binding)
	sections := v.FieldByName("Sections").Interface().([]key.Binding)

	if !key.Matches(tea.KeyMsg{Type: tea.KeyHome}, top) {
		t.Fatalf("expected Top to match home")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyEnd}, bottom) {
		t.Fatalf("expected Bottom to match end")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyPgDown}, next) {
		t.Fatalf("expected Next to match pgdown")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyPgUp}, prev) {
		t.Fatalf("expected Prev to match pgup")
	}
	if len(sections) != 0 {
		t.Fatalf("expected Sections to be empty")
	}
}

func TestHandleCommonQuitAndHelp(t *testing.T) {
	keys := NewCommonKeys()

	quitCmd := HandleCommon(tea.KeyMsg{Type: tea.KeyCtrlC}, keys)
	if quitCmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg")
	}

	if cmd := HandleCommon(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keys); cmd != nil {
		t.Fatalf("expected q to no longer trigger quit")
	}

	// F1 still works for external keyboards
	helpCmd := HandleCommon(tea.KeyMsg{Type: tea.KeyF1}, keys)
	if helpCmd == nil {
		t.Fatalf("expected help command from F1")
	}
	if _, ok := helpCmd().(ToggleHelpMsg); !ok {
		t.Fatalf("expected ToggleHelpMsg from F1")
	}

	// ? should NOT trigger help (use /help slash command instead)
	helpCmd2 := HandleCommon(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}, keys)
	if helpCmd2 != nil {
		t.Fatalf("expected ? to NOT trigger help (reserved for typing in chat)")
	}
}
