package commands

import (
	"fmt"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/specs"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/spf13/cobra"
)

func RejectCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <task-id>",
		Short: "Reject a task and return it to the queue",
		Args: wrapArgs("reject", func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			return project.ValidateTaskID(args[0])
		}),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("reject", err)
				}
			}()
			taskID := args[0]
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			if reason != "" {
				path, pathErr := project.TaskSpecPath(root, taskID)
				if pathErr != nil {
					return pathErr
				}
				if feedbackErr := specs.AppendReviewFeedback(path, reason); feedbackErr != nil {
					return feedbackErr
				}
			}
			db, err := storage.Open(project.StateDBPath(root))
			if err != nil {
				return err
			}
			defer db.Close()
			if err := storage.RejectTask(db, taskID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rejected %s\n", taskID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "rejection reason (appended as review feedback)")
	return cmd
}
