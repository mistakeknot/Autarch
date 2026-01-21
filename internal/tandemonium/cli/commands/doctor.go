package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/drift"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
	"github.com/spf13/cobra"
)

type doctorSummary struct {
	Initialized  bool
	DBQuickCheck string
	Sessions     []string
	SpecWarnings []string
	DriftFiles   []string
}

func doctorSummaryFromCwd() doctorSummary {
	cwd, _ := os.Getwd()
	root, err := project.FindRoot(cwd)
	if err != nil {
		return doctorSummary{Initialized: false}
	}
	sum := doctorSummary{Initialized: true}
	if db, err := storage.Open(project.StateDBPath(root)); err == nil {
		if res, err := storage.QuickCheck(db); err == nil {
			sum.DBQuickCheck = res
		}
		db.Close()
	}
	summaries, warnings := specs.LoadSummaries(project.SpecsDir(root))
	sum.SpecWarnings = warnings
	allowed := collectAllowedFiles(summaries)
	if len(allowed) > 0 {
		files, err := git.DiffNameOnly(&git.ExecRunner{}, "HEAD")
		if err == nil {
			sum.DriftFiles = drift.DetectDrift(allowed, files)
		}
	}
	sum.Sessions, _ = tmux.ListSessions("tand-")
	return sum
}

func collectAllowedFiles(list []specs.SpecSummary) []string {
	seen := make(map[string]struct{})
	for _, s := range list {
		for _, f := range s.FilesToModify {
			seen[f] = struct{}{}
		}
	}
	var files []string
	for f := range seen {
		files = append(files, f)
	}
	return files
}

func formatDoctorLines(sum doctorSummary) []string {
	if !sum.Initialized {
		return []string{"Not initialized (.tandemonium not found)"}
	}
	lines := []string{"Doctor checks:"}
	if sum.DBQuickCheck != "" {
		lines = append(lines, fmt.Sprintf("sqlite quick_check: %s", sum.DBQuickCheck))
	} else {
		lines = append(lines, "sqlite quick_check: (skipped)")
	}
	if len(sum.SpecWarnings) > 0 {
		lines = append(lines, fmt.Sprintf("spec warnings: %d", len(sum.SpecWarnings)))
	}
	if len(sum.DriftFiles) > 0 {
		lines = append(lines, fmt.Sprintf("drift warnings: %d", len(sum.DriftFiles)))
	}
	lines = append(lines, fmt.Sprintf("tmux sessions: %d", len(sum.Sessions)))
	return lines
}

func DoctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run integrity checks",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("doctor", err)
				}
			}()
			sum := doctorSummaryFromCwd()
			if !sum.Initialized {
				return fmt.Errorf("not a Tandemonium project")
			}
			if jsonOut {
				payload := map[string]interface{}{
					"initialized":    sum.Initialized,
					"db_quick_check": sum.DBQuickCheck,
					"sessions":       sum.Sessions,
					"spec_warnings":  sum.SpecWarnings,
					"drift_files":    sum.DriftFiles,
					"checks":         formatDoctorLines(sum),
				}
				return writeJSON(cmd, payload)
			}
			for _, line := range formatDoctorLines(sum) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}
