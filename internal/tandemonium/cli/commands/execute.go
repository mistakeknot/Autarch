package commands

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
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
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			db, err := storage.OpenShared(project.StateDBPath(root))
			if err != nil {
				return err
			}
			defer db.Close()
			if err := storage.Migrate(db); err != nil {
				return err
			}
			m := tui.NewModelWithDB(db)
			m.ConfirmApprove = cfg.TUI.ConfirmApprove
			p := tea.NewProgram(m)
			_, err = p.Run()
			return err
		},
	}
}
