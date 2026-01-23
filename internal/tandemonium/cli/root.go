package cli

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/cli/commands"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
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
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .tandemonium in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
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
