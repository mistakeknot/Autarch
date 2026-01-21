package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tmux"
	"github.com/spf13/cobra"
)

type statusSummary struct {
	ProjectRoot  string
	Initialized  bool
	DBExists     bool
	TaskCounts   map[string]int
	Sessions     []string
	SpecCount    int
	SpecWarnings int
}

func statusSummaryFromCwd() statusSummary {
	cwd, _ := os.Getwd()
	root, err := project.FindRoot(cwd)
	if err != nil {
		return statusSummary{Initialized: false}
	}
	sum := statusSummary{ProjectRoot: root, Initialized: true}
	if st, err := os.Stat(project.StateDBPath(root)); err == nil && !st.IsDir() {
		sum.DBExists = true
		if db, err := storage.Open(project.StateDBPath(root)); err == nil {
			if ok, _ := storage.HasTasksTable(db); ok {
				if counts, err := storage.CountTasksByStatus(db); err == nil {
					sum.TaskCounts = counts
				}
			}
			db.Close()
		}
	}
	if len(sum.TaskCounts) == 0 {
		if summaries, warnings := specs.LoadSummaries(project.SpecsDir(root)); len(summaries) > 0 {
			sum.TaskCounts = summariesToCounts(summaries)
			sum.SpecCount = len(summaries)
			sum.SpecWarnings = len(warnings)
		}
	}
	sum.Sessions, _ = tmux.ListSessions("tand-")
	return sum
}

func summariesToCounts(summaries []specs.SpecSummary) map[string]int {
	counts := make(map[string]int)
	for _, s := range summaries {
		status := s.Status
		if status == "" {
			status = "unknown"
		}
		counts[status]++
	}
	return counts
}

func formatStatusLines(sum statusSummary) []string {
	if !sum.Initialized {
		return []string{"Not initialized (.tandemonium not found)"}
	}
	lines := []string{fmt.Sprintf("Project: %s", sum.ProjectRoot)}
	lines = append(lines, fmt.Sprintf("state.db: %v", sum.DBExists))
	if len(sum.TaskCounts) > 0 {
		lines = append(lines, fmt.Sprintf("tasks: %v", sum.TaskCounts))
	} else {
		lines = append(lines, "tasks: (none)")
	}
	if sum.SpecCount > 0 {
		lines = append(lines, fmt.Sprintf("specs: %d (warnings: %d)", sum.SpecCount, sum.SpecWarnings))
	}
	lines = append(lines, fmt.Sprintf("tmux sessions: %d", len(sum.Sessions)))
	return lines
}

func StatusCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show quick project status",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("status", err)
				}
			}()
			sum := statusSummaryFromCwd()
			if !sum.Initialized {
				return fmt.Errorf("not a Tandemonium project")
			}
			if jsonOut {
				payload := map[string]interface{}{
					"project_root":  sum.ProjectRoot,
					"initialized":   sum.Initialized,
					"db_exists":     sum.DBExists,
					"task_counts":   sum.TaskCounts,
					"sessions":      sum.Sessions,
					"spec_count":    sum.SpecCount,
					"spec_warnings": sum.SpecWarnings,
				}
				return writeJSON(cmd, payload)
			}
			for _, line := range formatStatusLines(sum) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}
