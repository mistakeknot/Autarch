package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/sources"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Pollard in the current project",
	Long:  `Create the .pollard directory structure and default configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Create directory structure
		if err := sources.EnsureDirectories(cwd); err != nil {
			return fmt.Errorf("failed to create directories: %w", err)
		}

		// Check if config already exists
		existingCfg, err := config.Load(cwd)
		if err == nil && len(existingCfg.Hunters) > 0 {
			fmt.Println("Pollard already initialized. Config exists at .pollard/config.yaml")
			return nil
		}

		// Create default config
		cfg := config.DefaultConfig()
		if err := cfg.Save(cwd); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Initialized Pollard in .pollard/")
		fmt.Println()
		fmt.Println("Created directories:")
		fmt.Println("  .pollard/")
		fmt.Println("  .pollard/insights/")
		fmt.Println("  .pollard/patterns/")
		fmt.Println("  .pollard/sources/")
		fmt.Println("  .pollard/reports/")
		fmt.Println()
		fmt.Println("Created config: .pollard/config.yaml")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  pollard scan         # Run research agents")
		fmt.Println("  pollard report       # Generate landscape report")
		fmt.Println("  pollard search <q>   # Search patterns and insights")

		return nil
	},
}
