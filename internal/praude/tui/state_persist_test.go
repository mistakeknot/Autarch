package tui

import (
	"path/filepath"
	"testing"
)

func TestLoadSaveUIState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := UIState{
		Expanded:   map[string]bool{"draft": true, "research": false},
		SelectedID: "PRD-123",
	}
	if err := SaveUIState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUIState(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SelectedID != "PRD-123" || loaded.Expanded["draft"] != true {
		t.Fatalf("state not preserved")
	}
}

func TestLoadSaveUIStateIncludesArchiveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := UIState{
		Expanded:     map[string]bool{"draft": true},
		SelectedID:   "PRD-123",
		ShowArchived: true,
		LastAction:   &LastAction{Type: "archive", ID: "PRD-123"},
	}
	if err := SaveUIState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUIState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ShowArchived || loaded.LastAction == nil {
		t.Fatalf("expected archive fields persisted")
	}
}
