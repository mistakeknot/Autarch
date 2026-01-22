package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/archive"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/tui"
	"github.com/spf13/cobra"
)

func UndoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Undo the last archive/delete action",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			action, err := undoCmdRun(root)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Undid %s %s\n", action.Type, action.ID)
			return nil
		},
	}
}

func undoCmdRun(root string) (tui.LastAction, error) {
	state, err := loadUIState(root)
	if err != nil {
		return tui.LastAction{}, err
	}
	if state.LastAction == nil {
		return tui.LastAction{}, fmt.Errorf("no action to undo")
	}
	action := *state.LastAction
	if err := archive.Undo(root, action.From, action.To); err != nil {
		return tui.LastAction{}, err
	}
	if strings.TrimSpace(action.PrevStatus) != "" {
		specPath := specPathForAction(root, action)
		if err := specs.UpdateStatus(specPath, action.PrevStatus); err != nil {
			return tui.LastAction{}, err
		}
	}
	state.LastAction = nil
	if err := saveUIState(root, state); err != nil {
		return tui.LastAction{}, err
	}
	return action, nil
}

func specPathForAction(root string, action tui.LastAction) string {
	suffix := action.ID + ".yaml"
	for _, path := range action.From {
		if strings.HasSuffix(path, suffix) {
			return path
		}
	}
	return filepath.Join(project.SpecsDir(root), suffix)
}
