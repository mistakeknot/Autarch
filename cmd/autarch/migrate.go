package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// legacyMapping defines old directory names to their new equivalents.
var legacyMapping = map[string]string{
	".praude":      ".gurgeh",
	".tandemonium": ".coldwine",
}

func migrateCmd() *cobra.Command {
	var (
		dryRun       bool
		removeLegacy bool
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate legacy directory names to new names",
		Long: `Migrate legacy Autarch directory names to their new equivalents.

This command renames:
  .praude/      → .gurgeh/
  .tandemonium/ → .coldwine/

The migration uses atomic renames (no data copy on same filesystem).
By default, a marker file is left behind (e.g., .praude.migrated) with
a timestamp. Use --remove-legacy to skip creating marker files.

Examples:
  autarch migrate              # Migrate in current directory
  autarch migrate --dry-run    # Preview what would happen
  autarch migrate --remove-legacy  # Migrate without leaving marker files`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			return runMigrate(cwd, dryRun, removeLegacy, cmd)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without making them")
	cmd.Flags().BoolVar(&removeLegacy, "remove-legacy", false, "Don't create .migrated marker files")

	return cmd
}

// MigrateResult contains the outcome of a single directory migration.
type MigrateResult struct {
	OldName   string
	NewName   string
	FileCount int
	Migrated  bool
	Skipped   bool
	Error     error
}

func runMigrate(root string, dryRun, removeLegacy bool, cmd *cobra.Command) error {
	var results []MigrateResult

	for oldName, newName := range legacyMapping {
		result := migrateDirectory(root, oldName, newName, dryRun, removeLegacy)
		results = append(results, result)
	}

	// Print summary
	fmt.Fprintln(cmd.OutOrStdout())
	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "=== Dry Run (no changes made) ===")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "=== Migration Summary ===")
	}

	var anyErrors bool
	var anyMigrated bool
	for _, r := range results {
		if r.Error != nil {
			fmt.Fprintf(cmd.OutOrStderr(), "ERROR: %s → %s: %v\n", r.OldName, r.NewName, r.Error)
			anyErrors = true
		} else if r.Migrated {
			action := "Migrated"
			if dryRun {
				action = "Would migrate"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s (%d files)\n", action, r.OldName, r.NewName, r.FileCount)
			anyMigrated = true
		} else if r.Skipped {
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped: %s (already migrated or not present)\n", r.OldName)
		}
	}

	if !anyMigrated && !anyErrors {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to migrate.")
	}

	if anyErrors {
		return fmt.Errorf("migration completed with errors")
	}
	return nil
}

func migrateDirectory(root, oldName, newName string, dryRun, removeLegacy bool) MigrateResult {
	oldPath := filepath.Join(root, oldName)
	newPath := filepath.Join(root, newName)

	result := MigrateResult{
		OldName: oldName,
		NewName: newName,
	}

	// Check if old directory exists
	oldInfo, oldErr := os.Stat(oldPath)
	if os.IsNotExist(oldErr) {
		// Legacy directory doesn't exist - check if already migrated
		if _, newErr := os.Stat(newPath); newErr == nil {
			result.Skipped = true
			return result
		}
		// Neither exists - nothing to do
		result.Skipped = true
		return result
	}
	if oldErr != nil {
		result.Error = fmt.Errorf("failed to stat %s: %w", oldPath, oldErr)
		return result
	}
	if !oldInfo.IsDir() {
		result.Error = fmt.Errorf("%s exists but is not a directory", oldPath)
		return result
	}

	// Check if new directory already exists
	if _, newErr := os.Stat(newPath); newErr == nil {
		result.Error = fmt.Errorf("both %s and %s exist - please resolve manually (remove or merge one)", oldName, newName)
		return result
	}

	// Count files in old directory
	fileCount, err := countFiles(oldPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to count files in %s: %w", oldPath, err)
		return result
	}
	result.FileCount = fileCount

	if dryRun {
		result.Migrated = true
		return result
	}

	// Perform the rename
	if err := os.Rename(oldPath, newPath); err != nil {
		result.Error = fmt.Errorf("failed to rename %s to %s: %w", oldPath, newPath, err)
		return result
	}

	// Create marker file unless --remove-legacy is set
	if !removeLegacy {
		markerPath := oldPath + ".migrated"
		markerContent := fmt.Sprintf("Migrated to %s at %s\n", newName, time.Now().Format(time.RFC3339))
		if err := os.WriteFile(markerPath, []byte(markerContent), 0644); err != nil {
			// Non-fatal - log but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to create marker file %s: %v\n", markerPath, err)
		}
	}

	result.Migrated = true
	return result
}

func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}
