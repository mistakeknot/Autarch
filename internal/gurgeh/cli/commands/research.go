package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
	"github.com/mistakeknot/autarch/internal/gurgeh/brief"
	"github.com/mistakeknot/autarch/internal/gurgeh/config"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/research"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/pollard/insights"
	"github.com/spf13/cobra"
)

func ResearchCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "research <id>",
		Short: "Create a research artifact for a PRD",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadFromRoot(root)
			if err != nil {
				return err
			}
			profile, err := agents.Resolve(agentProfiles(cfg), agent)
			if err != nil {
				return err
			}
			id := args[0]
			now := time.Now()
			researchDir := project.ResearchDir(root)
			if err := os.MkdirAll(researchDir, 0o755); err != nil {
				return err
			}
			path, err := research.Create(researchDir, id, now)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), filepath.Base(path))
			briefPath, err := writeResearchBrief(root, id, path, now)
			if err != nil {
				return err
			}
			launcher := launchAgent
			if isClaudeProfile(agent, profile) {
				launcher = launchSubagent
			}
			if err := launcher(profile, briefPath); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "agent not found; brief at %s\n", briefPath)
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "codex", "Agent profile to use")
	return cmd
}

func writeResearchBrief(root, id, researchPath string, now time.Time) (string, error) {
	briefsDir := project.BriefsDir(root)
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102-150405")
	briefPath := filepath.Join(briefsDir, id+"-"+stamp+".md")
	specPath := filepath.Join(project.SpecsDir(root), id+".yaml")
	spec, err := specs.LoadSpec(specPath)
	if err != nil {
		return "", err
	}
	acceptance := []string{}
	for _, item := range spec.Acceptance {
		if strings.TrimSpace(item.Description) != "" {
			acceptance = append(acceptance, item.Description)
		}
	}
	pollardFindings := loadPollardFindings(root, spec.ID)
	content := buildResearchBrief(spec, researchPath, acceptance, pollardFindings)
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return briefPath, nil
}

func buildResearchBrief(spec specs.Spec, researchPath string, acceptance []string, pollardFindings string) string {
	base := brief.Compose(brief.Input{
		ID:            spec.ID,
		Title:         spec.Title,
		Summary:       spec.Summary,
		Requirements:  spec.Requirements,
		Acceptance:    acceptance,
		ResearchFiles: spec.Research,
	})
	instructions := "\n\nInstructions:\n" +
		"- Fill in market research and competitive landscape sections.\n" +
		"- Include an OSS project scan with evidence refs.\n" +
		"- Use evidence refs for all claims.\n" +
		"- Write results into the research template at:\n  " + researchPath + "\n"
	if pollardFindings != "" {
		instructions += "\n## Existing Pollard Research\n\n" +
			"The following insights are already available from Pollard. " +
			"Use these as a starting point and cite them where relevant:\n\n" +
			pollardFindings
	}
	return base + instructions
}

// loadPollardFindings loads Pollard insights relevant to a spec ID.
func loadPollardFindings(root, specID string) string {
	allInsights, err := insights.LoadAll(root)
	if err != nil || len(allInsights) == 0 {
		return ""
	}
	var linked, general []string
	for _, ins := range allInsights {
		summary := ins.Title
		if len(ins.Findings) > 0 {
			summary += ": " + ins.Findings[0].Description
		}
		isLinked := false
		for _, feat := range ins.LinkedFeatures {
			if feat == specID {
				isLinked = true
				break
			}
		}
		if isLinked {
			linked = append(linked, "- [linked] "+summary)
		} else {
			general = append(general, "- "+summary)
		}
	}
	var b strings.Builder
	for _, l := range linked {
		b.WriteString(l + "\n")
	}
	limit := 10
	if len(general) < limit {
		limit = len(general)
	}
	for _, g := range general[:limit] {
		b.WriteString(g + "\n")
	}
	return b.String()
}
