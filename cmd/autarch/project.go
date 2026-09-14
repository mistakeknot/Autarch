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
	var workBinary, workRegistry, workAuthority string
	command := &cobra.Command{
		Use:   "project [path]",
		Short: "Read product context and prepare a project's foundation onboarding",
		Long: `Open the product HUD for a project (default: current directory).
Reads docs/why.md, docs/roadmap.md, docs/cujs, card-linked decisions and the
nearest Beads tracker. Shared trackers are filtered by the card's project label.
Use 1–7 or Tab for sections, arrows to scroll, o for source files, r to refresh.
The first section is the direct-agent workbench. 7 Foundation discovers mission,
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
			model := door.NewProductModel(root).WithDisplay(door.DefaultDisplayPath())
			configured := workBinary != "" || workRegistry != "" || workAuthority != ""
			if configured {
				if workBinary == "" || workRegistry == "" || workAuthority == "" {
					return fmt.Errorf("--work-binary, --work-registry and --work-authority must be set together")
				}
				model = model.WithWorkAdapter(door.WorkAdapterConfig{Binary: workBinary, Registry: workRegistry, Authority: workAuthority})
			}
			defer pkgtui.RestoreTerminalOnPanic()
			_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
			return err
		},
	}
	command.Flags().StringVar(&workBinary, "work-binary", "", "absolute path to an explicitly qualified clavain-cli")
	command.Flags().StringVar(&workRegistry, "work-registry", "", "absolute path to its read-only work registry")
	command.Flags().StringVar(&workAuthority, "work-authority", "", "explicit authority label for read-only work discovery")
	return command
}
