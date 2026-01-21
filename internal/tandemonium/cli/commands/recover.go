package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
	"github.com/spf13/cobra"
)

func RecoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recover",
		Short: "Recover from crash",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("recover", err)
				}
			}()
			cwd, _ := os.Getwd()
			root, err := project.FindRoot(cwd)
			if err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Not initialized (.tandemonium not found)")
				return nil
			}

			specs, _ := listSpecFiles(project.SpecsDir(root))
			sessions, _ := tmux.ListSessions("tand-")

			fmt.Fprintln(cmd.OutOrStdout(), "Recovery plan:")
			fmt.Fprintf(cmd.OutOrStdout(), "  Rebuild from %d spec file(s)\n", len(specs))
			for _, s := range specs {
				fmt.Fprintf(cmd.OutOrStdout(), "   - %s\n", s)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  Reattach to %d tmux session(s)\n", len(sessions))
			for _, s := range sessions {
				fmt.Fprintf(cmd.OutOrStdout(), "   - %s\n", s)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Proceed? [y/N]")
			var answer string
			if _, err := fmt.Fscanln(cmd.InOrStdin(), &answer); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Recovery cancelled.")
				return nil
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				fmt.Fprintln(cmd.OutOrStdout(), "Recovery cancelled.")
				return nil
			}
			if err := storage.RebuildFromSpecs(root); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Rebuilt state from specs.")
			return nil
		},
	}
}

func listSpecFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}, nil
	}
	var specs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			specs = append(specs, filepath.Join(dir, name))
		}
	}
	return specs, nil
}
