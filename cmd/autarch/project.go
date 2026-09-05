package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/door"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

func projectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "project [path]",
		Short: "Read product context and prepare a project's foundation onboarding",
		Long: `Open the product HUD for a project (default: current directory).
Reads docs/why.md, docs/roadmap.md, docs/cujs, card-linked decisions and the
nearest Beads tracker. Shared trackers are filtered by the card's project label.
Use 1–6 or Tab for sections, arrows to scroll, o for source files, r to refresh.
6 Foundation discovers mission, vision, philosophy, personas, journeys, roadmap,
ADRs, backlog, and design standards. n opens an onboarding brief; c copies it
for your chosen agent. Existing project sources stay unchanged.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			root, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			info, err := os.Stat(root)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("project path must be a directory: %s", root)
			}
			defer pkgtui.RestoreTerminalOnPanic()
			_, err = tea.NewProgram(door.NewProductModel(root).WithDisplay(door.DefaultDisplayPath()), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
			return err
		},
	}
}
