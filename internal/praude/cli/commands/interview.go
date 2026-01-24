package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/research"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/scan"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// InterviewConfig holds configuration for non-interactive interview mode
type InterviewConfig struct {
	Vision       string   `yaml:"vision"`
	Users        string   `yaml:"users"`
	Problem      string   `yaml:"problem"`
	Requirements []string `yaml:"requirements"`
}

func InterviewCmd() *cobra.Command {
	var (
		agent         string
		vision        string
		users         string
		problem       string
		requirements  string
		skipScan      bool
		skipBootstrap bool
		skipResearch  bool
		configFile    string
	)
	cmd := &cobra.Command{
		Use:   "interview",
		Short: "Run guided interview to create a PRD",
		Long: `Run guided interview to create a PRD.

In interactive mode (default), prompts for input at each step.
In non-interactive mode (via flags or --config), uses provided values:

  praude interview --vision "..." --users "..." --problem "..." --requirements "req1,req2"
  praude interview --config answers.yaml

The config file format:
  vision: "Your vision statement"
  users: "Target users"
  problem: "Problem to solve"
  requirements:
    - "First requirement"
    - "Second requirement"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadFromRoot(root)
			if err != nil {
				return err
			}

			// Load config from file if provided
			var interviewCfg InterviewConfig
			if configFile != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return fmt.Errorf("failed to read config file: %w", err)
				}
				if err := yaml.Unmarshal(data, &interviewCfg); err != nil {
					return fmt.Errorf("failed to parse config file: %w", err)
				}
			}

			// Override with command-line flags (flags take precedence)
			if vision != "" {
				interviewCfg.Vision = vision
			}
			if users != "" {
				interviewCfg.Users = users
			}
			if problem != "" {
				interviewCfg.Problem = problem
			}
			if requirements != "" {
				interviewCfg.Requirements = splitInput(requirements)
			}

			// Determine if we're in non-interactive mode
			nonInteractive := configFile != "" || vision != "" || users != "" || problem != "" || requirements != ""

			reader := bufio.NewReader(cmd.InOrStdin())
			out := cmd.OutOrStdout()

			// Handle scanning
			summary := ""
			if !skipScan {
				if nonInteractive {
					// In non-interactive mode, always scan unless skipped
					res, _ := scan.ScanRepo(root, scan.Options{})
					summary = renderScanSummary(res)
				} else {
					scanNow, err := promptYesNo(reader, out, "Scan repo now? (y/n) ")
					if err != nil {
						return err
					}
					if scanNow {
						res, _ := scan.ScanRepo(root, scan.Options{})
						summary = renderScanSummary(res)
					}
				}
			}

			draft := buildDraftSpec(summary)
			fmt.Fprintln(out, "Draft PRD ready.")
			if draft.Summary != "" {
				fmt.Fprintln(out, draft.Summary)
			}

			// Confirm draft in interactive mode only
			if !nonInteractive {
				confirm, err := promptYesNo(reader, out, "Confirm draft? (y/n) ")
				if err != nil {
					return err
				}
				if !confirm {
					return nil
				}
			}

			// Get interview inputs
			var finalVision, finalUsers, finalProblem, finalRequirements string
			if nonInteractive {
				finalVision = interviewCfg.Vision
				finalUsers = interviewCfg.Users
				finalProblem = interviewCfg.Problem
				finalRequirements = strings.Join(interviewCfg.Requirements, ",")
			} else {
				finalVision, err = promptLine(reader, out, "Vision: ")
				if err != nil {
					return err
				}
				finalUsers, err = promptLine(reader, out, "Users: ")
				if err != nil {
					return err
				}
				finalProblem, err = promptLine(reader, out, "Problem: ")
				if err != nil {
					return err
				}
				finalRequirements, err = promptLine(reader, out, "Requirements (comma or newline separated): ")
				if err != nil {
					return err
				}
			}

			spec := buildSpecFromInterview(finalVision, finalUsers, finalProblem, finalRequirements)
			path, id, warnings, err := writeSpec(root, spec)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Created %s at %s\n", id, path)
			if len(warnings) > 0 {
				fmt.Fprintln(out, "Validation warnings:")
				for _, warning := range warnings {
					fmt.Fprintln(out, "- "+warning)
				}
			}

			// Auto-apply suggestions (skip if --skip-bootstrap)
			if !skipBootstrap {
				if err := autoApplySuggestions(root, id, cfg, agent, out); err != nil {
					fmt.Fprintln(out, "Suggestions failed:", err.Error())
				}
			}

			// Handle research
			if skipResearch {
				return nil
			}

			runResearch := true
			if !nonInteractive {
				runResearch, err = promptYesNo(reader, out, "Run research now? (y/n) ")
				if err != nil {
					return err
				}
			}
			if !runResearch {
				return nil
			}

			now := time.Now()
			researchDir := project.ResearchDir(root)
			if err := os.MkdirAll(researchDir, 0o755); err != nil {
				return err
			}
			researchPath, err := research.Create(researchDir, id, now)
			if err != nil {
				return err
			}
			briefPath, err := writeResearchBrief(root, id, researchPath, now)
			if err != nil {
				return err
			}
			profile, err := agents.Resolve(agentProfiles(cfg), agent)
			if err != nil {
				return err
			}
			launcher := launchAgent
			if isClaudeProfile(agent, profile) {
				launcher = launchSubagent
			}
			if err := launcher(profile, briefPath); err != nil {
				fmt.Fprintf(out, "agent not found; brief at %s\n", briefPath)
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "codex", "Agent profile to use")
	cmd.Flags().StringVar(&vision, "vision", "", "Vision statement (non-interactive)")
	cmd.Flags().StringVar(&users, "users", "", "Target users (non-interactive)")
	cmd.Flags().StringVar(&problem, "problem", "", "Problem statement (non-interactive)")
	cmd.Flags().StringVar(&requirements, "requirements", "", "Requirements, comma-separated (non-interactive)")
	cmd.Flags().BoolVar(&skipScan, "skip-scan", false, "Skip repository scan")
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "Skip agent bootstrap/suggestions")
	cmd.Flags().BoolVar(&skipResearch, "skip-research", false, "Skip research step")
	cmd.Flags().StringVar(&configFile, "config", "", "YAML config file with interview answers")
	return cmd
}

func autoApplySuggestions(root, id string, cfg config.Config, agent string, out io.Writer) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	suggDir := project.SuggestionsDir(root)
	if err := os.MkdirAll(suggDir, 0o755); err != nil {
		return err
	}
	now := time.Now()
	suggPath, err := suggestions.Create(suggDir, id, now)
	if err != nil {
		return err
	}
	briefPath, err := writeSuggestionBrief(root, id, suggPath, now)
	if err != nil {
		return err
	}
	profile, err := agents.Resolve(agentProfiles(cfg), agent)
	if err != nil {
		return err
	}
	launcher := launchAgent
	if isClaudeProfile(agent, profile) {
		launcher = launchSubagent
	}
	if err := launcher(profile, briefPath); err != nil {
		fmt.Fprintf(out, "agent not found; brief at %s\n", briefPath)
		return nil
	}
	applied, err := applyReadySuggestions(root, id, suggPath)
	if err != nil {
		return err
	}
	if applied {
		fmt.Fprintf(out, "Applied agent suggestions to %s\n", id)
	}
	return nil
}

func applyReadySuggestions(root, id, suggPath string) (bool, error) {
	raw, err := os.ReadFile(suggPath)
	if err != nil {
		return false, err
	}
	ready := suggestions.ParseReady(raw)
	if suggestions.IsEmpty(ready) {
		return false, nil
	}
	specPath, err := resolveSpecPath(project.SpecsDir(root), id)
	if err != nil {
		return false, err
	}
	if err := suggestions.Apply(specPath, ready); err != nil {
		return false, err
	}
	updated, err := os.ReadFile(specPath)
	if err != nil {
		return true, err
	}
	res, err := specs.Validate(updated, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		return true, err
	}
	if len(res.Warnings) > 0 {
		_ = specs.StoreValidationWarnings(specPath, res.Warnings)
	}
	return true, nil
}

func renderScanSummary(res scan.Result) string {
	return "Scan summary: " + itoa(len(res.Entries)) + " files, " + itoa(int(res.TotalBytes)) + " bytes"
}

func buildDraftSpec(summary string) specs.Spec {
	text := summary
	if strings.TrimSpace(text) == "" {
		text = "Draft from scan"
	}
	return specs.Spec{Title: "Draft PRD", Summary: text}
}

func buildSpecFromInterview(vision, users, problem, requirements string) specs.Spec {
	reqList := parseRequirements(requirements)
	if len(reqList) == 0 {
		reqList = []string{"REQ-001: TBD"}
	}
	firstReq := extractReqID(reqList[0])
	title := firstNonEmpty(vision, problem, "New PRD")
	summary := firstNonEmpty(problem, vision, "Summary pending")
	return specs.Spec{
		Title:        title,
		Summary:      summary,
		Requirements: reqList,
		StrategicContext: specs.StrategicContext{
			CUJID:       "CUJ-001",
			CUJName:     "Primary Journey",
			FeatureID:   "",
			MVPIncluded: true,
		},
		UserStory: specs.UserStory{
			Text: "As a user, " + firstNonEmpty(users, "I need", "I need") + ", " + summary,
			Hash: "pending",
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:                 "CUJ-001",
				Title:              "Primary Journey",
				Priority:           "high",
				Steps:              []string{"Start", "Finish"},
				SuccessCriteria:    []string{"Goal achieved"},
				LinkedRequirements: []string{firstReq},
			},
			{
				ID:                 "CUJ-002",
				Title:              "Maintenance",
				Priority:           "low",
				Steps:              []string{"Routine upkeep"},
				SuccessCriteria:    []string{"System remains stable"},
				LinkedRequirements: []string{firstReq},
			},
		},
	}
}

func writeSpec(root string, spec specs.Spec) (string, string, []string, error) {
	specDir := project.SpecsDir(root)
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return "", "", nil, err
	}
	id, err := specs.NextID(specDir)
	if err != nil {
		return "", "", nil, err
	}
	spec.ID = id
	if spec.CreatedAt == "" {
		spec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", id, nil, err
	}
	path := filepath.Join(specDir, id+".yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return path, id, nil, err
	}
	res, err := specs.Validate(raw, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		return path, id, nil, err
	}
	if len(res.Warnings) > 0 {
		if err := specs.StoreValidationWarnings(path, res.Warnings); err != nil {
			return path, id, res.Warnings, err
		}
	}
	return path, id, res.Warnings, nil
}

func parseRequirements(input string) []string {
	parts := splitInput(input)
	var out []string
	for i, part := range parts {
		id := formatReqID(i + 1)
		out = append(out, id+": "+part)
	}
	return out
}

func splitInput(input string) []string {
	input = strings.ReplaceAll(input, "\n", ",")
	parts := strings.Split(input, ",")
	var out []string
	for _, part := range parts {
		trim := strings.TrimSpace(part)
		if trim != "" {
			out = append(out, trim)
		}
	}
	return out
}

func formatReqID(n int) string {
	return "REQ-" + pad3(n)
}

func pad3(n int) string {
	if n < 10 {
		return "00" + itoa(n)
	}
	if n < 100 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func extractReqID(req string) string {
	fields := strings.Fields(req)
	if len(fields) == 0 {
		return "REQ-001"
	}
	id := strings.TrimSuffix(fields[0], ":")
	if strings.HasPrefix(id, "REQ-") {
		return id
	}
	return "REQ-001"
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}
