package commands

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mistakeknot/autarch/internal/coldwine/prd"
	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func ImportCmd() *cobra.Command {
	var fromBriefs string
	var storyID string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import state from JSON or Gurgeh briefs",
		Long: `Import state into Coldwine from various sources.

Examples:
  # Import from Gurgeh briefs (spec decomposition)
  coldwine import --from-briefs PRD-001

  # Import briefs into a specific story
  coldwine import --from-briefs PRD-001 --story STORY-001

  # Preview what would be imported
  coldwine import --from-briefs PRD-001 --dry-run`,
		Args: wrapArgs("import", cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("import", err)
				}
			}()
			// Handle --from-briefs flag
			if fromBriefs != "" {
				return importFromBriefs(cmd, fromBriefs, storyID, dryRun)
			}

			// Handle JSON file import (original behavior, still TODO)
			if len(args) == 0 {
				return fmt.Errorf("specify a file to import or use --from-briefs <spec-id>")
			}

			fmt.Fprintln(cmd.OutOrStdout(), "JSON import not implemented yet")
			return nil
		},
	}

	cmd.Flags().StringVar(&fromBriefs, "from-briefs", "", "Import from Gurgeh briefs for spec ID")
	cmd.Flags().StringVar(&storyID, "story", "", "Story ID to attach tasks to")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview import without making changes")

	return cmd
}

func importFromBriefs(cmd *cobra.Command, specID, storyID string, dryRun bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	root := findRootWithBriefs(cwd, specID)
	result, err := prd.ImportFromBriefs(prd.BriefImportOptions{
		Root:    root,
		SpecID:  specID,
		StoryID: storyID,
	})
	if err != nil {
		return err
	}

	// Print warnings
	for _, w := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", w)
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "\n📋 Would import %d tasks from briefs:\n\n", len(result.Tasks))
		for _, t := range result.Tasks {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", t.ID, t.Title)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nStory: %s\n", result.StoryID)
		fmt.Fprintln(cmd.OutOrStdout(), "\nRun without --dry-run to import.")
		return nil
	}

	// Ensure Coldwine project + DB exist before persisting.
	if err := project.Init(root); err != nil {
		return fmt.Errorf("failed to initialize coldwine project: %w", err)
	}
	db, err := storage.Open(project.StateDBPath(root))
	if err != nil {
		return fmt.Errorf("failed to open state db: %w", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		return fmt.Errorf("failed to migrate state db: %w", err)
	}

	specsDir := project.SpecsDir(root)
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create specs dir: %w", err)
	}
	for _, t := range result.Tasks {
		if err := upsertTask(db, t); err != nil {
			return fmt.Errorf("failed to persist task %s: %w", t.ID, err)
		}
		if err := writeTaskSpec(specsDir, t); err != nil {
			return fmt.Errorf("failed to write task spec %s: %w", t.ID, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Imported %d tasks from briefs:\n\n", len(result.Tasks))
	for _, t := range result.Tasks {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", t.ID, t.Title)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nAttached to story: %s\n", result.StoryID)
	fmt.Fprintf(cmd.OutOrStdout(), "Persisted to: %s\n", project.StateDBPath(root))
	fmt.Fprintf(cmd.OutOrStdout(), "Task specs written to: %s\n", specsDir)

	return nil
}

func upsertTask(db *sql.DB, t storage.WorkTask) error {
	_, err := db.Exec(
		`INSERT INTO tasks (id, title, status)
		 VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET title = excluded.title, status = excluded.status`,
		t.ID, t.Title, string(t.Status),
	)
	return err
}

func writeTaskSpec(specsDir string, t storage.WorkTask) error {
	payload := map[string]any{
		"id":          t.ID,
		"title":       t.Title,
		"description": t.Description,
		"status":      string(t.Status),
		"story_id":    t.StoryID,
		"priority":    t.Priority,
	}
	if t.Assignee != "" {
		payload["assignee"] = t.Assignee
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}
	path, err := project.SafePath(specsDir, t.ID+".yaml")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func findRootWithBriefs(start, specID string) string {
	cur := start
	for {
		candidate := filepath.Join(cur, ".gurgeh", "briefs", specID)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return start
		}
		cur = parent
	}
}
