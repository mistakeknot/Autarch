package commands

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tui"
	"github.com/spf13/cobra"
)

func ExecuteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "execute",
		Short: "Launch execute mode",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("execute", err)
				}
			}()
			cfg, err := config.LoadFromProject(".")
			if err != nil {
				return err
			}
			m := tui.NewModel()
			m.ConfirmApprove = cfg.TUI.ConfirmApprove
			p := tea.NewProgram(m)
			_, err = p.Run()
			return err
		},
	}
}
