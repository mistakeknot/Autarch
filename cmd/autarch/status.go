package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/status"
)

func statusCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Intercore kernel status in a live TUI",
		Long: `Display active runs, dispatches, events, and token usage from the Intercore kernel.

Data is read from the ic CLI and refreshed every 3 seconds.

Navigation:
  ↑/↓ or j/k  Select run
  r            Force refresh
  q            Quit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve project directory
			dir, err := resolveProjectDir(projectDir)
			if err != nil {
				return err
			}

			// Verify ic is available
			if _, err := exec.LookPath("ic"); err != nil {
				return fmt.Errorf("ic not found in PATH — install Intercore first")
			}

			// Verify database exists
			dbPath := filepath.Join(dir, ".clavain", "intercore.db")
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("no Intercore database at %s — run 'ic init' first", dbPath)
			}

			m := status.New(dir)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}

	cmd.Flags().StringVar(&projectDir, "project", "", "Project directory (default: auto-discover from CWD)")

	return cmd
}

// resolveProjectDir finds the project directory containing .clavain/intercore.db.
// If dir is specified, use it directly. Otherwise walk up from CWD.
func resolveProjectDir(dir string) (string, error) {
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("invalid project dir: %w", err)
		}
		return abs, nil
	}

	// Walk up from CWD looking for .clavain/intercore.db
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}

	dir = cwd
	for {
		dbPath := filepath.Join(dir, ".clavain", "intercore.db")
		if _, err := os.Stat(dbPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", fmt.Errorf("no .clavain/intercore.db found (searched from %s to /)", cwd)
		}
		dir = parent
	}
}
