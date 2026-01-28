package tui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCommonKeysBackMatchesEscAndH(t *testing.T) {
	keys := NewCommonKeys()
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	if !key.Matches(esc, keys.Back) {
		t.Fatalf("expected Back to match esc")
	}
	h := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	if !key.Matches(h, keys.Back) {
		t.Fatalf("expected Back to match h")
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

	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, top) {
		t.Fatalf("expected Top to match g")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, bottom) {
		t.Fatalf("expected Bottom to match G")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}, next) {
		t.Fatalf("expected Next to match n")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, prev) {
		t.Fatalf("expected Prev to match p")
	}
	if len(sections) < 2 {
		t.Fatalf("expected Sections bindings")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, sections[0]) {
		t.Fatalf("expected Sections[0] to match 1")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}}, sections[len(sections)-1]) {
		t.Fatalf("expected last Sections binding to match 9")
	}
}

func TestHandleCommonQuitAndHelp(t *testing.T) {
	keys := NewCommonKeys()

	quitCmd := HandleCommon(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keys)
	if quitCmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg")
	}

	helpCmd := HandleCommon(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}, keys)
	if helpCmd == nil {
		t.Fatalf("expected help command")
	}
	if _, ok := helpCmd().(ToggleHelpMsg); !ok {
		t.Fatalf("expected ToggleHelpMsg")
	}
}
