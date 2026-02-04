package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandPickerFuzzyMatch(t *testing.T) {
	tests := []struct {
		name   string
		target string
		query  string
		want   bool
	}{
		{"exact match", "help", "help", true},
		{"prefix match", "help", "hel", true},
		{"substring match", "settings", "set", true},
		{"fuzzy match", "settings", "stg", true},
		{"no match", "help", "xyz", false},
		{"case insensitive", "Help", "help", true},
		{"empty query matches all", "anything", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fuzzyMatch(tt.target, tt.query); got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.target, tt.query, got, tt.want)
			}
		})
	}
}

func TestCommandPickerFilter(t *testing.T) {
	commands := []SlashCommandDef{
		{Command: "help", Aliases: []string{"h"}, Description: "Show help"},
		{Command: "quit", Aliases: []string{"q", "exit"}, Description: "Quit app"},
		{Command: "settings", Aliases: []string{"config"}, Description: "Open settings"},
	}

	picker := NewCommandPicker(commands)

	// Show with empty query - should show all
	picker.Show("")
	if len(picker.filtered) != 3 {
		t.Errorf("empty query: got %d filtered, want 3", len(picker.filtered))
	}

	// Filter by prefix
	picker.UpdateQuery("he")
	if len(picker.filtered) != 1 || picker.filtered[0].Command != "help" {
		t.Errorf("query 'he': got %v, want [help]", picker.filtered)
	}

	// Filter by alias
	picker.UpdateQuery("config")
	if len(picker.filtered) != 1 || picker.filtered[0].Command != "settings" {
		t.Errorf("query 'config': got %v, want [settings]", picker.filtered)
	}

	// Filter with no matches
	picker.UpdateQuery("xyz")
	if len(picker.filtered) != 0 {
		t.Errorf("query 'xyz': got %d filtered, want 0", len(picker.filtered))
	}
}

func TestCommandPickerNavigation(t *testing.T) {
	commands := []SlashCommandDef{
		{Command: "help"},
		{Command: "quit"},
		{Command: "settings"},
	}

	picker := NewCommandPicker(commands)
	picker.Show("")

	// Initial selection should be 0
	if picker.selected != 0 {
		t.Errorf("initial selection: got %d, want 0", picker.selected)
	}

	// Navigate down
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.selected != 1 {
		t.Errorf("after down: got %d, want 1", picker.selected)
	}

	// Navigate down again
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.selected != 2 {
		t.Errorf("after down: got %d, want 2", picker.selected)
	}

	// Navigate down at bottom - should stay at 2
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.selected != 2 {
		t.Errorf("at bottom: got %d, want 2", picker.selected)
	}

	// Navigate up
	picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.selected != 1 {
		t.Errorf("after up: got %d, want 1", picker.selected)
	}
}

func TestCommandPickerSelection(t *testing.T) {
	commands := []SlashCommandDef{
		{Command: "help"},
		{Command: "quit"},
	}

	picker := NewCommandPicker(commands)
	picker.Show("")
	picker.Update(tea.KeyMsg{Type: tea.KeyDown}) // Select "quit"

	// Press Enter to select
	selected, consumed := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "quit" {
		t.Errorf("Enter selection: got %q, want %q", selected, "quit")
	}
	if !consumed {
		t.Error("Enter should be consumed")
	}
	if picker.Visible() {
		t.Error("picker should be hidden after selection")
	}
}

func TestCommandPickerTabSelection(t *testing.T) {
	commands := []SlashCommandDef{
		{Command: "help"},
	}

	picker := NewCommandPicker(commands)
	picker.Show("")

	// Press Tab to select
	selected, consumed := picker.Update(tea.KeyMsg{Type: tea.KeyTab})
	if selected != "help" {
		t.Errorf("Tab selection: got %q, want %q", selected, "help")
	}
	if !consumed {
		t.Error("Tab should be consumed")
	}
}

func TestCommandPickerEscape(t *testing.T) {
	picker := NewCommandPicker(GlobalCommands())
	picker.Show("")

	if !picker.Visible() {
		t.Error("picker should be visible after Show")
	}

	picker.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if picker.Visible() {
		t.Error("picker should be hidden after Escape")
	}
}

func TestCommandPickerSortPrefixFirst(t *testing.T) {
	commands := []SlashCommandDef{
		{Command: "settings"},
		{Command: "scan"},
		{Command: "status"},
	}

	picker := NewCommandPicker(commands)
	picker.Show("s")

	// All start with 's', so should be sorted alphabetically
	if len(picker.filtered) != 3 {
		t.Errorf("got %d filtered, want 3", len(picker.filtered))
	}
	if picker.filtered[0].Command != "scan" {
		t.Errorf("first should be 'scan', got %q", picker.filtered[0].Command)
	}

	// Now filter to "sc" - only "scan" should match with prefix
	picker.UpdateQuery("sc")
	if len(picker.filtered) != 1 || picker.filtered[0].Command != "scan" {
		t.Errorf("filter 'sc': got %v, want [scan]", picker.filtered)
	}
}
