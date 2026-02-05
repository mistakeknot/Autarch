package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/brief"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/spf13/cobra"
)

func ExportCmd() *cobra.Command {
	var format string
	var timeout time.Duration
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "export <spec-id>",
		Short: "Export a spec to different formats",
		Long: `Export a completed spec to various formats.

Currently supported formats:
  briefs - Decompose into atomic implementation briefs via Claude Code

Example:
  gurgeh export PRD-001 --format briefs
  gurgeh export PRD-001 --format briefs --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			// Load spec
			specPath := filepath.Join(project.SpecsDir(root), specID+".yaml")
			spec, err := specs.LoadSpec(specPath)
			if err != nil {
				return fmt.Errorf("failed to load spec %s: %w", specID, err)
			}

			switch format {
			case "briefs":
				return exportBriefs(cmd, &spec, root, timeout, dryRun)
			default:
				return fmt.Errorf("unsupported format: %s (supported: briefs)", format)
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", "briefs", "Export format (briefs)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Claude Code timeout")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview briefs without saving")

	return cmd
}

func exportBriefs(cmd *cobra.Command, spec *specs.Spec, root string, timeout time.Duration, dryRun bool) error {
	fmt.Fprintf(cmd.OutOrStdout(), "🔍 Decomposing spec %s into briefs...\n", spec.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "⏱️  Timeout: %s\n", timeout)
	fmt.Fprintln(cmd.OutOrStdout(), "⏳ This may take a few minutes...")
	fmt.Fprintln(cmd.OutOrStdout())

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	briefs, err := brief.Decompose(ctx, spec, root)
	elapsed := time.Since(start)

	if err != nil {
		return fmt.Errorf("decomposition failed: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✅ Generated %d briefs in %s\n\n", len(briefs), elapsed.Round(time.Second))

	// Print briefs
	for i, b := range briefs {
		fmt.Fprintf(cmd.OutOrStdout(), "📋 BRIEF-%03d: %s\n", i+1, b.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "   Outcome: %s\n", truncateStr(b.Outcome, 80))
		fmt.Fprintf(cmd.OutOrStdout(), "   Criteria: %d items\n", len(b.Criteria))
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "🔸 Dry run - briefs not saved")
		return nil
	}

	// Save briefs
	if err := brief.SaveBriefs(spec.ID, briefs); err != nil {
		return fmt.Errorf("failed to save briefs: %w", err)
	}

	briefsDir := filepath.Join(".gurgeh", "briefs", spec.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "💾 Briefs saved to: %s/\n", briefsDir)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "To execute the first brief:")
	fmt.Fprintf(cmd.OutOrStdout(), "  claude -p \"$(cat %s/BRIEF-001-*.md)\"\n", briefsDir)

	return nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
