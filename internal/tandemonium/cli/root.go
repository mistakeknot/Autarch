package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/cli/commands"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/plan"
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
	root.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize .tandemonium in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := project.Init("."); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Would you like to start planning? [Y/n]")
			return plan.Run(cmd.InOrStdin(), filepath.Join(".", ".tandemonium", "plan"))
		},
	})
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
	)
	root.Flags().BoolVarP(&quickMode, "quick", "q", false, "Create task in quick mode")
	return root
}
