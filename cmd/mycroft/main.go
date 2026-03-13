package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "mycroft",
	Short: "Fleet orchestrator — observe, rank, dispatch",
	Long: `Mycroft coordinates AI agent sessions with escalating autonomy:
  T0: Observe and shadow-suggest (no actions)
  T1: Suggest dispatches, user approves/rejects
  T2: Auto-dispatch low-risk work (v0.2)
  T3: Full autonomous dispatch (v0.2)`,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the patrol loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("mycroft: patrol loop not yet implemented")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet status and current tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("mycroft: status not yet implemented")
		return nil
	},
}

var shadowsCmd = &cobra.Command{
	Use:   "shadows",
	Short: "Show shadow suggestion digest",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("mycroft: shadows not yet implemented")
		return nil
	},
}

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause dispatching (in-flight agents continue)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("mycroft: pause not yet implemented")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume dispatching at current tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("mycroft: resume not yet implemented")
		return nil
	},
}

var overrideCmd = &cobra.Command{
	Use:   "override <bead> <agent>",
	Short: "Manually assign a bead to an agent",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("mycroft: override %s → %s not yet implemented\n", args[0], args[1])
		return nil
	},
}

func init() {
	pauseCmd.Flags().Bool("drain", false, "Also signal in-flight agents to checkpoint and stop")
	rootCmd.AddCommand(runCmd, statusCmd, shadowsCmd, pauseCmd, resumeCmd, overrideCmd)
}
