package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/spf13/cobra"
)

func ListCmd() *cobra.Command {
	var includeArchived bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List PRD specs",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			summaries, _ := specs.LoadSummariesWithArchived(project.SpecsDir(root), project.ArchivedSpecsDir(root), includeArchived)
			for _, s := range summaries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", s.ID, s.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived PRDs")
	return cmd
}
