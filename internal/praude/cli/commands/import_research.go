package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/pkg/discovery"
	"github.com/spf13/cobra"
)

// ImportResearchCmd creates the import-research command.
func ImportResearchCmd() *cobra.Command {
	var (
		specID   string
		fromTool string
		highOnly bool
	)

	cmd := &cobra.Command{
		Use:   "import-research",
		Short: "Import research from Pollard into PRD context",
		Long: `Import research insights from Pollard into a research document for a PRD.

This creates a populated research template with competitive analysis,
market trends, and recommendations from Pollard's collected insights.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			// Validate source
			if fromTool != "pollard" {
				return fmt.Errorf("unsupported source tool: %s (only 'pollard' is supported)", fromTool)
			}

			// Check if Pollard has data
			if !discovery.PollardHasData(root) {
				return fmt.Errorf("no Pollard data found in %s - run 'pollard scan' first", root)
			}

			// Load insights
			insights, err := discovery.PollardInsights(root)
			if err != nil {
				return fmt.Errorf("failed to load Pollard insights: %w", err)
			}

			if len(insights) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No insights found in Pollard data.")
				return nil
			}

			// Filter to high relevance only if requested
			if highOnly {
				insights = filterHighRelevance(insights)
				if len(insights) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No high-relevance insights found. Try without --high-only.")
					return nil
				}
			}

			// Ensure research directory exists
			researchDir := project.ResearchDir(root)
			if err := os.MkdirAll(researchDir, 0o755); err != nil {
				return fmt.Errorf("failed to create research directory: %w", err)
			}

			// Generate research document
			content := generateResearchFromPollard(specID, insights)

			// Write to file
			now := time.Now()
			filename := fmt.Sprintf("%s-pollard-%s.md", specID, now.Format("20060102-150405"))
			outputPath := filepath.Join(researchDir, filename)

			if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("failed to write research file: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created research document: %s\n", outputPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d insights from Pollard\n", len(insights))

			return nil
		},
	}

	cmd.Flags().StringVar(&specID, "spec", "PRD-XXX", "Spec ID to associate with the research")
	cmd.Flags().StringVar(&fromTool, "from-pollard", "pollard", "Source tool for research data")
	cmd.Flags().BoolVar(&highOnly, "high-only", false, "Only import high-relevance insights")

	// Make --from-pollard a boolean-style flag that sets fromTool
	cmd.Flags().Lookup("from-pollard").NoOptDefVal = "pollard"

	return cmd
}

// filterHighRelevance filters insights to only those with high relevance.
func filterHighRelevance(insights []discovery.PollardInsight) []discovery.PollardInsight {
	var result []discovery.PollardInsight
	for _, i := range insights {
		for _, f := range i.Findings {
			if f.Relevance == "high" {
				result = append(result, i)
				break
			}
		}
	}
	return result
}

// generateResearchFromPollard creates a research markdown from Pollard insights.
func generateResearchFromPollard(specID string, insights []discovery.PollardInsight) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Research for %s\n\n", specID))
	sb.WriteString(fmt.Sprintf("*Auto-generated from Pollard on %s*\n\n", time.Now().Format("2006-01-02")))

	// Categorize insights
	competitive := filterByCategory(insights, "competitive")
	trends := filterByCategory(insights, "trends")
	user := filterByCategory(insights, "user")

	// Competitive Analysis
	sb.WriteString("## Competitive Analysis\n\n")
	if len(competitive) > 0 {
		for _, insight := range competitive {
			sb.WriteString(fmt.Sprintf("### %s\n\n", insight.Title))
			for _, f := range insight.Findings {
				relevance := ""
				if f.Relevance != "" {
					relevance = fmt.Sprintf(" [%s]", f.Relevance)
				}
				sb.WriteString(fmt.Sprintf("- **%s**%s\n", f.Title, relevance))
				if f.Description != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", f.Description))
				}
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("*No competitive insights available. Run `pollard scan --hunter competitor-tracker`.*\n\n")
	}

	// Market Trends
	sb.WriteString("## Market Trends\n\n")
	if len(trends) > 0 {
		for _, insight := range trends {
			sb.WriteString(fmt.Sprintf("### %s\n\n", insight.Title))
			for _, f := range insight.Findings {
				relevance := ""
				if f.Relevance != "" {
					relevance = fmt.Sprintf(" [%s]", f.Relevance)
				}
				sb.WriteString(fmt.Sprintf("- **%s**%s\n", f.Title, relevance))
				if f.Description != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", f.Description))
				}
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("*No trend insights available. Run `pollard scan --hunter hackernews-trendwatcher`.*\n\n")
	}

	// User Research
	sb.WriteString("## User Research\n\n")
	if len(user) > 0 {
		for _, insight := range user {
			sb.WriteString(fmt.Sprintf("### %s\n\n", insight.Title))
			for _, f := range insight.Findings {
				sb.WriteString(fmt.Sprintf("- %s\n", f.Title))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("*No user research insights available.*\n\n")
	}

	// Key Recommendations
	sb.WriteString("## Key Recommendations\n\n")
	hasRecommendations := false
	for _, insight := range insights {
		for _, r := range insight.Recommendations {
			hasRecommendations = true
			priority := ""
			if r.Priority != "" {
				priority = fmt.Sprintf(" [%s]", r.Priority)
			}
			sb.WriteString(fmt.Sprintf("- **%s**%s\n", r.FeatureHint, priority))
			if r.Rationale != "" {
				sb.WriteString(fmt.Sprintf("  *Rationale:* %s\n", r.Rationale))
			}
		}
	}
	if !hasRecommendations {
		sb.WriteString("*No recommendations extracted yet.*\n")
	}
	sb.WriteString("\n")

	// OSS Project Scan
	sb.WriteString("## OSS Project Scan\n\n")
	sb.WriteString("*See `.pollard/sources/github/` for collected repositories.*\n\n")
	sb.WriteString("| Project | Key Learnings | Bootstrap Potential |\n")
	sb.WriteString("|---------|---------------|---------------------|\n")
	sb.WriteString("| *Run `pollard scan --hunter github-scout` to populate* | | |\n\n")

	// Source Attribution
	sb.WriteString("---\n\n")
	sb.WriteString("## Sources\n\n")
	sb.WriteString("This research was imported from Pollard. Source files:\n\n")
	sb.WriteString("- `.pollard/insights/competitive/` - Competitor tracking\n")
	sb.WriteString("- `.pollard/insights/trends/` - HackerNews, arXiv trends\n")
	sb.WriteString("- `.pollard/sources/` - Raw collected data\n")

	return sb.String()
}

// filterByCategory filters insights by category.
func filterByCategory(insights []discovery.PollardInsight, category string) []discovery.PollardInsight {
	var result []discovery.PollardInsight
	for _, i := range insights {
		if i.Category == category {
			result = append(result, i)
		}
	}
	return result
}
