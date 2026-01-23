package commands

import (
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/plan"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/spf13/cobra"
)

func PlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Run planning flow",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("plan", err)
				}
			}()
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			return plan.Run(cmd.InOrStdin(), cmd.OutOrStdout(), filepath.Join(root, ".tandemonium", "plan"))
		},
	}
}
