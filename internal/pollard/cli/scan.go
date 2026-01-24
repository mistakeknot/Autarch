package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/hunters"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/sources"
	"github.com/mistakeknot/vauxpraudemonium/internal/pollard/state"
)

var (
	scanHunter string
	scanDryRun bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run research hunters to collect data",
	Long:  `Run all configured research hunters or a specific hunter to collect data from sources.`,
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

		if len(cfg.Hunters) == 0 {
			fmt.Println("No hunters configured. Run 'pollard init' to create a default config.")
			return nil
		}

		// Open state database
		db, err := state.Open(cwd)
		if err != nil {
			return fmt.Errorf("failed to open state database: %w", err)
		}
		defer db.Close()

		// Set up context with cancellation
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle interrupt signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nInterrupted, stopping hunters...")
			cancel()
		}()

		// Get the hunter registry
		registry := hunters.DefaultRegistry()

		// Determine which hunters to run
		hunterNames := cfg.EnabledHunters()
		if scanHunter != "" {
			// Run only the specified hunter
			if _, ok := cfg.GetHunterConfig(scanHunter); !ok {
				return fmt.Errorf("hunter %q not found in config", scanHunter)
			}
			hunterNames = []string{scanHunter}
		}

		if len(hunterNames) == 0 {
			fmt.Println("No enabled hunters to run.")
			return nil
		}

		// Dry run mode - just show what would run
		if scanDryRun {
			fmt.Println("Dry run - would execute these hunters:")
			for _, name := range hunterNames {
				hunterCfg, _ := cfg.GetHunterConfig(name)
				fmt.Printf("  %s:\n", name)
				fmt.Printf("    interval: %s\n", hunterCfg.Interval)
				fmt.Printf("    output: %s\n", hunterCfg.Output)
				if len(hunterCfg.Queries) > 0 {
					fmt.Printf("    queries: %d\n", len(hunterCfg.Queries))
				}
				if len(hunterCfg.Targets) > 0 {
					fmt.Printf("    targets: %d\n", len(hunterCfg.Targets))
				}
			}
			return nil
		}

		// Run each hunter
		for _, name := range hunterNames {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			hunter, ok := registry.Get(name)
			if !ok {
				fmt.Printf("Warning: hunter %q not found in registry, skipping\n", name)
				continue
			}

			hunterCfg, _ := cfg.GetHunterConfig(name)
			fmt.Printf("Running hunter: %s\n", name)

			// Record run start
			runID, err := db.StartRun(name)
			if err != nil {
				fmt.Printf("  Warning: failed to record run start: %v\n", err)
			}

			// Build hunter config
			hCfg := hunters.HunterConfig{
				Queries:     hunterCfg.Queries,
				MaxResults:  hunterCfg.MaxResults,
				MinStars:    hunterCfg.MinStars,
				MinPoints:   hunterCfg.MinPoints,
				Categories:  hunterCfg.Categories,
				OutputDir:   hunterCfg.Output,
				ProjectPath: cwd,
			}

			// Add targets for competitor tracker
			for _, t := range hunterCfg.Targets {
				hCfg.Targets = append(hCfg.Targets, hunters.CompetitorTarget{
					Name:      t.Name,
					Changelog: t.Changelog,
					Docs:      t.Docs,
					GitHub:    t.GitHub,
				})
			}

			// Execute the hunt
			result, err := hunter.Hunt(ctx, hCfg)
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				if runID > 0 {
					db.CompleteRun(runID, false, 0, 0, err.Error())
				}
				continue
			}

			// Record run completion
			success := result.Success()
			errMsg := ""
			if !success && len(result.Errors) > 0 {
				errMsg = result.Errors[0].Error()
			}
			if runID > 0 {
				db.CompleteRun(runID, success, result.SourcesCollected, result.InsightsCreated, errMsg)
			}

			// Print results
			fmt.Printf("  %s\n", result.String())
			if len(result.OutputFiles) > 0 {
				fmt.Printf("  Output files:\n")
				for _, f := range result.OutputFiles {
					fmt.Printf("    - %s\n", f)
				}
			}
			if len(result.Errors) > 0 {
				fmt.Printf("  Warnings:\n")
				for _, e := range result.Errors {
					fmt.Printf("    - %v\n", e)
				}
			}
		}

		return nil
	},
}

func init() {
	scanCmd.Flags().StringVar(&scanHunter, "hunter", "", "Run a specific hunter by name")
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "Show what would run without executing")
}
