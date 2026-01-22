package tui

import (
	"encoding/json"
	"os"
)

type UIState struct {
	Expanded     map[string]bool `json:"expanded"`
	SelectedID   string          `json:"selected_id"`
	ShowArchived bool            `json:"show_archived"`
	LastAction   *LastAction     `json:"last_action"`
}

type LastAction struct {
	Type string   `json:"type"`
	ID   string   `json:"id"`
	From []string `json:"from"`
	To   []string `json:"to"`
}

func LoadUIState(path string) (UIState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return UIState{Expanded: map[string]bool{}}, err
	}
	var out UIState
	if err := json.Unmarshal(raw, &out); err != nil {
		return UIState{Expanded: map[string]bool{}}, err
	}
	if out.Expanded == nil {
		out.Expanded = map[string]bool{}
	}
	return out, nil
}

func SaveUIState(path string, state UIState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
