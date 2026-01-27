package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// CreateCmd creates a new PRD with optional interactive interview.
func CreateCmd() *cobra.Command {
	var interactive bool
	var title string
	var summary string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new PRD",
		Long: `Create a new PRD specification.

Use --interactive to launch the interview workflow.
Use --title and --summary for quick creation without interview.

Examples:
  gurgeh create --interactive
  gurgeh create --title "User Authentication" --summary "Add login/logout functionality"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}

			if interactive {
				// Delegate to interview command
				interviewCmd := InterviewCmd()
				return interviewCmd.RunE(cmd, args)
			}

			// Quick creation with title/summary
			if title == "" {
				return fmt.Errorf("--title is required for non-interactive creation")
			}

			// Generate PRD ID
			id := generatePRDID(cwd)

			spec := &specs.Spec{
				ID:        id,
				Title:     title,
				Summary:   summary,
				Status:    "draft",
				CreatedAt: time.Now().Format(time.RFC3339),
			}

			// Write the spec
			specsDir := filepath.Join(cwd, ".gurgeh", "specs")
			if err := os.MkdirAll(specsDir, 0755); err != nil {
				return fmt.Errorf("failed to create specs directory: %w", err)
			}

			specPath := filepath.Join(specsDir, id+".yaml")
			data, err := yaml.Marshal(spec)
			if err != nil {
				return fmt.Errorf("failed to serialize spec: %w", err)
			}

			if err := os.WriteFile(specPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write spec: %w", err)
			}

			fmt.Printf("Created PRD: %s\n", id)
			fmt.Printf("  Title: %s\n", title)
			fmt.Printf("  Status: draft\n")
			fmt.Printf("  Path: %s\n", specPath)
			fmt.Println("\nNext steps:")
			fmt.Printf("  gurgeh edit %s    # Add more details\n", id)
			fmt.Printf("  gurgeh validate %s # Check completeness\n", id)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Launch interactive interview")
	cmd.Flags().StringVarP(&title, "title", "t", "", "PRD title")
	cmd.Flags().StringVarP(&summary, "summary", "s", "", "PRD summary")

	return cmd
}

func generatePRDID(cwd string) string {
	specsDir := filepath.Join(cwd, ".gurgeh", "specs")
	entries, _ := os.ReadDir(specsDir)

	maxNum := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "PRD-") && strings.HasSuffix(name, ".yaml") {
			numStr := strings.TrimPrefix(strings.TrimSuffix(name, ".yaml"), "PRD-")
			var num int
			fmt.Sscanf(numStr, "%d", &num)
			if num > maxNum {
				maxNum = num
			}
		}
	}

	return fmt.Sprintf("PRD-%03d", maxNum+1)
}
