package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
)

var linkFeature string

var linkCmd = &cobra.Command{
	Use:   "link [insight-id]",
	Short: "Link an insight to a Praude feature",
	Long:  `Link a research insight to a Praude feature for cross-referencing.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		insightID := args[0]
		if linkFeature == "" {
			return fmt.Errorf("--feature flag is required")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Find and load the insight
		insightPath := filepath.Join(cwd, ".pollard", "insights", insightID+".yaml")
		insight, err := insights.Load(insightPath)
		if err != nil {
			return fmt.Errorf("failed to load insight %s: %w", insightID, err)
		}

		// Check if already linked
		for _, f := range insight.LinkedFeatures {
			if f == linkFeature {
				fmt.Printf("Insight %s is already linked to %s\n", insightID, linkFeature)
				return nil
			}
		}

		// Add the link
		insight.LinkedFeatures = append(insight.LinkedFeatures, linkFeature)

		// Save
		if err := insight.Save(cwd); err != nil {
			return fmt.Errorf("failed to save insight: %w", err)
		}

		fmt.Printf("Linked %s to feature %s\n", insightID, linkFeature)
		return nil
	},
}

func init() {
	linkCmd.Flags().StringVar(&linkFeature, "feature", "", "Feature ID to link (e.g., FEAT-001)")
	linkCmd.MarkFlagRequired("feature")
}
