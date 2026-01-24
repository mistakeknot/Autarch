package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	pollardPlan "github.com/mistakeknot/vauxpraudemonium/internal/pollard/plan"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/proposal"
	"github.com/mistakeknot/vauxpraudemonium/pkg/plan"
)

var (
	proposePlan       bool
	proposeOutput     string
	proposeMaxAgendas int
	proposeIncludeSrc bool
	proposeSelect     bool
)

var proposeCmd = &cobra.Command{
	Use:   "propose",
	Short: "Propose research agendas from project context",
	Long: `Scan project documentation and propose research agendas.

Uses your AI agent to analyze CLAUDE.md, AGENTS.md, README.md
and propose 3-5 research agendas. Select which to pursue.

Examples:
  pollard propose --plan     # Show what will be analyzed
  pollard apply              # Invoke agent, save proposals
  pollard propose --select   # Choose agendas interactively
  pollard propose            # Direct mode: scan, propose, save
`,
	RunE: runPropose,
}

func runPropose(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if proposePlan {
		return runProposePlan(cwd)
	}

	if proposeSelect {
		return runAgendaSelection(cwd)
	}

	// Direct mode: scan, invoke agent, save proposals
	return runDirectPropose(cwd)
}

// runProposePlan generates a plan for review before agent invocation.
func runProposePlan(cwd string) error {
	scanner := proposal.NewContextScanner(cwd)
	ctx, err := scanner.Scan()
	if err != nil {
		return fmt.Errorf("failed to scan project: %w", err)
	}

	// Collect found files
	var filesFound []string
	for filename := range ctx.Files {
		filesFound = append(filesFound, filename)
	}

	opts := pollardPlan.ProposePlanOptions{
		Root:         cwd,
		ProjectName:  ctx.ProjectName,
		Technologies: ctx.Technologies,
		DetectedType: ctx.DetectedType,
		Domain:       ctx.Domain,
		FilesFound:   filesFound,
		MaxAgendas:   proposeMaxAgendas,
		IncludeSrc:   proposeIncludeSrc,
	}

	p, err := pollardPlan.GenerateProposePlan(opts)
	if err != nil {
		return fmt.Errorf("failed to generate plan: %w", err)
	}

	// Save the plan
	planPath, err := p.Save(cwd)
	if err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}

	// Display the plan
	fmt.Println("Proposal Plan")
	fmt.Println("=============")
	fmt.Printf("Project: %s\n", ctx.ProjectName)
	if ctx.DetectedType != "" && ctx.DetectedType != "unknown" {
		fmt.Printf("Type: %s\n", ctx.DetectedType)
	}
	if len(ctx.Technologies) > 0 {
		fmt.Printf("Technologies: %s\n", strings.Join(ctx.Technologies, ", "))
	}
	if ctx.Domain != "" {
		fmt.Printf("Domain: %s\n", ctx.Domain)
	}
	fmt.Printf("Files to analyze: %s\n", strings.Join(filesFound, ", "))
	fmt.Printf("Max agendas: %d\n", proposeMaxAgendas)
	fmt.Println()

	// Show recommendations
	if len(p.Recommendations) > 0 {
		fmt.Println("Recommendations:")
		for _, r := range p.Recommendations {
			icon := "ℹ"
			switch r.Severity {
			case plan.SeverityError:
				icon = "✗"
			case plan.SeverityWarning:
				icon = "⚠"
			}
			fmt.Printf("  %s %s\n", icon, r.Message)
			if r.Suggestion != "" {
				fmt.Printf("    → %s\n", r.Suggestion)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Plan saved to: %s\n", planPath)
	fmt.Println("Run 'pollard apply' to invoke your agent and generate proposals")

	return nil
}

// runDirectPropose scans, invokes agent, saves proposals, then prompts for selection.
func runDirectPropose(cwd string) error {
	scanner := proposal.NewContextScanner(cwd)
	var ctx *proposal.ProjectContext
	var err error

	if proposeIncludeSrc {
		ctx, err = scanner.ScanWithSrc()
	} else {
		ctx, err = scanner.Scan()
	}
	if err != nil {
		return fmt.Errorf("failed to scan project: %w", err)
	}

	fmt.Printf("Scanning project: %s\n", ctx.ProjectName)
	if len(ctx.Files) == 0 {
		return fmt.Errorf("no documentation files found (CLAUDE.md, AGENTS.md, README.md)")
	}
	fmt.Printf("Found %d documentation file(s)\n", len(ctx.Files))

	// Create interruptible context
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nInterrupted...")
		cancel()
	}()

	// Create generator with config
	cfg := proposal.DefaultConfig()
	cfg.MaxAgendas = proposeMaxAgendas
	cfg.IncludeSrc = proposeIncludeSrc
	generator := proposal.NewAgendaGeneratorWithConfig(cfg)

	fmt.Println("Invoking AI agent to propose research agendas...")
	result, err := generator.Generate(runCtx, ctx)
	if err != nil {
		return fmt.Errorf("failed to generate proposals: %w", err)
	}

	// Save results
	if err := proposal.SaveResult(cwd, result); err != nil {
		return fmt.Errorf("failed to save proposals: %w", err)
	}

	// Display results for review
	displayProposalsForReview(result)

	// Prompt for selection
	return promptAndApplySelection(cwd, result)
}

// displayProposalsForReview shows proposals in a review-friendly format.
func displayProposalsForReview(result *proposal.ProposalResult) {
	fmt.Printf("\n")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf(" RESEARCH AGENDA PROPOSALS (%d generated)\n", len(result.Agendas))
	fmt.Println("═══════════════════════════════════════════════════════════════")

	for i, agenda := range result.Agendas {
		fmt.Println()
		priorityIcon := "○"
		switch agenda.Priority {
		case "high":
			priorityIcon = "●"
		case "medium":
			priorityIcon = "◐"
		}
		fmt.Printf("%s %d. [%s] %s\n", priorityIcon, i+1, agenda.ID, agenda.Title)
		fmt.Printf("   Priority: %s | Scope: %s\n", agenda.Priority, agenda.EstimatedScope)
		fmt.Printf("   %s\n", agenda.Description)
		fmt.Println()
		fmt.Println("   Research Questions:")
		for _, q := range agenda.Questions {
			fmt.Printf("     • %s\n", q)
		}
		if len(agenda.SuggestedHunters) > 0 {
			fmt.Printf("   Hunters: %s\n", strings.Join(agenda.SuggestedHunters, ", "))
		}
	}
	fmt.Println()
	fmt.Println("───────────────────────────────────────────────────────────────")
}

// promptAndApplySelection prompts user to select agendas and applies them.
func promptAndApplySelection(cwd string, result *proposal.ProposalResult) error {
	fmt.Println("Review the agendas above.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  • Enter numbers (e.g., 1,2) to select specific agendas")
	fmt.Println("  • Enter 'all' to select all agendas")
	fmt.Println("  • Enter 'none' or press Enter to skip (proposals saved for later)")
	fmt.Println()
	fmt.Print("Select agendas to apply: ")

	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)

	if input == "" || strings.ToLower(input) == "none" {
		fmt.Println()
		fmt.Println("No agendas applied. Proposals saved to .pollard/proposals/current.yaml")
		fmt.Println("Run 'pollard propose --select' later to apply agendas.")
		return nil
	}

	var selectedIDs []string
	if strings.ToLower(input) == "all" {
		for _, agenda := range result.Agendas {
			selectedIDs = append(selectedIDs, agenda.ID)
		}
	} else {
		// Parse comma-separated numbers or IDs
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// Handle numeric selection (1, 2, 3, etc.)
			if idx := parseNumber(part); idx > 0 && idx <= len(result.Agendas) {
				selectedIDs = append(selectedIDs, result.Agendas[idx-1].ID)
			} else if part != "" {
				// Assume it's an ID
				selectedIDs = append(selectedIDs, part)
			}
		}
	}

	if len(selectedIDs) == 0 {
		fmt.Println("No valid agendas selected")
		return nil
	}

	// Show what will be applied
	fmt.Println()
	fmt.Println("Applying selected agendas:")
	for _, id := range selectedIDs {
		for _, agenda := range result.Agendas {
			if agenda.ID == id {
				fmt.Printf("  • %s\n", agenda.Title)
				break
			}
		}
	}
	fmt.Println()

	// Apply selected agendas
	selector := proposal.NewAgendaSelector(cwd)
	selResult, err := selector.ApplySelectedAgendasWithResult(selectedIDs, result)
	if err != nil {
		return fmt.Errorf("failed to apply agendas: %w", err)
	}

	// Display results
	fmt.Println(proposal.FormatSelectionResult(selResult))
	fmt.Println()
	fmt.Println("Next step: Run 'pollard scan' to execute research with the new queries")

	return nil
}

// parseNumber parses a string as a positive integer, returns 0 if invalid.
func parseNumber(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// runAgendaSelection allows interactive selection of agendas.
func runAgendaSelection(cwd string) error {
	// Load current proposals
	result, err := proposal.LoadResult(cwd)
	if err != nil {
		return fmt.Errorf("failed to load proposals: %w (run 'pollard propose' first)", err)
	}

	if len(result.Agendas) == 0 {
		return fmt.Errorf("no agendas found in proposals")
	}

	// Display agendas for selection
	fmt.Println("Available Research Agendas:")
	fmt.Println("===========================")
	for i, agenda := range result.Agendas {
		fmt.Printf("\n%d. [%s] %s\n", i+1, agenda.ID, agenda.Title)
		fmt.Printf("   Priority: %s | Scope: %s\n", agenda.Priority, agenda.EstimatedScope)
		fmt.Printf("   %s\n", agenda.Description)
		if len(agenda.Questions) > 0 {
			fmt.Println("   Questions:")
			for _, q := range agenda.Questions {
				fmt.Printf("     - %s\n", q)
			}
		}
		if len(agenda.SuggestedHunters) > 0 {
			fmt.Printf("   Hunters: %s\n", strings.Join(agenda.SuggestedHunters, ", "))
		}
	}

	// Prompt for selection
	fmt.Println()
	fmt.Println("Enter agenda IDs to apply (comma-separated), or 'all' for all:")
	fmt.Print("> ")

	var input string
	fmt.Scanln(&input)
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Println("No agendas selected")
		return nil
	}

	var selectedIDs []string
	if strings.ToLower(input) == "all" {
		for _, agenda := range result.Agendas {
			selectedIDs = append(selectedIDs, agenda.ID)
		}
	} else {
		// Parse comma-separated IDs
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// Handle numeric selection (1, 2, 3)
			if len(part) == 1 && part[0] >= '1' && part[0] <= '9' {
				idx := int(part[0] - '1')
				if idx < len(result.Agendas) {
					selectedIDs = append(selectedIDs, result.Agendas[idx].ID)
				}
			} else {
				// Assume it's an ID
				selectedIDs = append(selectedIDs, part)
			}
		}
	}

	if len(selectedIDs) == 0 {
		fmt.Println("No valid agendas selected")
		return nil
	}

	// Apply selected agendas
	selector := proposal.NewAgendaSelector(cwd)
	selResult, err := selector.ApplySelectedAgendasWithResult(selectedIDs, result)
	if err != nil {
		return fmt.Errorf("failed to apply agendas: %w", err)
	}

	// Display results
	fmt.Println()
	fmt.Println(proposal.FormatSelectionResult(selResult))
	fmt.Println("Run 'pollard scan' to execute research with the new queries")

	return nil
}

// outputResult outputs the proposal result in the specified format.
func outputResult(result *proposal.ProposalResult, format string) error {
	switch format {
	case "yaml":
		data, err := yaml.Marshal(result)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "markdown":
		fmt.Printf("# Research Agenda Proposals for %s\n\n", result.ProjectContext.ProjectName)
		fmt.Printf("Generated: %s\n\n", result.GeneratedAt.Format("2006-01-02 15:04:05"))
		for i, agenda := range result.Agendas {
			fmt.Printf("## %d. %s\n\n", i+1, agenda.Title)
			fmt.Printf("**ID:** %s  \n", agenda.ID)
			fmt.Printf("**Priority:** %s  \n", agenda.Priority)
			fmt.Printf("**Scope:** %s  \n\n", agenda.EstimatedScope)
			fmt.Printf("%s\n\n", agenda.Description)
			if len(agenda.Questions) > 0 {
				fmt.Println("### Research Questions")
			fmt.Println()
				for _, q := range agenda.Questions {
					fmt.Printf("- %s\n", q)
				}
				fmt.Println()
			}
			if len(agenda.SuggestedHunters) > 0 {
				fmt.Printf("**Suggested Hunters:** %s\n\n", strings.Join(agenda.SuggestedHunters, ", "))
			}
		}
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
	return nil
}

func init() {
	proposeCmd.Flags().BoolVar(&proposePlan, "plan", false, "Generate plan JSON (review before invoking agent)")
	proposeCmd.Flags().StringVar(&proposeOutput, "output", "yaml", "Output format: yaml, json, markdown")
	proposeCmd.Flags().IntVar(&proposeMaxAgendas, "max-agendas", 5, "Maximum agendas to propose")
	proposeCmd.Flags().BoolVar(&proposeIncludeSrc, "include-src", false, "Also scan source files for tech detection")
	proposeCmd.Flags().BoolVar(&proposeSelect, "select", false, "Interactive selection of agendas")
}
