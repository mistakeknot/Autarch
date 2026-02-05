package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/internal/coldwine/prd"
	"github.com/spf13/cobra"
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
		RunE: func(cmd *cobra.Command, args []string) error {
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

	result, err := prd.ImportFromBriefs(prd.BriefImportOptions{
		Root:    cwd,
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

	// TODO: Actually persist to database
	// For now, print what would be imported
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Imported %d tasks from briefs:\n\n", len(result.Tasks))
	for _, t := range result.Tasks {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", t.ID, t.Title)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nAttached to story: %s\n", result.StoryID)

	// Note: Database persistence would require opening the DB and inserting.
	// For MVP, we just demonstrate the parsing works.
	fmt.Fprintln(cmd.OutOrStdout(), "\nNote: Database persistence coming soon. Tasks parsed but not yet saved.")

	return nil
}
