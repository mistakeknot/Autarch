package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/review"
	"github.com/mistakeknot/autarch/internal/gurgeh/spec"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ReviewCmd runs multi-agent PRD review.
func ReviewCmd() *cobra.Command {
	var verbose bool
	var includeGaps bool

	cmd := &cobra.Command{
		Use:   "review <PRD-ID>",
		Short: "Run multi-agent PRD quality review",
		Long: `Run parallel quality reviewers on a PRD to identify issues.

Reviewers:
  - completeness: Checks for required sections
  - cuj-consistency: Validates CUJ structure and cross-references
  - acceptance-criteria: Checks criteria are measurable
  - scope-creep: Identifies overly broad requirements

Use --gaps to also run the SpecFlow gap analyzer.

Examples:
  gurgeh review PRD-001
  gurgeh review PRD-001 --gaps
  gurgeh review PRD-001 --verbose`,
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

			// Read the PRD
			specPath := filepath.Join(cwd, ".gurgeh", "specs", prdID+".yaml")
			data, err := os.ReadFile(specPath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("PRD not found: %s", prdID)
				}
				return fmt.Errorf("failed to read PRD: %w", err)
			}

			var prdSpec specs.Spec
			if err := yaml.Unmarshal(data, &prdSpec); err != nil {
				return fmt.Errorf("failed to parse PRD: %w", err)
			}

			// Run parallel review
			fmt.Printf("Reviewing PRD: %s\n", prdID)
			fmt.Println("Running reviewers...")

			reviewers := review.DefaultReviewers()
			result, err := review.RunParallelReview(context.Background(), &prdSpec, reviewers)
			if err != nil {
				return fmt.Errorf("review failed: %w", err)
			}

			// Print results
			fmt.Println()
			fmt.Printf("Overall Score: %.0f%%\n", result.OverallScore*100)
			fmt.Printf("Total Issues: %d (Errors: %d, Warnings: %d)\n",
				result.TotalIssues, result.Errors, result.Warnings)

			if result.ReadyForImplementation {
				fmt.Println("Status: ✓ Ready for implementation")
			} else {
				fmt.Println("Status: ✗ Not ready - address issues first")
			}

			// Print detailed results
			if verbose || result.TotalIssues > 0 {
				fmt.Println("\n--- Reviewer Results ---")
				for _, r := range result.Results {
					fmt.Printf("\n[%s] Score: %.0f%%\n", r.Reviewer, r.Score*100)

					for _, issue := range r.Issues {
						icon := "•"
						if issue.Severity == review.SeverityError {
							icon = "✗"
						} else if issue.Severity == review.SeverityWarning {
							icon = "!"
						}
						fmt.Printf("  %s [%s] %s\n", icon, issue.Severity, issue.Description)
						if verbose && issue.Location != "" {
							fmt.Printf("    Location: %s\n", issue.Location)
						}
					}

					if verbose {
						for _, suggestion := range r.Suggestions {
							fmt.Printf("  → %s\n", suggestion)
						}
					}
				}
			}

			// Run SpecFlow gap analysis if requested
			if includeGaps {
				fmt.Println("\n--- SpecFlow Gap Analysis ---")
				analyzer := spec.NewSpecFlowAnalyzer()
				gapResult := analyzer.Analyze(&prdSpec)

				fmt.Printf("Coverage: %.0f%%\n", gapResult.Coverage*100)
				fmt.Printf("Gaps: %d (Blockers: %d, Warnings: %d)\n",
					len(gapResult.Gaps), gapResult.Blockers, gapResult.Warnings)

				if len(gapResult.Gaps) > 0 {
					for _, gap := range gapResult.Gaps {
						icon := "→"
						if gap.Severity == spec.SeverityBlocker {
							icon = "✗"
						} else if gap.Severity == spec.SeverityWarning {
							icon = "!"
						}
						fmt.Printf("  %s [%s] %s\n", icon, gap.Category, gap.Description)
						if verbose && gap.Suggestion != "" {
							fmt.Printf("    Suggestion: %s\n", gap.Suggestion)
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output")
	cmd.Flags().BoolVarP(&includeGaps, "gaps", "g", false, "Include SpecFlow gap analysis")

	return cmd
}
