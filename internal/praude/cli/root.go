package cli

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/cli/commands"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/tui"
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
		Use:   "praude",
		Short: "PM-focused PRD CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
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
		commands.ListCmd(),
		commands.ShowCmd(),
		commands.InterviewCmd(),
		commands.RunCmd(),
		commands.ResearchCmd(),
		commands.SuggestCmd(),
		commands.SuggestionsCmd(),
		commands.ValidateCmd(),
		commands.ArchiveCmd(),
		commands.DeleteCmd(),
		commands.UndoCmd(),
	)
	return root
}
