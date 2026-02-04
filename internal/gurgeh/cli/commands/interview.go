package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/gurgeh/config"
	pollardquick "github.com/mistakeknot/autarch/internal/pollard/quick"
	praudePlan "github.com/mistakeknot/autarch/internal/gurgeh/plan"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/pkg/plan"
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
		vision       string
		users        string
		problem      string
		requirements string
		configFile   string
		planMode     bool
	)
	cmd := &cobra.Command{
		Use:   "interview",
		Short: "Run guided interview to create a PRD",
		Long: `Run guided interview to create a PRD using the Arbiter sprint engine.

All interviews use the full 8-phase Arbiter engine. Pre-populate phases
via flags or --config for non-interactive use:

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
			if _, err := config.LoadFromRoot(root); err != nil {
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

			// Check if vision spec is required before allowing another PRD
			summaries, _ := specs.LoadSummaries(project.SpecsDir(root))
			if specs.NeedsVisionSpec(summaries) {
				fmt.Fprintln(cmd.OutOrStdout(), "Vision spec required before creating another PRD. Starting vision sprint...")
				return runVisionSprint(cmd.OutOrStdout(), root, interviewCfg)
			}

			// Handle plan mode
			if planMode {
				return runInterviewPlan(cmd.OutOrStdout(), root, interviewCfg)
			}

			// Always use Arbiter sprint
			return runArbiterSprint(cmd.OutOrStdout(), root, interviewCfg)
		},
	}
	cmd.Flags().StringVar(&vision, "vision", "", "Vision statement (non-interactive)")
	cmd.Flags().StringVar(&users, "users", "", "Target users (non-interactive)")
	cmd.Flags().StringVar(&problem, "problem", "", "Problem statement (non-interactive)")
	cmd.Flags().StringVar(&requirements, "requirements", "", "Requirements, comma-separated (non-interactive)")
	cmd.Flags().StringVar(&configFile, "config", "", "YAML config file with interview answers")
	cmd.Flags().BoolVar(&planMode, "plan", false, "Generate plan JSON instead of executing")
	return cmd
}

// runArbiterSprint uses the Arbiter orchestrator to programmatically walk through
// all phases, accepting each draft automatically. Outputs the resulting spec.
func runArbiterSprint(out io.Writer, root string, cfg InterviewConfig) error {
	orch := arbiter.NewOrchestrator(root)
	orch.SetScanner(pollardquick.NewScanner())
	ctx := context.Background()

	state, err := orch.Start(ctx, cfg.Vision)
	if err != nil {
		return fmt.Errorf("starting sprint: %w", err)
	}

	// Accept vision and advance through all phases
	state = orch.AcceptDraft(state)

	for {
		state, err = orch.Advance(ctx, state)
		if err != nil {
			return fmt.Errorf("advancing sprint: %w", err)
		}

		// Inject known content for specific phases
		switch state.Phase {
		case arbiter.PhaseProblem:
			if cfg.Problem != "" {
				orch.ReviseDraft(state, cfg.Problem, "cli input")
			}
		case arbiter.PhaseUsers:
			if cfg.Users != "" {
				orch.ReviseDraft(state, cfg.Users, "cli input")
			}
		case arbiter.PhaseRequirements:
			if len(cfg.Requirements) > 0 {
				orch.ReviseDraft(state, strings.Join(cfg.Requirements, "\n- "), "cli input")
			}
		}

		state = orch.AcceptDraft(state)

		// Check if we've reached the last phase
		phases := arbiter.AllPhases()
		if state.Phase == phases[len(phases)-1] {
			break
		}
	}

	// Export to spec
	spec, err := orch.ExportSpec(state)
	if err != nil {
		return fmt.Errorf("exporting spec: %w", err)
	}

	path, id, vres, err := writeSpec(root, *spec)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Created %s at %s (via Arbiter sprint)\n", id, path)
	if len(vres.Errors) > 0 {
		fmt.Fprintln(out, "Validation errors:")
		for _, e := range vres.Errors {
			fmt.Fprintln(out, "  ✗ "+e)
		}
	}
	if len(vres.Warnings) > 0 {
		fmt.Fprintln(out, "Validation warnings:")
		for _, w := range vres.Warnings {
			fmt.Fprintln(out, "  - "+w)
		}
	}
	fmt.Fprintf(out, "Confidence: %.0f%%\n", state.Confidence.Total()*100)
	return nil
}

// runVisionSprint runs an Arbiter sprint in vision mode.
func runVisionSprint(out io.Writer, root string, cfg InterviewConfig) error {
	orch := arbiter.NewOrchestrator(root)
	orch.SetScanner(pollardquick.NewScanner())
	ctx := context.Background()

	state, err := orch.StartVision(ctx, cfg.Vision)
	if err != nil {
		return fmt.Errorf("starting vision sprint: %w", err)
	}

	state = orch.AcceptDraft(state)

	for {
		state, err = orch.Advance(ctx, state)
		if err != nil {
			return fmt.Errorf("advancing vision sprint: %w", err)
		}
		state = orch.AcceptDraft(state)

		phases := arbiter.AllPhases()
		if state.Phase == phases[len(phases)-1] {
			break
		}
	}

	spec, err := orch.ExportSpec(state)
	if err != nil {
		return fmt.Errorf("exporting vision spec: %w", err)
	}

	path, id, vres, err := writeSpec(root, *spec)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Created vision spec %s at %s\n", id, path)
	if len(vres.Errors) > 0 {
		fmt.Fprintln(out, "Validation errors:")
		for _, e := range vres.Errors {
			fmt.Fprintln(out, "  ✗ "+e)
		}
	}
	if len(vres.Warnings) > 0 {
		fmt.Fprintln(out, "Validation warnings:")
		for _, w := range vres.Warnings {
			fmt.Fprintln(out, "  - "+w)
		}
	}
	return nil
}


func writeSpec(root string, spec specs.Spec) (string, string, specs.ValidationResult, error) {
	specDir := project.SpecsDir(root)
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return "", "", specs.ValidationResult{}, err
	}
	id, err := specs.NextID(specDir)
	if err != nil {
		return "", "", specs.ValidationResult{}, err
	}
	spec.ID = id
	if spec.CreatedAt == "" {
		spec.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if spec.Status == "" {
		spec.Status = "draft"
	}
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", id, specs.ValidationResult{}, err
	}
	path := filepath.Join(specDir, id+".yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return path, id, specs.ValidationResult{}, err
	}
	res, err := specs.Validate(raw, specs.ValidationOptions{Mode: specs.ValidationSoft, Root: root})
	if err != nil {
		return path, id, specs.ValidationResult{}, err
	}
	if len(res.Warnings) > 0 {
		if err := specs.StoreValidationWarnings(path, res.Warnings); err != nil {
			return path, id, res, err
		}
	}
	return path, id, res, nil
}

// reqIDPrefix matches an existing REQ-NNN: prefix at the start of a requirement.
var reqIDPrefix = regexp.MustCompile(`^REQ-\d{3}:\s*`)

func parseRequirements(input string) []string {
	parts := splitRequirements(input)
	var out []string
	nextID := 1
	for _, part := range parts {
		if reqIDPrefix.MatchString(part) {
			// Already has a requirement ID — keep it as-is
			out = append(out, part)
		} else {
			id := formatReqID(nextID)
			out = append(out, id+": "+part)
		}
		nextID++
	}
	return out
}

// splitRequirements splits on newlines only — NOT commas, which are valid
// punctuation inside requirement text (e.g. "Must support SSO, SAML, and OIDC").
func splitRequirements(input string) []string {
	lines := strings.Split(input, "\n")
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		// Strip leading list markers ("- ", "* ", "1. ", etc.)
		trim = strings.TrimLeft(trim, "-*•")
		trim = strings.TrimSpace(trim)
		if trim != "" {
			out = append(out, trim)
		}
	}
	return out
}

// splitInput splits on commas or newlines. Used for non-requirement fields
// where comma separation is the documented interface (e.g. --requirements flag).
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

// runInterviewPlan generates a plan for the interview command.
func runInterviewPlan(out io.Writer, root string, cfg InterviewConfig) error {
	specDir := project.SpecsDir(root)
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return err
	}

	nextID, err := specs.NextID(specDir)
	if err != nil {
		return err
	}

	p, err := praudePlan.GenerateInterviewPlan(praudePlan.InterviewPlanOptions{
		Root:         root,
		NextID:       nextID,
		Vision:       cfg.Vision,
		Users:        cfg.Users,
		Problem:      cfg.Problem,
		Requirements: cfg.Requirements,
	})
	if err != nil {
		return err
	}

	// Save the plan
	planPath, err := p.Save(root)
	if err != nil {
		return err
	}

	// Output the plan as JSON
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(data))
	fmt.Fprintf(out, "\nPlan saved to: %s\n", planPath)
	fmt.Fprintln(out, "Run 'praude apply' to execute this plan.")

	return nil
}

// Ensure we use the imported packages
var _ = plan.Version
var _ = praudePlan.GenerateInterviewPlan
