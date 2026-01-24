package cli

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/cli/commands"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/epics"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/prd"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/tui"
	"github.com/spf13/cobra"
)

func Execute() error {
	root := newRootCommand()
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	var quickMode bool
	var initAgent string
	var initExisting string
	var initDepth int
	var initUseTUI bool
	root := &cobra.Command{
		Use:   "tandemonium",
		Short: "Task orchestration for human-AI collaboration",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if !quickMode {
					return fmt.Errorf("PM refinement not implemented; use -q for quick mode")
				}
				rootDir, err := project.FindRoot(".")
				if err != nil {
					return err
				}
				prompt := strings.TrimSpace(strings.Join(args, " "))
				path, err := specs.CreateQuickSpec(project.SpecsDir(rootDir), prompt, time.Now())
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Created quick task spec: %s\n", path)
				return nil
			}
			cfg, err := config.LoadFromProject(".")
			if err != nil {
				return err
			}
			m := tui.NewModel()
			m.ConfirmApprove = cfg.TUI.ConfirmApprove
			m.RefreshTasks()
			p := tea.NewProgram(m)
			_, err = p.Run()
			return err
		},
	}
	var initFromPRD string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .tandemonium in current directory",
		Long: `Initialize .tandemonium in current directory.

By default, runs exploration and generates epics with an AI agent.
Use --from-prd to import epics from an existing Praude PRD instead:

  tandemonium init --from-prd PRD-001
  tandemonium init --from-prd mvp
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle --from-prd flag
			if initFromPRD != "" {
				return runInitFromPRD(cmd, initFromPRD, initExisting, cmd.Flags().Changed("existing"))
			}
			opts := initOptions{
				Agent:       initAgent,
				Existing:    initExisting,
				ExistingSet: cmd.Flags().Changed("existing"),
				Depth:       initDepth,
				DepthSet:    cmd.Flags().Changed("depth"),
				UseTUI:      initUseTUI,
			}
			return runInit(cmd.OutOrStdout(), cmd.InOrStdin(), opts)
		},
	}
	initCmd.Flags().StringVar(&initAgent, "agent", "claude", "Agent target to use for init")
	initCmd.Flags().StringVar(&initExisting, "existing", "skip", "Existing epic handling (skip|overwrite|prompt)")
	initCmd.Flags().IntVar(&initDepth, "depth", 0, "Exploration depth (1-3)")
	initCmd.Flags().BoolVar(&initUseTUI, "tui", false, "Show progress UI")
	initCmd.Flags().StringVar(&initFromPRD, "from-prd", "", "Import epics from a Praude PRD (e.g., PRD-001, mvp)")
	root.AddCommand(initCmd)
	root.AddCommand(
		commands.AgentCmd(),
		commands.StatusCmd(),
		commands.DoctorCmd(),
		commands.RecoverCmd(),
		commands.CleanupCmd(),
		commands.ApproveCmd(),
		commands.MailCmd(),
		commands.LockCmd(),
		commands.PlanCmd(),
		commands.ExecuteCmd(),
		commands.StopCmd(),
		commands.ExportCmd(),
		commands.ImportCmd(),
		commands.ScanCmd(),
	)
	root.Flags().BoolVarP(&quickMode, "quick", "q", false, "Create task in quick mode")
	return root
}

// runInitFromPRD imports epics from a Praude PRD.
func runInitFromPRD(cmd *cobra.Command, prdID, existing string, existingSet bool) error {
	out := cmd.OutOrStdout()

	// Initialize the .tandemonium directory
	if err := project.Init("."); err != nil {
		return err
	}

	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Importing epics from PRD %s...\n", prdID)

	result, err := prd.ImportFromPRD(prd.ImportOptions{
		Root:  root,
		PRDID: prdID,
	})
	if err != nil {
		return fmt.Errorf("failed to import PRD: %w", err)
	}

	if len(result.Warnings) > 0 {
		fmt.Fprintln(out, "Warnings:")
		for _, w := range result.Warnings {
			fmt.Fprintf(out, "  - %s\n", w)
		}
	}

	if len(result.Epics) == 0 {
		fmt.Fprintln(out, "No epics generated from PRD.")
		return nil
	}

	fmt.Fprintf(out, "Generated %d epic(s) from PRD %s\n", len(result.Epics), result.SourcePRD)
	for _, e := range result.Epics {
		storyCount := len(e.Stories)
		fmt.Fprintf(out, "  - %s: %s (%d stories)\n", e.ID, e.Title, storyCount)
	}

	specsDir := project.SpecsDir(root)
	existingMode := existing
	if existingMode == "" {
		existingMode = "skip"
	}

	var writeOpts epics.WriteOptions
	switch strings.ToLower(existingMode) {
	case "overwrite":
		writeOpts.Existing = epics.ExistingOverwrite
	default:
		writeOpts.Existing = epics.ExistingSkip
	}

	if err := epics.WriteEpics(specsDir, result.Epics, writeOpts); err != nil {
		return fmt.Errorf("failed to write epics: %w", err)
	}

	fmt.Fprintf(out, "Wrote epics to %s\n", specsDir)
	return nil
}
