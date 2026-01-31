package planstatus

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// NewCommand builds the plan-status CLI command.
func NewCommand() *cobra.Command {
	var repoRoot string
	var intermuteRoot string
	var outputPath string

	cmd := &cobra.Command{
		Use:   "plan-status",
		Short: "Generate plan status report",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoRoot == "" {
				repoRoot = "."
			}
			if outputPath == "" {
				outputPath = filepath.Join(repoRoot, "docs", "plans", "STATUS.md")
			}
			opts := Options{
				RepoRoot:      repoRoot,
				IntermuteRoot: intermuteRoot,
				Now:           time.Now(),
			}
			changed, err := WriteReportToFile(opts, outputPath)
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", outputPath)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Up to date: %s\n", outputPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoRoot, "repo", ".", "Repository root to scan")
	cmd.Flags().StringVar(&intermuteRoot, "intermute", "", "Intermute repository root (optional)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output path for report (default: docs/plans/STATUS.md)")

	return cmd
}

// WriteReportToFile generates the report and writes it if changed.
func WriteReportToFile(opts Options, outputPath string) (bool, error) {
	report, err := GenerateReport(opts)
	if err != nil {
		return false, err
	}

	if existing, err := os.ReadFile(outputPath); err == nil {
		if string(existing) == report {
			return false, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(outputPath, []byte(report), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
