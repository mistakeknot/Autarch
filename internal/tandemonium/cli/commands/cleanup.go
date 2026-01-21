package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
	"github.com/spf13/cobra"
)

func CleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Clean orphaned worktrees and sessions",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("cleanup", err)
				}
			}()
			cwd, _ := os.Getwd()
			root, err := project.FindRoot(cwd)
			if err != nil {
				return fmt.Errorf("not a Tandemonium project")
			}

			worktrees, _ := listDirNames(project.WorktreesDir(root))
			sessions, _ := tmux.ListSessions("tand-")

			fmt.Fprintln(cmd.OutOrStdout(), "Cleanup (dry-run):")
			fmt.Fprintf(cmd.OutOrStdout(), "  Would inspect %d worktree(s)\n", len(worktrees))
			for _, w := range worktrees {
				fmt.Fprintf(cmd.OutOrStdout(), "   - %s\n", w)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Would inspect %d tmux session(s)\n", len(sessions))
			for _, s := range sessions {
				fmt.Fprintf(cmd.OutOrStdout(), "   - %s\n", s)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "  No changes applied.")
			return nil
		},
	}
}

func listDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
