package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/cli/commands"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/tui"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRoot().Execute()
}

var runTUI = func() error {
	m := tui.NewModel()
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "gurgeh",
		Short: "PM-focused PRD CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Deprecation warning for standalone TUI
			fmt.Fprintln(cmd.ErrOrStderr(), "\033[33m⚠ Deprecation warning: standalone gurgeh TUI is deprecated.\033[0m")
			fmt.Fprintln(cmd.ErrOrStderr(), "  Use: autarch tui --tool=gurgeh")
			fmt.Fprintln(cmd.ErrOrStderr(), "  CLI commands (list, create, export, etc.) remain available.")
			fmt.Fprintln(cmd.ErrOrStderr())

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}
			return runTUI()
		},
	}
	root.AddCommand(
		commands.InitCmd(),
		commands.CreateCmd(),
		commands.ExploreCmd(),
		commands.ExportCmd(),
		commands.ListCmd(),
		commands.ShowCmd(),
		commands.EditCmd(),
		commands.InterviewCmd(),
		commands.RunCmd(),
		commands.ResearchCmd(),
		commands.ImportResearchCmd(),
		commands.SuggestCmd(),
		commands.SuggestionsCmd(),
		commands.ValidateCmd(),
		commands.ReviewCmd(),
		commands.ApproveCmd(),
		commands.ArchiveCmd(),
		commands.DeleteCmd(),
		commands.UndoCmd(),
		commands.ApplyCmd(),
		commands.HistoryCmd(),
		commands.DiffCmd(),
		commands.PrioritizeCmd(),
		commands.SignalsCmd(),
		commands.VisionReviewCmd(),
		commands.ServeCmd(),
	)
	return root
}
