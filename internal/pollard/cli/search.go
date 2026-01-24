package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/patterns"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search collected patterns and insights",
	Long:  `Search for patterns and insights matching the given query string.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Search patterns
		allPatterns, err := patterns.LoadAll(cwd)
		if err != nil {
			return fmt.Errorf("failed to load patterns: %w", err)
		}

		matchedPatterns := patterns.Search(allPatterns, query)

		// Search insights
		allInsights, err := insights.LoadAll(cwd)
		if err != nil {
			return fmt.Errorf("failed to load insights: %w", err)
		}

		matchedInsights := searchInsights(allInsights, query)

		// Display results
		fmt.Printf("Search results for: %q\n\n", query)

		if len(matchedPatterns) > 0 {
			fmt.Printf("## Patterns (%d matches)\n\n", len(matchedPatterns))
			for _, p := range matchedPatterns {
				fmt.Printf("- **%s** [%s]: %s\n", p.Title, p.ID, truncate(p.Description, 80))
			}
			fmt.Println()
		}

		if len(matchedInsights) > 0 {
			fmt.Printf("## Insights (%d matches)\n\n", len(matchedInsights))
			for _, i := range matchedInsights {
				fmt.Printf("- **%s** [%s] (%s)\n", i.Title, i.ID, i.Category)
			}
			fmt.Println()
		}

		if len(matchedPatterns) == 0 && len(matchedInsights) == 0 {
			fmt.Println("No results found.")
		}

		return nil
	},
}

func searchInsights(items []*insights.Insight, query string) []*insights.Insight {
	if query == "" {
		return items
	}
	queryLower := strings.ToLower(query)
	var matches []*insights.Insight
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Title), queryLower) {
			matches = append(matches, item)
			continue
		}
		// Search in findings
		for _, f := range item.Findings {
			if strings.Contains(strings.ToLower(f.Title), queryLower) ||
				strings.Contains(strings.ToLower(f.Description), queryLower) {
				matches = append(matches, item)
				break
			}
		}
	}
	return matches
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
