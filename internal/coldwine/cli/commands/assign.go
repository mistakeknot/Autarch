package commands

import (
	"fmt"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/spf13/cobra"
)

func AssignCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assign <task-id> <assignee>",
		Short: "Assign a task to an agent or user",
		Args: wrapArgs("assign", func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			return project.ValidateTaskID(args[0])
		}),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("assign", err)
				}
			}()
			taskID := args[0]
			assignee := args[1]
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			db, err := storage.Open(project.StateDBPath(root))
			if err != nil {
				return err
			}
			defer db.Close()
			if err := storage.AssignWorkTask(db, taskID, assignee); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Assigned %s to %s\n", taskID, assignee)
			return nil
		},
	}
	return cmd
}
