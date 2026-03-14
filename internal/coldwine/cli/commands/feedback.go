package commands

import (
	"fmt"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/specs"
	"github.com/spf13/cobra"
)

func FeedbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback <task-id> <text>",
		Short: "Add review feedback to a task",
		Args: wrapArgs("feedback", func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			return project.ValidateTaskID(args[0])
		}),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("feedback", err)
				}
			}()
			taskID := args[0]
			text := args[1]
			root, err := project.FindRoot(".")
			if err != nil {
				return err
			}
			path, err := project.TaskSpecPath(root, taskID)
			if err != nil {
				return err
			}
			if err := specs.AppendReviewFeedback(path, text); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Feedback added to %s\n", taskID)
			return nil
		},
	}
	return cmd
}
