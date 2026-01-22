package commands

import (
	"os"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/tui"
)

func loadUIState(root string) (tui.UIState, error) {
	state, err := tui.LoadUIState(project.StatePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return tui.UIState{Expanded: map[string]bool{}}, nil
		}
		return tui.UIState{}, err
	}
	if state.Expanded == nil {
		state.Expanded = map[string]bool{}
	}
	return state, nil
}

func saveUIState(root string, state tui.UIState) error {
	return tui.SaveUIState(project.StatePath(root), state)
}

func recordLastAction(root string, action tui.LastAction) error {
	state, err := loadUIState(root)
	if err != nil {
		return err
	}
	state.LastAction = &action
	return saveUIState(root, state)
}
