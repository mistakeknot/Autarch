package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/insights"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/patterns"
)

var exportFormat string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export research data for other tools",
	Long:  `Export research insights and patterns in a format suitable for Praude or Tandemonium.`,
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

		switch exportFormat {
		case "praude":
			return exportForPraude(allInsights, allPatterns)
		case "tandemonium":
			return exportForTandemonium(allPatterns)
		default:
			return exportForPraude(allInsights, allPatterns)
		}
	},
}

// PraudeContext is the export format for Praude
type PraudeContext struct {
	ResearchSummary  string               `yaml:"research_summary"`
	KeyCompetitors   []string             `yaml:"key_competitors"`
	Recommendations  []RecommendationItem `yaml:"recommendations"`
	LinkedInsights   []LinkedInsight      `yaml:"linked_insights"`
}

type RecommendationItem struct {
	FeatureHint string `yaml:"feature_hint"`
	Priority    string `yaml:"priority"`
	Rationale   string `yaml:"rationale"`
	SourceID    string `yaml:"source_id"`
}

type LinkedInsight struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Category string   `yaml:"category"`
	Features []string `yaml:"linked_features,omitempty"`
}

func exportForPraude(allInsights []*insights.Insight, allPatterns []*patterns.Pattern) error {
	ctx := PraudeContext{
		ResearchSummary: fmt.Sprintf("Based on %d insights and %d patterns", len(allInsights), len(allPatterns)),
	}

	// Collect competitors from competitive insights
	competitorSet := make(map[string]bool)
	for _, i := range insights.FilterByCategory(allInsights, insights.CategoryCompetitive) {
		for _, s := range i.Sources {
			if s.Type == "product" {
				competitorSet[s.URL] = true
			}
		}
	}
	for c := range competitorSet {
		ctx.KeyCompetitors = append(ctx.KeyCompetitors, c)
	}

	// Collect recommendations
	for _, i := range allInsights {
		for _, r := range i.Recommendations {
			ctx.Recommendations = append(ctx.Recommendations, RecommendationItem{
				FeatureHint: r.FeatureHint,
				Priority:    r.Priority,
				Rationale:   r.Rationale,
				SourceID:    i.ID,
			})
		}
	}

	// Collect linked insights
	for _, i := range allInsights {
		ctx.LinkedInsights = append(ctx.LinkedInsights, LinkedInsight{
			ID:       i.ID,
			Title:    i.Title,
			Category: string(i.Category),
			Features: i.LinkedFeatures,
		})
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

// TandemoniumContext is the export format for Tandemonium
type TandemoniumContext struct {
	ImplementationHints []ImplementationHint `yaml:"implementation_hints"`
	AntiPatterns        []AntiPattern        `yaml:"anti_patterns"`
}

type ImplementationHint struct {
	PatternID string   `yaml:"pattern_id"`
	Title     string   `yaml:"title"`
	Category  string   `yaml:"category"`
	Hints     []string `yaml:"hints"`
	Examples  []string `yaml:"examples"`
}

type AntiPattern struct {
	PatternID   string `yaml:"pattern_id"`
	Description string `yaml:"description"`
}

func exportForTandemonium(allPatterns []*patterns.Pattern) error {
	ctx := TandemoniumContext{}

	for _, p := range allPatterns {
		if p.Category == patterns.CategoryAnti {
			for _, ap := range p.AntiPatterns {
				ctx.AntiPatterns = append(ctx.AntiPatterns, AntiPattern{
					PatternID:   p.ID,
					Description: ap,
				})
			}
		} else {
			var examples []string
			for _, e := range p.Examples {
				examples = append(examples, e.Name)
			}
			ctx.ImplementationHints = append(ctx.ImplementationHints, ImplementationHint{
				PatternID: p.ID,
				Title:     p.Title,
				Category:  string(p.Category),
				Hints:     p.ImplementationHints,
				Examples:  examples,
			})
		}
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "praude", "Export format: praude, tandemonium")
}
