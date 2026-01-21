package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func ExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export state to JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "Export not implemented")
			return nil
		},
	}
}
