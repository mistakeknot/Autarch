package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func StopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop all agent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "Stop not implemented")
			return nil
		},
	}
}
