package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/spf13/cobra"
)

func reconcileCmd() *cobra.Command {
	var projectPath string
	var eventsDB string

	cmd := &cobra.Command{
		Use:   "reconcile [project-path]",
		Short: "Reconcile file state into the event spine",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectPath == "" {
				if len(args) > 0 {
					projectPath = args[0]
				} else {
					cwd, err := os.Getwd()
					if err != nil {
						return err
					}
					projectPath = cwd
				}
			}

			absPath, err := filepath.Abs(projectPath)
			if err != nil {
				return err
			}

			store, err := events.OpenStore(eventsDB)
			if err != nil {
				return err
			}
			defer store.Close()

			summary, err := events.ReconcileProject(absPath, store)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Reconciled %s\n", absPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Specs: %d seen, %d events\n", summary.SpecsSeen, summary.SpecsEmitted)
			fmt.Fprintf(cmd.OutOrStdout(), "Tasks: %d seen, %d events\n", summary.TasksSeen, summary.TaskEventsEmitted)
			fmt.Fprintf(cmd.OutOrStdout(), "Conflicts: %d\n", summary.Conflicts)
			return nil
		},
	}

	cmd.Flags().StringVar(&projectPath, "project", "", "Project root to reconcile")
	cmd.Flags().StringVar(&eventsDB, "events-db", "", "Override events DB path")

	return cmd
}
