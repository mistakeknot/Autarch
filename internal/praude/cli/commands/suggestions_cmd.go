package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
	"github.com/spf13/cobra"
)

func SuggestionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggestions",
		Short: "Review or apply suggestion files",
	}
	cmd.AddCommand(suggestionsReviewCmd())
	cmd.AddCommand(suggestionsApplyCmd())
	return cmd
}

func suggestionsReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <id>",
		Short: "Review latest suggestions for a PRD",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			id := args[0]
			sugg, path, err := suggestions.LoadLatest(project.SuggestionsDir(root), id)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "File: %s\n", filepath.Base(path))
			fmt.Fprintf(out, "Summary: %s\n", yesNo(sugg.Summary != ""))
			fmt.Fprintf(out, "Requirements: %d\n", len(sugg.Requirements))
			fmt.Fprintf(out, "CUJ: %d\n", len(sugg.CriticalUserJourneys))
			fmt.Fprintf(out, "Market: %d\n", len(sugg.MarketResearch))
			fmt.Fprintf(out, "Competitive: %d\n", len(sugg.CompetitiveLandscape))
			return nil
		},
	}
}

func suggestionsApplyCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "apply <id>",
		Short: "Apply suggestions to a PRD",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			id := args[0]
			sugg, _, err := suggestions.LoadLatest(project.SuggestionsDir(root), id)
			if err != nil {
				return err
			}
			selected, err := selectSuggestions(cmd, sugg, all)
			if err != nil {
				return err
			}
			specPath := filepath.Join(project.SpecsDir(root), id+".yaml")
			if err := suggestions.Apply(specPath, selected); err != nil {
				return err
			}
			if err := git.EnsureRepo(root); err == nil {
				_ = git.CommitFiles(root, []string{specPath}, "chore(praude): apply suggestions "+id)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Applied suggestions to %s\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Apply all suggestions without prompts")
	return cmd
}

func selectSuggestions(cmd *cobra.Command, sugg suggestions.Suggestion, all bool) (suggestions.Suggestion, error) {
	if all {
		return suggestions.Suggestion{
			Summary:              sugg.Summary,
			Requirements:         sugg.Requirements,
			CriticalUserJourneys: sugg.CriticalUserJourneys,
			MarketResearch:       sugg.MarketResearch,
			CompetitiveLandscape: sugg.CompetitiveLandscape,
		}, nil
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	selected := suggestions.Suggestion{}
	if strings.TrimSpace(sugg.Summary) != "" {
		ok, err := promptYesNo(reader, out, "Apply summary? (y/n) ")
		if err != nil {
			return selected, err
		}
		if ok {
			selected.Summary = sugg.Summary
		}
	}
	if len(sugg.Requirements) > 0 {
		ok, err := promptYesNo(reader, out, "Apply requirements? (y/n) ")
		if err != nil {
			return selected, err
		}
		if ok {
			selected.Requirements = sugg.Requirements
		}
	}
	if len(sugg.CriticalUserJourneys) > 0 {
		ok, err := promptYesNo(reader, out, "Apply CUJs? (y/n) ")
		if err != nil {
			return selected, err
		}
		if ok {
			selected.CriticalUserJourneys = sugg.CriticalUserJourneys
		}
	}
	if len(sugg.MarketResearch) > 0 {
		ok, err := promptYesNo(reader, out, "Apply market research? (y/n) ")
		if err != nil {
			return selected, err
		}
		if ok {
			selected.MarketResearch = sugg.MarketResearch
		}
	}
	if len(sugg.CompetitiveLandscape) > 0 {
		ok, err := promptYesNo(reader, out, "Apply competitive landscape? (y/n) ")
		if err != nil {
			return selected, err
		}
		if ok {
			selected.CompetitiveLandscape = sugg.CompetitiveLandscape
		}
	}
	return selected, nil
}

func yesNo(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}
