package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/patterns"
)

var reportType string

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a research report",
	Long:  `Generate a landscape report summarizing all collected insights and patterns.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		allInsights, err := insights.LoadAll(cwd)
		if err != nil {
			return fmt.Errorf("failed to load insights: %w", err)
		}

		allPatterns, err := patterns.LoadAll(cwd)
		if err != nil {
			return fmt.Errorf("failed to load patterns: %w", err)
		}

		switch reportType {
		case "landscape":
			return generateLandscapeReport(allInsights, allPatterns)
		case "competitive":
			competitive := insights.FilterByCategory(allInsights, insights.CategoryCompetitive)
			return generateCategoryReport("Competitive Analysis", competitive)
		case "trends":
			trends := insights.FilterByCategory(allInsights, insights.CategoryTrends)
			return generateCategoryReport("Industry Trends", trends)
		default:
			return generateLandscapeReport(allInsights, allPatterns)
		}
	},
}

func generateLandscapeReport(allInsights []*insights.Insight, allPatterns []*patterns.Pattern) error {
	fmt.Println("# Landscape Report")
	fmt.Println()
	fmt.Printf("Total Insights: %d\n", len(allInsights))
	fmt.Printf("Total Patterns: %d\n", len(allPatterns))
	fmt.Println()

	// Group by category
	competitive := insights.FilterByCategory(allInsights, insights.CategoryCompetitive)
	trends := insights.FilterByCategory(allInsights, insights.CategoryTrends)
	user := insights.FilterByCategory(allInsights, insights.CategoryUser)

	fmt.Printf("## Insights by Category\n")
	fmt.Printf("- Competitive: %d\n", len(competitive))
	fmt.Printf("- Trends: %d\n", len(trends))
	fmt.Printf("- User Research: %d\n", len(user))
	fmt.Println()

	ui := patterns.FilterByCategory(allPatterns, patterns.CategoryUI)
	arch := patterns.FilterByCategory(allPatterns, patterns.CategoryArch)
	anti := patterns.FilterByCategory(allPatterns, patterns.CategoryAnti)

	fmt.Printf("## Patterns by Category\n")
	fmt.Printf("- UI: %d\n", len(ui))
	fmt.Printf("- Architecture: %d\n", len(arch))
	fmt.Printf("- Anti-patterns: %d\n", len(anti))

	return nil
}

func generateCategoryReport(title string, items []*insights.Insight) error {
	fmt.Printf("# %s\n\n", title)
	fmt.Printf("Total: %d insights\n\n", len(items))

	for _, item := range items {
		fmt.Printf("## %s (%s)\n", item.Title, item.ID)
		fmt.Printf("Collected: %s\n", item.CollectedAt.Format("2006-01-02"))
		fmt.Println()

		if len(item.Findings) > 0 {
			fmt.Println("### Findings")
			for _, f := range item.Findings {
				fmt.Printf("- **%s** [%s]: %s\n", f.Title, f.Relevance, f.Description)
			}
			fmt.Println()
		}

		if len(item.Recommendations) > 0 {
			fmt.Println("### Recommendations")
			for _, r := range item.Recommendations {
				fmt.Printf("- [%s] %s: %s\n", r.Priority, r.FeatureHint, r.Rationale)
			}
			fmt.Println()
		}
	}

	return nil
}

func init() {
	reportCmd.Flags().StringVar(&reportType, "type", "landscape", "Report type: landscape, competitive, trends")
}
