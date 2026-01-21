package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func wrapCommandError(command string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s failed: %w", command, err)
}

func wrapArgs(command string, validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return wrapCommandError(command, err)
		}
		return nil
	}
}
