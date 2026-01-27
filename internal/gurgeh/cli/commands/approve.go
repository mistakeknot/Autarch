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

// ApproveCmd marks a PRD as approved.
func ApproveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "approve <PRD-ID>",
		Short: "Mark a PRD as approved",
		Long: `Mark a PRD as approved and ready for implementation.

By default, validation is run before approval. Use --force to skip validation.

Examples:
  gurgeh approve PRD-001
  gurgeh approve PRD-001 --force`,
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

			var spec specs.Spec
			if err := yaml.Unmarshal(data, &spec); err != nil {
				return fmt.Errorf("failed to parse PRD: %w", err)
			}

			// Check current status
			if spec.Status == "approved" {
				fmt.Printf("PRD %s is already approved\n", prdID)
				return nil
			}

			// Run validation unless forced
			if !force {
				issues := validateForApproval(&spec)
				if len(issues) > 0 {
					fmt.Printf("PRD %s has validation issues:\n", prdID)
					for _, issue := range issues {
						fmt.Printf("  - %s\n", issue)
					}
					fmt.Println("\nUse --force to approve anyway, or fix the issues first.")
					return fmt.Errorf("validation failed")
				}
			}

			// Update status
			spec.Status = "approved"
			spec.Metadata.ValidationWarnings = append(spec.Metadata.ValidationWarnings,
				fmt.Sprintf("Approved at %s", time.Now().Format(time.RFC3339)))

			// Write back
			newData, err := yaml.Marshal(&spec)
			if err != nil {
				return fmt.Errorf("failed to serialize PRD: %w", err)
			}

			if err := os.WriteFile(specPath, newData, 0644); err != nil {
				return fmt.Errorf("failed to write PRD: %w", err)
			}

			fmt.Printf("✓ PRD %s approved\n", prdID)
			fmt.Println("\nNext steps:")
			fmt.Printf("  coldwine epic create --prd %s  # Generate tasks\n", prdID)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip validation")

	return cmd
}

// validateForApproval checks if a PRD is ready for approval.
func validateForApproval(spec *specs.Spec) []string {
	var issues []string

	if strings.TrimSpace(spec.Title) == "" {
		issues = append(issues, "Missing title")
	}

	if strings.TrimSpace(spec.Summary) == "" {
		issues = append(issues, "Missing summary")
	}

	if len(spec.Requirements) == 0 {
		issues = append(issues, "No requirements defined")
	}

	if len(spec.Acceptance) == 0 {
		issues = append(issues, "No acceptance criteria defined")
	}

	if len(spec.CriticalUserJourneys) == 0 {
		issues = append(issues, "No Critical User Journeys defined")
	}

	return issues
}
