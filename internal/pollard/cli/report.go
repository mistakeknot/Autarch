package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/reports"
)

var (
	reportType   string
	reportStdout bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a research report",
	Long: `Generate a research report summarizing collected intelligence.

Report types:
  landscape   - Comprehensive overview of all collected data (default)
  competitive - Focus on competitor activity and threats
  trends      - Industry trends from HackerNews and other sources
  research    - Academic papers from arXiv and research sources

Examples:
  pollard report                    # Generate landscape report
  pollard report --type competitive # Generate competitive analysis
  pollard report --type trends      # Generate trends report
  pollard report --stdout           # Output to stdout instead of file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		generator := reports.NewGenerator(cwd)

		var rType reports.ReportType
		switch reportType {
		case "landscape":
			rType = reports.TypeLandscape
		case "competitive":
			rType = reports.TypeCompetitive
		case "trends":
			rType = reports.TypeTrends
		case "research":
			rType = reports.TypeResearch
		default:
			rType = reports.TypeLandscape
		}

		filePath, err := generator.Generate(rType)
		if err != nil {
			return fmt.Errorf("failed to generate report: %w", err)
		}

		if reportStdout {
			// Read and print the file
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read report: %w", err)
			}
			fmt.Print(string(content))
		} else {
			fmt.Printf("Report generated: %s\n", filePath)
		}

		return nil
	},
}

func init() {
	reportCmd.Flags().StringVar(&reportType, "type", "landscape", "Report type: landscape, competitive, trends, research")
	reportCmd.Flags().BoolVar(&reportStdout, "stdout", false, "Output report to stdout instead of file")
}
