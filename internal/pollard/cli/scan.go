package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/sources"
)

var scanAgent string

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run research agents to collect data",
	Long:  `Run all configured research agents or a specific agent to collect data from sources.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		cfg, err := config.Load(cwd)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Ensure directories exist
		if err := sources.EnsureDirectories(cwd); err != nil {
			return fmt.Errorf("failed to create directories: %w", err)
		}

		if len(cfg.Agents) == 0 {
			fmt.Println("No agents configured. Run 'pollard init' to create a default config.")
			return nil
		}

		for _, agent := range cfg.Agents {
			if scanAgent != "" && agent.Name != scanAgent {
				continue
			}
			fmt.Printf("Running agent: %s\n", agent.Name)
			// TODO: Implement actual agent execution
			fmt.Printf("  Schedule: %s\n", agent.Schedule)
			fmt.Printf("  Output: %s\n", agent.Output)
			fmt.Printf("  Sources: %d configured\n", len(agent.Sources))
		}

		return nil
	},
}

func init() {
	scanCmd.Flags().StringVar(&scanAgent, "agent", "", "Run a specific agent by name")
}
