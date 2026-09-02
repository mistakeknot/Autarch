package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/door"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// doorOptions are the flags the surface takes, whether opened as bare
// `autarch` (the 2026-09-01 ruling: the surface has no name of its own) or as
// `autarch door`, its earlier name kept as an alias.
type doorOptions struct {
	roots       []string
	rankingPath string
	since       string
	layout      string
}

func addDoorFlags(cmd *cobra.Command, o *doorOptions) {
	cmd.Flags().StringArrayVar(&o.roots, "root", nil, "estate root to scan (repeatable; default ~/projects)")
	cmd.Flags().StringVar(&o.rankingPath, "ranking", "", "ranking file (default ~/.autarch/door.yaml)")
	cmd.Flags().StringVar(&o.since, "since", "", "briefing window: a duration (36h, 3d) or an RFC3339 time (default: since the last visit; 24h on the first)")
	cmd.Flags().StringVar(&o.layout, "layout", "alone", "where the briefing sits: alone (rows one tab away) or above (rows beneath it)")
}

// runDoor discovers the estate, resolves the briefing window, and opens the
// surface. Every failure that can be stated on screen stays non-fatal: a
// broken ranking file degrades the order, a missing checker leaves rows
// unchecked, an unreadable visit stamp widens the window and says so.
// Refusing to open would hide the same facts a working surface exists to show.
func runDoor(o doorOptions) error {
	roots := o.roots
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no --root and no home directory: %w", err)
		}
		roots = []string{filepath.Join(home, "projects")}
	}
	rankingPath := o.rankingPath
	if rankingPath == "" {
		rankingPath = door.DefaultRankingPath()
	}
	layout, err := door.ParseLayout(o.layout)
	if err != nil {
		return err
	}
	visitPath := door.DefaultVisitPath()
	since, source, sinceErr := door.Window(o.since, visitPath, time.Now())
	if since.IsZero() {
		return sinceErr // a --since that does not parse is the one fatal case
	}

	projects, err := door.DiscoverProjects(roots)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return fmt.Errorf("no git repositories under %v", roots)
	}

	ranking, rankingErr := door.LoadRanking(rankingPath)
	checker, checkerErr := door.ResolveChecker()

	m := door.NewModel(projects, ranking, rankingPath, rankingErr, checker, checkerErr).
		WithBriefing(door.BriefingOptions{
			Since:           since,
			SinceSource:     source,
			SinceErr:        sinceErr,
			VisitPath:       visitPath,
			TranscriptsRoot: door.DefaultTranscriptsRoot(),
			Layout:          layout,
		})

	defer pkgtui.RestoreTerminalOnPanic()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func doorCmd() *cobra.Command {
	var o doorOptions
	cmd := &cobra.Command{
		Use:   "door",
		Short: "Open Autarch (the bare command, by its earlier name)",
		Long: `Opens the same surface as bare "autarch": a briefing of what moved in the
estate since your last visit, with the ranked project rows one tab away.
One row per project, ordered funded first, then pins, then weakest card
first (ruling 11), each card's verdict and strength read from card-check.py.
Enter switches to the project's tmux session, or opens its card in Zed --
including a card that does not exist yet, which is where backfill on first
touch starts.

Verdicts come from card-check.py on PATH (deployed by the dotfiles
installers). A project the checker could not read shows as UNCHECKED, which
is a different fact from ABSENT and is never folded into it.`,
		RunE: func(cmd *cobra.Command, args []string) error { return runDoor(o) },
	}
	addDoorFlags(cmd, &o)
	return cmd
}
