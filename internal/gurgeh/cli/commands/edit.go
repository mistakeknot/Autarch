package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/spf13/cobra"
)

// EditCmd opens a PRD for editing.
func EditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <PRD-ID>",
		Short: "Edit a PRD in your editor",
		Long: `Open a PRD specification for editing in your default editor.

The editor is determined by $EDITOR, falling back to vim.

Examples:
  gurgeh edit PRD-001
  EDITOR=code gurgeh edit PRD-001`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}

			prdID := args[0]

			// Normalize ID
			if !strings.HasPrefix(prdID, "PRD-") {
				prdID = "PRD-" + prdID
			}

			// Find the PRD file
			specPath := filepath.Join(cwd, ".gurgeh", "specs", prdID+".yaml")
			if _, err := os.Stat(specPath); os.IsNotExist(err) {
				return fmt.Errorf("PRD not found: %s", prdID)
			}

			// Get editor
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}

			// Open editor
			editorCmd := exec.Command(editor, specPath)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr

			if err := editorCmd.Run(); err != nil {
				return fmt.Errorf("editor failed: %w", err)
			}

			fmt.Printf("Edited: %s\n", prdID)
			fmt.Println("\nNext steps:")
			fmt.Printf("  gurgeh validate %s  # Validate changes\n", prdID)
			fmt.Printf("  gurgeh approve %s   # Mark as approved\n", prdID)

			return nil
		},
	}

	return cmd
}
