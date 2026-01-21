package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func ImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import state from JSON",
		Args:  wrapArgs("import", cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "Import not implemented")
			return nil
		},
	}
}
