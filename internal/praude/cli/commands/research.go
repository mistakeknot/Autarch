package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/brief"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/research"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
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
	content := buildResearchBrief(spec, researchPath, acceptance)
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return briefPath, nil
}

func buildResearchBrief(spec specs.Spec, researchPath string, acceptance []string) string {
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
	return base + instructions
}
