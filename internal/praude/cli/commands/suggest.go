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
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
	"github.com/spf13/cobra"
)

func SuggestCmd() *cobra.Command {
	var agent string
	cmd := &cobra.Command{
		Use:   "suggest <id>",
		Short: "Create a suggestions artifact for a PRD",
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
			suggDir := project.SuggestionsDir(root)
			if err := os.MkdirAll(suggDir, 0o755); err != nil {
				return err
			}
			suggPath, err := suggestions.Create(suggDir, id, now)
			if err != nil {
				return err
			}
			briefPath, err := writeSuggestionBrief(root, id, suggPath, now)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), filepath.Base(suggPath))
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

var launchAgent = agents.Launch
var launchSubagent = agents.LaunchSubagent

func isClaudeProfile(name string, profile agents.Profile) bool {
	if strings.EqualFold(name, "claude") {
		return true
	}
	return strings.Contains(strings.ToLower(profile.Command), "claude")
}

func agentProfiles(cfg config.Config) map[string]agents.Profile {
	out := make(map[string]agents.Profile)
	for name, profile := range cfg.Agents {
		out[name] = agents.Profile{Command: profile.Command, Args: profile.Args}
	}
	return out
}

func writeSuggestionBrief(root, id, suggPath string, now time.Time) (string, error) {
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
	content := buildSuggestionBrief(spec, suggPath)
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return briefPath, nil
}

func buildSuggestionBrief(spec specs.Spec, suggPath string) string {
	acceptance := []string{}
	for _, item := range spec.Acceptance {
		if strings.TrimSpace(item.Description) != "" {
			acceptance = append(acceptance, item.Description)
		}
	}
	base := brief.Compose(brief.Input{
		ID:           spec.ID,
		Title:        spec.Title,
		Summary:      spec.Summary,
		Requirements: spec.Requirements,
		Acceptance:   acceptance,
		ResearchFiles: spec.Research,
	})
	instructions := `\n\nInstructions:
- Create per-section suggestions for Summary, Requirements, CUJs, Market Research, Competitive Landscape.
- Set status: ready for any section you complete (leave pending otherwise).
- Use evidence refs for all research claims.
- Write results into the suggestions template at:
  ` + suggPath + `
`
	return base + instructions
}
