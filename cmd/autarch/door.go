package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/door"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

func doorCmd() *cobra.Command {
	var (
		roots       []string
		rankingPath string
	)

	cmd := &cobra.Command{
		Use:   "door",
		Short: "The estate door: project rows ranked by ruling 11",
		Long: `The thin project switcher: one row per project, ordered funded first,
then pins, then weakest card first (ruling 11), with each card's verdict and
strength read from card-check.py. Enter opens the selected project's card in
Zed -- including a card that does not exist yet, which is where backfill on
first touch starts.

Verdicts come from card-check.py on PATH (deployed by the dotfiles
installers). A project the checker could not read shows as UNCHECKED, which
is a different fact from ABSENT and is never folded into it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(roots) == 0 {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("no --root and no home directory: %w", err)
				}
				roots = []string{filepath.Join(home, "projects")}
			}
			if rankingPath == "" {
				rankingPath = door.DefaultRankingPath()
			}

			projects, err := door.DiscoverProjects(roots)
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				return fmt.Errorf("no git repositories under %v", roots)
			}

			// Both failures stay non-fatal: the door opens and states them.
			// A broken ranking file degrades the order; a missing checker
			// leaves every row unchecked. Refusing to open would hide the
			// same facts a working door exists to show.
			ranking, rankingErr := door.LoadRanking(rankingPath)
			checker, checkerErr := door.ResolveChecker()

			m := door.NewModel(projects, ranking, rankingPath, rankingErr, checker, checkerErr)

			defer pkgtui.RestoreTerminalOnPanic()
			_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
			return err
		},
	}

	cmd.Flags().StringArrayVar(&roots, "root", nil, "estate root to scan (repeatable; default ~/projects)")
	cmd.Flags().StringVar(&rankingPath, "ranking", "", "ranking file (default ~/.autarch/door.yaml)")

	return cmd
}
